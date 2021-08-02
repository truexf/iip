// Copyright 2021 fangyousong(方友松). All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// 客户端实现
package iip

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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
	WaitResponseTimeout   time.Duration
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
	connCount   int32
	connLock    sync.RWMutex
	handler     *clientHandler
	Count       *Count
	Measure     *Measure
}

func (m *Client) GetConnectionStatis() (respData []byte, e error) {
	conns := make(map[int]*Connection)
	m.connLock.Lock()
	for i, v := range m.connections {
		conns[i] = v
	}
	m.connLock.Unlock()

	ret := make(map[int]*ConnectionSatis)
	for i, v := range conns {
		rec := &ConnectionSatis{WriteQueue: len(v.tcpWriteQueue), Count: v.Count, Channels: make(map[uint32]struct {
			ReceiveQueue int
			Count        *Count
		})}
		v.ChannelsLock.RLock()
		for cId, c := range v.Channels {
			rec.Channels[cId] = struct {
				ReceiveQueue int
				Count        *Count
			}{ReceiveQueue: len(c.receivedQueue), Count: c.Count}
		}
		v.ChannelsLock.RUnlock()
		ret[i] = rec
	}
	return json.Marshal(&ret)
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
	if ret.config.ChannelPacketQueueLen <= 0 {
		ret.config.ChannelPacketQueueLen = 100
	}
	if ret.config.MaxChannelsPerConn <= 0 {
		ret.config.MaxChannelsPerConn = 10
	}
	if ret.config.MaxConnections <= 0 {
		ret.config.MaxConnections = 1000
	}
	if ret.config.TcpConnectTimeout <= 0 {
		ret.config.TcpConnectTimeout = time.Second * 3
	}
	if ret.config.TcpWriteBufferSize <= 0 {
		ret.config.TcpWriteBufferSize = 16 * 1024
	}
	if ret.config.TcpReadBufferSize <= 0 {
		ret.config.TcpReadBufferSize = 16 * 1024
	}
	if ret.config.TcpWriteQueueLen <= 0 {
		ret.config.TcpWriteQueueLen = 100
	}
	if ret.config.WaitResponseTimeout <= 0 {
		ret.config.WaitResponseTimeout = time.Second * 5
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
		c := &ClientChannel{client: m, uncompletedRequestQueue: ucrq}
		c.internalChannel = conn.newChannel(false,
			m.config.ChannelPacketQueueLen,
			map[string]interface{}{CtxUncompletedRequestChan: ucrq, CtxClient: m, CtxClientChannel: c},
			nil,
		)
		return c, nil
	} else {
		return nil, fmt.Errorf(resp.Message)
	}
}

func (m *Client) dialTLS(timeout time.Duration) (net.Conn, error) {
	log.Logf("dail tls")
	var cert tls.Certificate
	var err error
	config := tls.Config{Certificates: []tls.Certificate{}, InsecureSkipVerify: true}
	if m.tlsCertFile != "" && m.tlsKeyFile != "" {
		cert, err = tls.LoadX509KeyPair(m.tlsCertFile, m.tlsKeyFile)
		if err != nil {
			return nil, err
		}
		config.Certificates = append(config.Certificates, cert)
	}
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: timeout}, "tcp4", m.serverAddr, &config)
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
	dialTimeout := m.config.TcpConnectTimeout
	if dialTimeout <= 0 {
		dialTimeout = time.Second * 3
	}
	if !m.isTls {
		conn, err = net.DialTimeout("tcp4", m.serverAddr, dialTimeout)
	} else {
		conn, err = m.dialTLS(dialTimeout)
	}
	if err != nil {
		return nil, err
	}
	// tcpConn := conn.(*net.TCPConn)
	ret, err := NewConnection(m, nil, conn, RoleClient, int(m.config.TcpWriteQueueLen))
	if err != nil {
		conn.Close()
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
	atomic.AddInt32(&m.connCount, 1)
	return ret, nil
}

