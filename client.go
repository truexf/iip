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
	connLock    sync.Mutex
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
		c := &ClientChannel{
			internalChannel: conn.newChannel(false,
				m.config.ChannelPacketQueueLen,
				map[string]interface{}{CtxUncompletedRequestChan: ucrq, CtxClient: m},
				nil,
			),
			client:                  m,
			uncompletedRequestQueue: ucrq,
		}
		return c, nil
	} else {
		return nil, fmt.Errorf(resp.Message)
	}
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
