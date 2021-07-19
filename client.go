// Copyright 2021 fangyousong(方友松). All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// 客户端实现
package iip

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/truexf/goutil"
)

type ClientConfig struct {
	MaxConnections        int           //单client最大连接数
	MaxChannelsPerConn    int           //单connection最大channel数
	ChannelPacketQueueLen uint32        //channel的packet接收队列长度
	TcpWriteQueueLen      uint32        //connection的packet写队列长度
	TcpConnectTimeout     time.Duration //服务器连接超时限制
	TcpReadBufferSize     int           //内核socket读缓冲区大小
	TcpWriteBufferSize    int           //内核socket写缓冲区大小
}

type Client struct {
	DefaultErrorHolder
	DefaultContext
	isTls       bool
	tlsCertFile string
	tlsKeyFile  string
	config      ClientConfig
	serverAddr  string
	connections []*Connection
	connLock    sync.RWMutex
	handler     *clientHandler
	Count       *Count
	Measure     *Measure
}

type ClientChannel struct {
	internalChannel *Channel
	client          *Client

	// "uncompleted request queue"
	// iip协议要求每一个request对应一个response。
	// channel是“全双工”的，也就是说request可以连续发出，response也会保证按request的次序顺序到达，同时调用handler进行处理。
	// 前一个request的response不会阻塞下一个request,也就是不用等待一个request的rewponse完成，就可以继续发送下一个request。
	// 当异步的response到达的时候，handler需要知道该response对应哪个request（可能之前已经发出多个request）。因此，通过一个
	// “未收到response的已经发出request队列”来保存发出request，当response到达时，获取队列的第一个元素即为该response对应的request。
	// 在handle函数里,可以通过channel.GetCtxData(CtxRequest)获得当前响应对应的请求
	uncompletedRequestQueue *goutil.LinkedList
	sendRequestLock         sync.Mutex
}

// 创建一个新的client
func NewClient(config ClientConfig, serverAddr string, timeCountRangeFunc EnsureTimeRangeFunc) (*Client, error) {
	ret := &Client{
		config:      config,
		serverAddr:  serverAddr,
		connections: make([]*Connection, 0),
		handler:     &clientHandler{pathHandlerManager: &ClientPathHandlerManager{}},
		Count:       &Count{},
		Measure:     NewMesure(timeCountRangeFunc),
	}
	ret.config.MaxChannelsPerConn += 1 //channel 0 is internal required
	return ret, nil
}

// 创建一个新的client, TLS模式
func NewClientTLS(config ClientConfig, serverAddr string, timeCountRangeFunc EnsureTimeRangeFunc, certFile, keyFile string) (*Client, error) {
	ret := &Client{
		isTls:       true,
		tlsCertFile: certFile,
		tlsKeyFile:  keyFile,
		config:      config,
		serverAddr:  serverAddr,
		connections: make([]*Connection, 0),
		handler:     &clientHandler{pathHandlerManager: &ClientPathHandlerManager{}},
		Count:       &Count{},
		Measure:     NewMesure(timeCountRangeFunc),
	}
	return ret, nil
}

func (m *Client) Close() {
	for _, v := range m.connections {
		v.Close(nil)
	}
}

// 创建一个新的channel
// 每个connection会默认建立一个ID为0的信道，用于基础通讯功能，创建一个新的channel就是通过这个0号channel实现的：
// 创建channel的流程由client发起，服务器返回新创建的channel id，后续的业务通讯（request/response）应该在新创建的channel上进行
func (m *Client) NewChannel() (*ClientChannel, error) {
	conn, err := m.getFreeConnection()
	if err != nil {
		return nil, err
	}

	return m.newChannel(conn)
}

func (m *Client) wrapperChannel(c *Channel) *ClientChannel {
	if c == nil {
		return nil
	}
	ret := &ClientChannel{
		internalChannel:         c,
		client:                  m,
		uncompletedRequestQueue: c.GetCtxData(CtxUncompletedRequestChan).(*goutil.LinkedList),
	}
	c.SetCtxData(CtxClientChannel, ret)
	return ret
}

