// Copyright 2021 fangyousong(方友松). All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

//客户端实现
package iip

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"
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
	config      ClientConfig
	serverAddr  string
	connections []*Connection
	connLock    sync.Mutex
	handler     *clientHandler
}

type ClientChannel struct {
	internalChannel *Channel
	client          *Client
}

//创建一个新的client
func NewClient(config ClientConfig, serverAddr string) (*Client, error) {
	ret := &Client{
		config:      config,
		serverAddr:  serverAddr,
		connections: make([]*Connection, 0),
		handler:     &clientHandler{pathHandlerManager: &PathHandlerManager{}},
	}
	return ret, nil
}

//创建一个新的channel
//每个connection会默认建立一个ID为0的信道，用于基础通讯功能，创建一个新的channel就是通过这个0号channel实现的：
//创建channel的流程由client发起，服务器返回新创建的channel id，后续的业务通讯（request/response）应该在新创建的channel上进行
func (m *Client) NewChannel() (*ClientChannel, error) {
	conn, err := m.getFreeConnection()
	if err != nil {
		return nil, err
	}

	c := &ClientChannel{internalChannel: conn.Channels[0], client: m}
	bts, err := c.DoRequest(PathNewChannel, []byte("{}"), time.Second)
	if err != nil {
		return nil, err
	}
	var resp ResponseNewChannel
	if err := json.Unmarshal(bts, &resp); err != nil {
		return nil, err
	}
	if resp.ChannelId > 0 && resp.Code == 0 {
		c := &ClientChannel{internalChannel: conn.newChannel(m.config.ChannelPacketQueueLen), client: m}
		c.client.SetCtxData(CtxClient, m)
		return c, nil
	} else {
		return nil, fmt.Errorf(resp.Message)
	}
}

func (m *Client) newConnection() (*Connection, error) {
	conn, err := net.DialTimeout("tcp4", m.serverAddr, m.config.TcpConnectTimeout)
	if err != nil {
		return nil, err
	}
	tcpConn := conn.(*net.TCPConn)
	ret, err := NewConnection(tcpConn, RoleClient, int(m.config.TcpWriteQueueLen))
	if err != nil {
		return nil, err
	}

	tcpConn.SetKeepAlive(true)
	tcpConn.SetKeepAlivePeriod(time.Second * 15)
	tcpConn.SetReadBuffer(m.config.TcpReadBufferSize)
	tcpConn.SetWriteBuffer(m.config.TcpWriteBufferSize)

	m.connLock.Lock()
	m.connections = append(m.connections, ret)
	m.connLock.Unlock()
	return ret, nil
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
	}
	return conn, err
}

//用于"消息式"请求/响应（系统自动将多个部分的响应数据合成为一个完整的响应，并通过这个阻塞的函数返回）
func (m *ClientChannel) DoRequest(path string, requestData []byte, timeout time.Duration) ([]byte, error) {
	if m.internalChannel != nil && m.internalChannel.err != nil {
		return nil, fmt.Errorf("this channel is invalid, [%s]", m.internalChannel.err.Error())
	}

	pkt := &Packet{
		Type:      PacketTypeRequest,
		Path:      path,
		ChannelId: m.internalChannel.Id,
		Data:      requestData,
		channel:   m.internalChannel,
	}
	if err := m.internalChannel.SendPacket(pkt); err != nil {
		return nil, err
	}

	respChan := make(chan *Packet)
	m.internalChannel.SetCtxData(CtxResponseChan, respChan)
	defer func() {
		m.internalChannel.RemoveCtxData(CtxResponseChan)
		close(respChan)
	}()

	if timeout > 0 {
		select {
		case <-time.After(timeout):
			return nil, ErrRequestTimeout
		case resp := <-respChan:
			if resp != nil {
				return resp.Data, nil
			}
		}
	} else {
		resp := <-respChan
		if resp != nil {
			return resp.Data, nil
		}
	}
	return nil, ErrUnknown
}

//用于于流式请求/响应（用户自己注册处理Handler，每接收到一部分响应数据，系统会调用Handler一次，这个调用是异步的，发送函数立即返回）
func (m *ClientChannel) DoStreamRequest(path string, requestData []byte) error {
	if m.internalChannel != nil && m.internalChannel.err != nil {
		return fmt.Errorf("this channel is invalid, [%s]", m.internalChannel.err.Error())
	}

	pkt := &Packet{
		Type:      PacketTypeRequest,
		Path:      path,
		ChannelId: m.internalChannel.Id,
		Data:      requestData,
		channel:   m.internalChannel,
	}
	if err := m.internalChannel.SendPacket(pkt); err != nil {
		return err
	}

	return nil
}

//关闭channel
func (m *ClientChannel) Close(err error) {
	if m.internalChannel != nil {
		m.internalChannel.Close(err)
	}
}

//注册Path-Handler
//iip协议中包含一个path字段，该字段一般用来代表具体的服务器接口和资源
//client和server通过注册对path的处理函数，以实现基于iip框架的开发
func (m *Client) RegisterHandler(path string, handler PathHandler) error {
	return m.handler.pathHandlerManager.registerHandler(path, handler)
}

//取消注册Path-Handler
func (m *Client) UnRegisterHandler(path string) {
	m.handler.pathHandlerManager.unRegisterHandler(path)
}
