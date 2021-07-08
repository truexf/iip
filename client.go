package iip

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"
)

type ClientConfig struct {
	MaxConnections        int
	MaxChannelsPerConn    int
	ChannelPacketQueueLen uint32
	TcpWriteQueueLen      uint32
	TcpConnectTimeout     time.Duration
	TcpReadBufferSize     int
	TcpWriteBufferSize    int
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

func NewClient(config ClientConfig, serverAddr string) (*Client, error) {
	ret := &Client{
		config:      config,
		serverAddr:  serverAddr,
		connections: make([]*Connection, 0),
		handler:     &clientHandler{pathHandlerManager: &PathHandlerManager{}},
	}
	return ret, nil
}

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

//对于"消息式"请求/响应（系统自动将多个部分的响应数据合成为一个完整的响应，并通过这个阻塞的函数返回）
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

//对于流式请求/响应（用户自己注册处理Handler，每接收到一部分响应数据，系统会调用Handler一次，这个调用是异步的，发送函数立即返回）
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

func (m *ClientChannel) Close(err error) {
	if m.internalChannel != nil {
		m.internalChannel.Close(err)
	}
}

func (m *Client) RegisterHandler(path string, handler PathHandler) error {
	return m.handler.pathHandlerManager.registerHandler(path, handler)
}

func (m *Client) UnRegisterHandler(path string) {
	m.handler.pathHandlerManager.unRegisterHandler(path)
}