func (m *Client) newChannel(conn *Connection) (*ClientChannel, error) {
	c := &ClientChannel{
		internalChannel:         conn.Channels[0],
		client:                  m,
		uncompletedRequestQueue: conn.Channels[0].GetCtxData(CtxUncompletedRequestChan).(*goutil.LinkedList),
	}
	c.SetCtx(CtxUncompletedRequestChan, c.uncompletedRequestQueue)
	c.SetCtx(CtxClient, m)
	bts, err := c.DoRequest(PathNewChannel, NewDefaultRequest([]byte("{}")), time.Second*5)
	if err != nil {
		return nil, fmt.Errorf("new sys channel fail, %s", err.Error())
	}

	var resp ResponseNewChannel
	if err := json.Unmarshal(bts, &resp); err != nil {
		return nil, err
	}
	ucrq := goutil.NewLinkedList(true)
	if resp.ChannelId > 0 && resp.Code == 0 {
		cInternal := conn.newChannel(false,
			m.config.ChannelPacketQueueLen,
			map[string]interface{}{CtxUncompletedRequestChan: ucrq, CtxClient: m},
			nil,
		)
		c := m.wrapperChannel(cInternal)
		return c, nil
	} else {
		return nil, fmt.Errorf(resp.Message)
	}
}

// 获取一个最不忙碌的channel:
// 1. 找到发送队列最短的connection，如果其channel数<limit, 则在其上创建一个channel返回，否则:
// 2. 如果connection数<limit, newConnection,并在其上创建一个channel返回，否则返回 nil, error
func (m *Client) GetFreeChannel() (*ClientChannel, error) {
	var freeConn *Connection
	sendQueueLen := math.MaxInt32
	m.connLock.RLock()
	for _, v := range m.connections {
		if sendQueueLen > len(v.tcpWriteQueue) {
			sendQueueLen = len(v.tcpWriteQueue)
			freeConn = v
		}
	}
	m.connLock.RUnlock()

	if freeConn == nil {
		var err error
		if freeConn, err = m.newConnection(); err != nil {
			return nil, err
		}
	}

	if freeConn != nil {
		freeConn.ChannelsLock.RLock()
		defer freeConn.ChannelsLock.RUnlock()
		var ch *Channel
		minQueue := int(m.config.ChannelPacketQueueLen)
		for k, v := range freeConn.Channels {
			if k == 0 {
				continue
			}
			qLen := len(v.receivedQueue)
			if qLen < minQueue {
				minQueue = qLen
				ch = v
			}
		}

		if ch != nil {
			ret := ch.GetCtxData(CtxClientChannel).(*ClientChannel)
			return ret, nil
		} else if len(freeConn.Channels) < m.config.MaxChannelsPerConn {
			return m.newChannel(freeConn)
		}
	}
	
	return nil, ErrClientConnectionsLimited
}