func (m *Client) removeConnection(conn *Connection) {
	m.connLock.Lock()
	defer m.connLock.Unlock()
	for i, v := range m.connections {
		if v == conn {
			atomic.AddInt32(&m.connCount, -1)
			conns := m.connections[:i]
			if i < len(m.connections) {
				conns = append(conns, m.connections[i+1:]...)
			}
			m.connections = conns
			conn.Close(fmt.Errorf("from Client.removeConnection"))
			return
		}
	}
}
func (m *Client) getFreeConnection() (*Connection, error) {
	var conn *Connection = nil
	m.connLock.Lock()
	for _, v := range m.connections {
		if len(v.Channels) < m.config.MaxChannelsPerConn {
			conn = v
			break
		}
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

	respChan := acquireNotifyChan() //make(chan []byte)
	request.SetCtxData(CtxResponseChan, respChan)
	defer func() {
		request.RemoveCtxData(CtxResponseChan)
		releaseNotifyChan(respChan)
	}()

	if err := func() error {
		m.sendRequestLock.Lock()
		defer m.sendRequestLock.Unlock()
		m.uncompletedRequestQueue.PushTail(request, true)
		if err := m.internalChannel.SendPacket(pkt); err != nil {
			m.uncompletedRequestQueue.PopTail(true)
			return err
		}
		return nil
	}(); err != nil {
		return nil, err
	}

	if timeout <= 0 {
		timeout = time.Second * 10
	}

	select {
	case <-time.After(timeout):
		return nil, ErrRequestTimeout
	case resp := <-respChan:
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
	m.uncompletedRequestQueue.PushTail(request, true)
	if err := m.internalChannel.SendPacket(pkt); err != nil {
		m.uncompletedRequestQueue.PopTail(true)
		return err
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
	blc               *LoadBalanceClient
	paused            bool
	lastRequest       time.Time
	client            *Client
	connIndex         int
	requestErrorCount int
	err               error
	getChannelLock    sync.Mutex
}

func (m *EvaluatedClient) getTaskChannel() (*ClientChannel, error) {
	m.getChannelLock.Lock()
	defer m.getChannelLock.Unlock()

	if m.client == nil {
		return nil, fmt.Errorf("not init yet")
	}

	if m.client.connCount < int32(m.blc.serverKeepConns) {
		if conn, err := m.client.newConnection(); err != nil {
			return nil, fmt.Errorf("connect to %s fail, %s", m.client.serverAddr, err.Error())
		} else {
			if c, err := m.client.newChannel(conn); err != nil {
				m.client.removeConnection(conn)
				return nil, fmt.Errorf("new channel to %s fail, %s", m.client.serverAddr, err.Error())
			} else {
				conn.SetCtxData(CtxLblClientChannel, c)
				return c, nil
			}
		}
	} else {
		m.client.connLock.RLock()
		defer m.client.connLock.RUnlock()
		m.connIndex++
		if m.connIndex >= int(m.client.connCount) {
			m.connIndex = 0
		}
		ret := m.client.connections[m.connIndex].GetCtxData(CtxLblClientChannel)
		if ret == nil {
			return nil, fmt.Errorf("connection have no lbl-client-channel")
		}
		return ret.(*ClientChannel), nil

	}
}

func (m *EvaluatedClient) DoRequest(path string, request Request, timeout time.Duration) ([]byte, error) {
	channel, err := m.getTaskChannel()
	if err != nil {
		m.err = err
		m.paused = true
		return nil, err
	}
	m.paused = false
	m.lastRequest = time.Now()
	if ret, err := channel.DoRequest(path, request, timeout); err == nil {
		return ret, err
	} else {
		channel.Close(err)
		channel.internalChannel.conn.Close(fmt.Errorf("client do request fail, %s", err.Error()))
		return ret, fmt.Errorf("request [%s], %s", m.client.serverAddr, err.Error())
	}
}

// 由一组server提供无状态服务的场景下，LoadBalanceClient根据可配置的负载权重、keepalive检测，自动调节对不同server的请求频率
// LoadBalanceClient内部管理多个client
type LoadBalanceClient struct {
	serverMaxConns  int
	serverKeepConns int
	activeClients   []*EvaluatedClient
	clientsLock     sync.RWMutex
	idx             int //当前轮转的server index
}

func (m *LoadBalanceClient) getTaskClient() (*EvaluatedClient, error) {
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
			if time.Since(m.activeClients[idx].lastRequest) > time.Second*5 {
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

func (m *LoadBalanceClient) GetConnectionStatis() ([]byte, error) {
	retFinal := make(map[string]map[int]*ConnectionSatis)
	m.clientsLock.RLock()
	defer m.clientsLock.RLock()
	for _, ec := range m.activeClients {
		c := ec.client
		conns := make(map[int]*Connection)
		c.connLock.Lock()
		for i, v := range c.connections {
			conns[i] = v
		}
		c.connLock.Unlock()

		ret := make(map[int]*ConnectionSatis)
		for i, v := range conns {
			rec := &ConnectionSatis{WriteQueue: len(v.tcpWriteQueue), Count: v.Count, Channels: make(map[uint32]struct {
				ReceiveQueue int
				Count        *Count
			})}
			v.ChannelsLock.RLock()
			for cId, c := range v.Channels {
				rec.Channels[cId] = struct {
					ReceiveQueue int
					Count        *Count
				}{ReceiveQueue: len(c.receivedQueue), Count: c.Count}
			}
			v.ChannelsLock.RUnlock()
			ret[i] = rec
		}

		retFinal[c.serverAddr] = ret
	}
	return json.Marshal(retFinal)
}

func (m *LoadBalanceClient) DoRequest(path string, request Request, timeout time.Duration) ([]byte, error) {
	client, err := m.getTaskClient()
	if err != nil {
		return nil, err
	}
	ret, err := client.DoRequest(path, request, timeout)
	if err != nil {
		client.requestErrorCount++
		if client.requestErrorCount > 5 {
			log.Errorf("[%s] error 5+ times, paused, latest error: %s", client.client.serverAddr, err.Error())
			client.paused = true
		}
	} else {
		if client.requestErrorCount > 0 {
			client.requestErrorCount = 0
		}
		if client.requestErrorCount == 0 && client.paused {
			client.err = nil
			client.paused = false
		}
	}
	return ret, err
}

type AddrWeightClient struct {
	Addr   string
	Weight int
	client *Client
}

// 创建一个LoadBalanceClient，
// severList格式：ip:port#weight,ip:port#weight,ip:port#weight,...，
// 0<weight<100,表明server的权重。weight大于1，相当于增加weight-1个相同地址的server
// 每个server地址保持serverKeepConns个活跃连接，如果并发突增超过serverKeepConns，则自动创建新连接。
// serverMaxConns指定单个server最大连接数，所有的server都超出serverMaxConns，则返回ErrClientConnectionsLimited错误,否则以其他有空的server代其服务
// server任务分配以轮转模式运行
// 由于load balance客户端一般式针对同类型的业务，因此没有必要区分多个channel,内部每个connection开启一个channel.
func NewLoadBalanceClient(serverKeepConns int, serverMaxConns int, serverList string) (*LoadBalanceClient, error) {
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
		cfg := ClientConfig{MaxConnections: serverMaxConns,
			MaxChannelsPerConn:    3,
			ChannelPacketQueueLen: 1000,
			TcpWriteQueueLen:      1000,
			TcpConnectTimeout:     time.Second * 3,
			TcpReadBufferSize:     1024 * 32,
			TcpWriteBufferSize:    1024 * 32,
		}
		if client, err := NewClient(cfg, v.Addr, nil); err == nil {
			v.client = client
			addrListFinal = append(addrListFinal, v)
		}
	}

	ret := &LoadBalanceClient{serverMaxConns: serverMaxConns, serverKeepConns: serverKeepConns}
	if ret.serverKeepConns < 1 {
		ret.serverKeepConns = 1
	}
	if ret.serverMaxConns < 1 {
		ret.serverMaxConns = 1
	}

	exists := false
	for {
		for _, v := range addrListFinal {
			if v.Weight > 0 {
				v.Weight--
				ret.activeClients = append(ret.activeClients, &EvaluatedClient{client: v.client, blc: ret})
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
