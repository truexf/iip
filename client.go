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
	TcpReadTimeout        time.Duration
	TcpWriteTimeout       time.Duration
}

type Client struct {
	config      ClientConfig
	serverAddr  string
	connections []*Connection
	connLock    sync.Mutex
	channels    map[uint32]*Channel
	channelLock sync.Mutex
}

func NewClient(config ClientConfig, serverAddr string) (*Client, error) {
	ret := &Client{config: config, serverAddr: serverAddr, channels: make(map[uint32]*Connection), connections: make([]*Connection, 0)}
	return ret, nil
}

func (m *Client) NewChannel() (uint32, error) {
	conn, err := m.getFreeConnection()
	if err != nil {
		return 0, err
	}

	bts, err := m.doRequest(nil, NewChannelPath, []byte("{}"), time.Second)
	if err != nil {
		return 0, err
	}
	var resp ResponseNewChannel
	if err := json.Unmarshal(bts, &resp); err != nil {
		return 0, err
	}
	if resp.ChannelId > 0 && resp.Code == 0 {
		m.channelLock.Lock()
		m.channels[resp.ChannelId] = conn.newChannel(m.config.ChannelPacketQueueLen)
		m.channelLock.Unlock()
		return resp.ChannelId, nil
	} else {
		return 0, fmt.Errorf(resp.Message)
	}
}

func (m *Client) newConnection() (*Connection, error) {
	tcpAddr, err := net.ResolveTCPAddr("tcp4", m.serverAddr)
	if err != nil {
		return nil, err
	}
	tcpConn, err := net.DialTCP("tcp4", nil, tcpAddr)
	if err != nil {
		return nil, err
	}
	ret, err := NewConnection(tcpConn, RoleClient, int(m.config.TcpWriteQueueLen))
	if err != nil {
		return nil, err
	}
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
		if len(m.channels) < m.config.MaxChannelsPerConn {
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

func (m *Client) doRequest(channel *Channel, path string, requestData []byte, timeout time.Duration) ([]byte, error) {
	pkt := &Packet{
		Type:      PacketTypeRequest,
		Path:      path,
		ChannelId: channel.Id,
		Data:      requestData,
		channel:   channel,
	}
	if err := channel.SendPacket(pkt); err != nil {
		return nil, err
	}
	if timeout > 0 {
		select {
		case <-time.After(timeout):
			err := fmt.Errorf("wait response timeout")
			channel.Close(err)
			return nil, err
		case resp := <-channel.serverResponse:
			if resp != nil {
				return resp.Data, nil
			}
		}
	} else {
		resp := <-channel.serverResponse
		if resp != nil {
			return resp.Data, nil
		}
	}
	return nil, fmt.Errorf("unknown")
}

func (m *Client) DoRequest(channelId uint32, path string, requestData []byte, timeout time.Duration) ([]byte, error) {
	m.channelLock.Lock()
	c, ok := m.channels[channelId]
	m.channelLock.Unlock()
	if !ok {
		return nil, fmt.Errorf("invalid channel id: %d", channelId)
	}
	return m.doRequest(c, path, requestData)
}

func (m *Client) CloseChannel(channdlId uint32) error {

}