func (m *Client) dialTLS() (net.Conn, error) {
	log.Logf("dail tls")
	cert, err := tls.LoadX509KeyPair(m.tlsCertFile, m.tlsKeyFile)
	if err != nil {
		return nil, err
	}
	config := tls.Config{Certificates: []tls.Certificate{cert}, InsecureSkipVerify: true}
	conn, err := tls.Dial("tcp4", m.serverAddr, &config)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func (m *Client) newConnection() (*Connection, error) {
	m.connLock.RLock()
	connLen := len(m.connections)
	m.connLock.RUnlock()
	if connLen >= m.config.MaxConnections {
		return nil, ErrClientConnectionsLimited
	}

	var conn net.Conn
	var err error
	if !m.isTls {
		conn, err = net.DialTimeout("tcp4", m.serverAddr, m.config.TcpConnectTimeout)
	} else {
		conn, err = m.dialTLS()
	}
	if err != nil {
		return nil, err
	}
	// tcpConn := conn.(*net.TCPConn)
	ret, err := NewConnection(m, nil, conn, RoleClient, int(m.config.TcpWriteQueueLen))
	if err != nil {
		return nil, err
	}
	ret.SetCtxData(CtxClient, m)

	if !m.isTls {
		tcpConn := conn.(*net.TCPConn)
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(time.Second * 15)
		tcpConn.SetReadBuffer(m.config.TcpReadBufferSize)
		tcpConn.SetWriteBuffer(m.config.TcpWriteBufferSize)
	}

	m.connLock.Lock()
	m.connections = append(m.connections, ret)
	m.connLock.Unlock()
	return ret, nil
}

func (m *Client) removeConnection(conn *Connection) {
	m.connLock.Lock()
	defer m.connLock.Unlock()
	for i, v := range m.connections {
		if v == conn {
			conns := m.connections[:i]
			if i < len(m.connections) {
				conns = append(conns, m.connections[i+1:]...)
			}
			m.connections = conns
			return
		}
	}
}
func (m *Client) getFreeConnection() (*Connection, error) {
	var conn *Connection = nil
	m.connLock.Lock()
	for _, v := range m.connections {
		v.ChannelsLock.Lock()
		if len(v.Channels) < m.config.MaxChannelsPerConn {
			conn = v
			v.ChannelsLock.Unlock()
			break
		}
		v.ChannelsLock.Unlock()
	}
	m.connLock.Unlock()
	var err error
	if conn == nil {
		conn, err = m.newConnection()
		if err != nil {
			log.Errorf("connect server fail, %s\n", err.Error())
		}
	}
	return conn, err
}

// 注册Path-Handler
// iip协议中包含一个path字段，该字段一般用来代表具体的服务器接口和资源
// client和server通过注册对path的处理函数，以实现基于iip框架的开发
func (m *Client) RegisterHandler(path string, handler ClientPathHandler) error {
	return m.handler.pathHandlerManager.registerHandler(path, handler)
}

// 取消注册Path-Handler
func (m *Client) UnRegisterHandler(path string) {
	m.handler.pathHandlerManager.unRegisterHandler(path)
}

// 用于"消息式"请求/响应（系统自动将多个部分的响应数据合成为一个完整的响应，并通过这个阻塞的函数返回）
func (m *ClientChannel) DoRequest(path string, request Request, timeout time.Duration) ([]byte, error) {
	if m.internalChannel != nil && m.internalChannel.err != nil {
		return nil, fmt.Errorf("this channel is invalid, [%s]", m.internalChannel.err.Error())
	}
	if !ValidatePath(path) {
		return nil, fmt.Errorf("invalid path of request: %s", path)
	}

	pkt := &Packet{
		Type:      PacketTypeRequest,
		Path:      path,
		ChannelId: m.internalChannel.Id,
		Data:      request.Data(),
		channel:   m.internalChannel,
		DontChunk: true,
	}
	if len(pkt.Data) == 0 {
		return nil, fmt.Errorf("request.Data() is nil")
	}
	respChan := make(chan []byte)
	request.SetCtxData(CtxResponseChan, respChan)
	m.sendRequestLock.Lock()
	if err := m.internalChannel.SendPacket(pkt); err != nil {
		m.sendRequestLock.Unlock()
		return nil, err
	} else {
		m.uncompletedRequestQueue.PushTail(request, true)
		m.sendRequestLock.Unlock()
	}

	defer func() {
		request.RemoveCtxData(CtxResponseChan)
		close(respChan)
	}()

	if timeout > 0 {
		select {
		case <-time.After(timeout):
			return nil, ErrRequestTimeout
		case resp := <-respChan:
			if resp != nil {
				return resp, nil
			}
		}
	} else {
		resp := <-respChan
		if resp != nil {
			return resp, nil
		}
	}
	return nil, ErrUnknown
}

// 用于于流式请求/响应（用户自己注册处理Handler，每接收到一部分响应数据，系统会调用Handler一次，这个调用是异步的，发送函数立即返回）
func (m *ClientChannel) DoStreamRequest(path string, request Request) error {
	if m == nil {
		time.Sleep(time.Second)
		panic("client channel is nil")
	}
	if m.internalChannel != nil {
		if m.internalChannel.err != nil {
			return fmt.Errorf("this channel is invalid, [%s]", m.internalChannel.err.Error())
		}
	}
	if !ValidatePath(path) {
		return fmt.Errorf("invalid path: %s", path)
	}

	pkt := &Packet{
		Type:      PacketTypeRequest,
		Path:      path,
		ChannelId: m.internalChannel.Id,
		Data:      request.Data(),
		channel:   m.internalChannel,
		DontChunk: true,
	}
	if len(pkt.Data) == 0 {
		return fmt.Errorf("request.Data() is nil")
	}
	m.sendRequestLock.Lock()
	defer m.sendRequestLock.Unlock()
	if err := m.internalChannel.SendPacket(pkt); err != nil {
		return err
	} else {
		m.uncompletedRequestQueue.PushTail(request, true)
	}

	return nil
}

// 关闭channel
func (m *ClientChannel) Close(err error) {
	if m.internalChannel != nil {
		m.internalChannel.Close(err)
	}
}

func (m *ClientChannel) SetCtx(key string, value interface{}) {
	m.internalChannel.SetCtxData(key, value)
}

func (m *ClientChannel) GetCtx(key string) interface{} {
	return m.internalChannel.GetCtxData(key)
}

type EvaluatedClient struct {
	paused            bool
	lastRequest       time.Time
	client            *Client
	requestErrorCount int
	err               error
}

func (m *EvaluatedClient) DoRequest(path string, request Request, timeout time.Duration) ([]byte, error) {
	channel, err := m.client.GetFreeChannel()
	if err != nil {
		m.err = err
		m.paused = true
		return nil, err
	}
	m.lastRequest = time.Now()
	return channel.DoRequest(path, request, timeout)
}

// 由一组server提供无状态服务的场景下，LoadBalanceClient根据可配置的负载权重、keepalive检测，自动调节对不同server的请求频率
// LoadBalanceClient内部管理多个client
type LoadBalanceClient struct {
	activeClients []*EvaluatedClient
	idx           int
	clientsLock   sync.RWMutex
}

type AddrWeightClient struct {
	Addr   string
	Weight int
	client *Client
}

func (m *LoadBalanceClient) getFreeClient() (*EvaluatedClient, error) {
	m.clientsLock.RLock()
	defer m.clientsLock.RUnlock()
	idx := m.idx
	iterCnt := 0
	for {
		iterCnt++
		idx++
		if idx >= len(m.activeClients) {
			idx = 0
		}
		m.idx = idx
		if !m.activeClients[idx].paused {
			return m.activeClients[idx], nil
		} else {
			if time.Since(m.activeClients[idx].lastRequest) > time.Second*15 {
				return m.activeClients[idx], nil
			}
		}
		if iterCnt >= len(m.activeClients) {
			break
		}
	}
	log.Error("no alived client")
	return nil, fmt.Errorf("no alived client")
}

func (m *LoadBalanceClient) Status() string {
	m.clientsLock.RLock()
	defer m.clientsLock.RLock()
	ret := ""
	for _, v := range m.activeClients {
		ret += fmt.Sprintf("%s\n", v.client.serverAddr)
	}
	return ret
}

func (m *LoadBalanceClient) DoRequest(path string, request Request, timeout time.Duration) ([]byte, error) {
	client, err := m.getFreeClient()
	if err != nil {
		return nil, err
	}
	ret, err := client.DoRequest(path, request, timeout)
	if err != nil {
		client.requestErrorCount++
		if client.requestErrorCount > 3 {
			client.paused = true
		}
	} else {
		if client.requestErrorCount > 0 {
			client.requestErrorCount--
		}
		if client.requestErrorCount == 0 {
			client.paused = false
		}
	}
	return ret, err
}

// 创建一个LoadBalanceClient
// severList格式：ip:port#weight,ip:port#weight,ip:port#weight,...
// 0<weight<100,表明server的权重
func NewLoadBalanceClient(cfg ClientConfig, serverList string) (*LoadBalanceClient, error) {
	log.Debugf("new iip blc, serverlist: %s", serverList)
	svrs := strings.Split(serverList, ",")
	var addrList []*AddrWeightClient
	for _, v := range svrs {
		s := strings.TrimSpace(v)
		if s == "" {
			continue
		}
		aw := strings.Split(s, "#")
		if len(aw) != 2 {
			continue
		}
		if aw[0] == "" {
			continue
		}
		if weight, err := strconv.Atoi(aw[1]); err != nil {
			continue
		} else {
			if weight < 1 || weight > 100 {
				continue
			}
			addrList = append(addrList, &AddrWeightClient{Addr: aw[0], Weight: weight})
		}
	}
	if len(addrList) == 0 {
		return nil, fmt.Errorf("invalid param: serverList")
	}

	var addrListFinal []*AddrWeightClient
	for _, v := range addrList {
		if client, err := NewClient(cfg, v.Addr, nil); err == nil {
			v.client = client
			addrListFinal = append(addrListFinal, v)
		}
	}

	ret := &LoadBalanceClient{}
	exists := false
	for {
		for _, v := range addrListFinal {
			if v.Weight > 0 {
				v.Weight--
				ret.activeClients = append(ret.activeClients, &EvaluatedClient{client: v.client})
				if v.Weight > 0 {
					exists = true
				}
			}
		}
		if !exists {
			break
		} else {
			exists = false
		}
	}
	if log.GetLevel() == LogLevelDebug {
		log.Debug(ret.Status())
	}
	return ret, nil
}
