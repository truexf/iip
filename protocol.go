package iip

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"net"
	"sync"
	"time"
)

const (
	MaxPathLen            uint32 = 512
	MaxPacketSize         uint32 = 16 * 1024 * 1024
	PacketReadBufSize     uint32 = 16 * 1024
	ChannelPacketQueueLen uint32 = 1000
	ConnWriteQueueLen     int    = 1000
	NewChannelPath        string = "/sys/new_channel"
	DeleteChannelPath     string = "/sys/delete_channel"
	RoleClient            byte   = 0
	RoleServer            byte   = 4
	PacketTypeRequest     byte   = 0
	PacketTypeResponse    byte   = 4
	StatusC0              byte   = 0 //请求首帧，请求未完成
	StatusC1              byte   = 1 //请求首帧，请求完成
	StatusC2              byte   = 2 //请求后续帧，请求未完成
	StatusC3              byte   = 3 //请求后续帧，请求完成
	StatusS4              byte   = 4 //响应首帧，响应未完成
	StatusS5              byte   = 5 //表示响应首帧，响应完成
	StatusS6              byte   = 6 //表示响应后续帧，响应未完成
	StatusS7              byte   = 7 //表示响应后续帧，响应完成
	Status8               byte   = 8 //关闭连接
)

func isClientStatus(status byte) bool {
	return status == StatusC0 || status == StatusC1 || status == StatusC2 || status == StatusC3
}

func isServerStatus(status byte) bool {
	return status == StatusS4 || status == StatusS5 || status == StatusS6 || status == StatusS7
}

func isClientStatusCompleted(status byte) bool {
	return status == StatusC1 || status == StatusC3
}

func isClientStatusUncompleted(status byte) bool {
	return status == StatusC0 || status == StatusC2
}

func isServerStatusCompleted(status byte) bool {
	return status == StatusS5 || status == StatusS7
}

func isServerStatusUncompleted(status byte) bool {
	return status == StatusS4 || status == StatusS6
}

type Packet struct {
	Type      byte   `json:"type"` //0 request, 4 response
	Status    byte   `json:"status"`
	Path      string `json:"path"`
	ChannelId uint32 `json:"channel_id"`
	Data      []byte `json:"data"`
	channel   *Channel
}

/*
帧格式：
* 1字节数据帧状态标识：
	0表示请求首帧，请求未完成
	1表示请求首帧，请求完成
	2表示请求后续帧，请求未完成
	3表示请求后续帧，请求完成;
	4表示响应首帧，响应未完成
	5表示响应首帧，响应完成
	6表示响应后续帧，响应未完成
	7表示响应后续帧，响应完成
	8关闭连接
* 文本路径（只存在于请求首帧。与unix路径格式相同，类似于url的path，用于指明请求的路径,限制不能大于1024字节）
* \0
* 4字节channel识符（多路复用的流身份ID，无符号整数，请求方自增实现）
* 4字节数据长度（限制一个帧的数据长度不能大于16MB）
* 数据
*/
func CreateNetPacket(pkt *Packet) ([]byte, error) {
	if len(pkt.Path) > int(MaxPathLen) {
		return nil, fmt.Errorf("path is too large, must be <= %d bytes", MaxPathLen)
	}
	if len(pkt.Data) > int(MaxPacketSize) {
		return nil, fmt.Errorf("data is too large, must be <= %d bytes", MaxPacketSize)
	}
	pktLen := 1 + len(pkt.Path) + 4 + 4 + len(pkt.Data)
	pktData := make([]byte, 0, pktLen)
	pktData = append(pktData, pkt.Status)          //packet type
	pktData = append(pktData, []byte(pkt.Path)...) //path
	pktData = append(pktData, 0)                   //\0
	bt := make([]byte, 4)
	binary.BigEndian.PutUint32(bt, pkt.ChannelId)
	pktData = append(pktData, bt...) //channel id
	binary.BigEndian.PutUint32(bt, uint32(len(pkt.Data)))
	pktData = append(pktData, bt...)       //data length
	pktData = append(pktData, pkt.Data...) //data
	return pktData, nil
}

func WritePacket(pkt *Packet, writer io.Writer) (int, error) {
	data, err := CreateNetPacket(pkt)
	if err != nil {
		return 0, err
	}
	n, err := writer.Write(data)
	if err != nil {
		return n, err
	}
	if n != len(data) {
		return n, fmt.Errorf("writepacket not complete, totoal %d bytes, %d bytes writted. ", len(data), n)
	}
	if pkt.channel != nil {
		pkt.channel.WriteBytes += int64(n)
	}
	return n, nil
}

func CheckClientPacketStatus(prev, current byte) error {
	switch current {
	case StatusC0, StatusC1:
		if prev != 255 && !isClientStatusCompleted(prev) {
			return fmt.Errorf("invalid protocol, prev status: %d, current %d", prev, current)
		}
	case StatusC2, StatusC3:
		if !isClientStatusUncompleted(prev) {
			return fmt.Errorf("invalid protocol, prev status: %d, current %d", prev, current)
		}
	case Status8:
		return nil
	default:
		return fmt.Errorf("invalid status value: %d", current)
	}
	return nil
}

func CheckServerPacketStatus(prev, current byte) error {
	switch current {
	case StatusS4, StatusS5:
		if prev != 255 && !isServerStatusCompleted(prev) {
			return fmt.Errorf("invalid protocol, prev status: %d, current %d", prev, current)
		}
	case StatusS6, StatusS7:
		if !isServerStatusUncompleted(prev) {
			return fmt.Errorf("invalid protocol, prev status: %d, current %d", prev, current)
		}
	case Status8:
		return nil
	default:
		return fmt.Errorf("invalid status value: %d", current)
	}
	return nil
}

type Channel struct {
	Id               uint32
	NewTime          time.Time
	WritePacketCount int64
	ReadPacketCount  int64
	ReadBytes        int64
	WriteBytes       int64
	Conn             *Connection
	ReceivedQueue    chan *Packet //received streamed packet from peer side
	serverResponse   chan *Packet //for client side, merge 1 or 1+ packet into an whole response
	PacketStatus     byte         //recent received packet status
	err              error
	closeNotify      chan int
	PacketHandler    Handler //for server side
}

func (m *Channel) SendPacket(pkt *Packet) error {
	if m.err != nil {
		return fmt.Errorf("current channel is invalid, %s", m.err.Error())
	}
	if len(pkt.Data) <= int(MaxPacketSize) {
		if m.Conn.Role == RoleClient {
			pkt.Status = 1
		} else if m.Conn.Role == RoleServer {
			pkt.Status = 5
		}
		m.Conn.tcpWriteQueue <- pkt
		m.WritePacketCount++
		return nil
	}
	remainDataSize := len(pkt.Data)
	firstSend := true
	for {
		chunkSize := int(MaxPacketSize)
		if remainDataSize < int(MaxPacketSize) {
			chunkSize = remainDataSize
		}
		start := len(pkt.Data) - remainDataSize
		end := start + chunkSize
		chunk := &Packet{Type: pkt.Type, Path: pkt.Path, ChannelId: m.Id, Data: pkt.Data[start:end], channel: m}
		if chunkSize == remainDataSize {
			if m.Conn.Role == RoleClient {
				if firstSend {
					chunk.Status = 1
				} else {
					chunk.Status = 3
				}
			} else if m.Conn.Role == RoleServer {
				if firstSend {
					chunk.Status = 5
				} else {
					chunk.Status = 7
				}
			} else {
				return fmt.Errorf("protocol error")
			}
		} else if chunkSize < remainDataSize {
			if m.Conn.Role == RoleClient {
				if firstSend {
					chunk.Status = 0
				} else {
					chunk.Status = 2
				}
			} else if m.Conn.Role == RoleServer {
				if firstSend {
					chunk.Status = 4
				} else {
					chunk.Status = 6
				}
			} else {
				return fmt.Errorf("protocol error")
			}
		} else {
			return fmt.Errorf("protocol error")
		}
		m.Conn.tcpWriteQueue <- chunk

		firstSend = false
		remainDataSize -= chunkSize
		if remainDataSize <= 0 {
			break
		}
	}

	m.WritePacketCount++
	return nil
}

func (m *Channel) handleServerLoop() {
	for {
		select {
		case <-m.closeNotify:
			return
		case pkt := <-m.ReceivedQueue:
			if pkt.Status == Status8 {
				m.Close(fmt.Errorf("closed by peer command"))
				return
			}
			handler := m.PacketHandler
			if handler == nil {
				handler = sysHandler
			}

			ret, err := handler.Handle(pkt)
			if err != nil {
				log.Errorf("handle pkt %s fail, %s", pkt.Path, err.Error())
			} else {
				retPkt := &Packet{
					Type:      PacketTypeResponse,
					Path:      pkt.Path,
					ChannelId: pkt.ChannelId,
					Data:      ret,
					channel:   m,
				}
				if err := m.SendPacket(retPkt); err != nil {
					log.Errorf("channel.SendPacket fail, %s", err.Error())
				}
			}

		}
	}
}

func (m *Channel) handleClientLoop() {
	// merge 1 or 1+ packet into an whole response
	var pktWholeResponse *Packet
	for {
		select {
		case <-m.closeNotify:
			return
		case pkt := <-m.ReceivedQueue:
			if pkt.Status == Status8 {
				m.Close(fmt.Errorf("closed by peer command"))
				return
			}
			if pktWholeResponse == nil {
				pktWholeResponse = pkt
			} else {
				pktWholeResponse.Data = append(pktWholeResponse.Data, pkt.Data...)
				pktWholeResponse.Status = pkt.Status
			}
			if isClientStatusCompleted(pkt.Status) {
				m.serverResponse <- pktWholeResponse
				pktWholeResponse = nil
			}
		}
	}
}

func (m *Channel) Close(err error) {
	m.SendPacket(&Packet{Type: 8, ChannelId: m.Id, channel: m})
	m.Conn.removeChannel(m)
	if err != nil {
		m.err = err
	} else {
		m.err = fmt.Errorf("unknown")
	}
	log.Errorf("channel closed: %s", err.Error())
	close(m.closeNotify)
}

type Connection struct {
	Role          byte //0 client, 4 server
	Channels      map[uint32]*Channel
	MaxChannelId  uint32
	FreeChannleId map[uint32]struct{}
	ChannelsLock  sync.RWMutex
	tcpConn       *net.TCPConn
	tcpWriteQueue chan *Packet
	closeNotify   chan int
	err           error
}

func NewConnection(netConn *net.TCPConn, role byte, writeQueueLen int) (*Connection, error) {
	if role != RoleClient && role != RoleServer {
		return nil, fmt.Errorf("invalid role value")
	}
	ret := &Connection{
		Role:          role,
		Channels:      make(map[uint32]*Channel),
		FreeChannleId: make(map[uint32]struct{}),
		tcpConn:       netConn,
		tcpWriteQueue: make(chan *Packet, writeQueueLen),
		closeNotify:   make(chan int, 1),
	}
	if role == RoleClient {
		go ret.clientReadLoop()
	} else {
		go ret.serverReadLoop()
	}
	go ret.writeLoop()

	return ret, nil
}

func (m *Connection) writeLoop() {
	for {
		select {
		case pkt := <-m.tcpWriteQueue:
			if _, err := WritePacket(pkt, m.tcpConn); err != nil {
				m.Close(err)
				return
			}
		case <-m.closeNotify:
			return
		}
	}
}

func (m *Connection) Close(err error) {
	if err != nil {
		m.err = err
	} else {
		m.err = fmt.Errorf("unknown")
	}
	log.Errorf("connection closed, role %d, remote addr: %s, error: %s", m.Role, m.tcpConn.RemoteAddr().String(), m.err.Error())
	m.tcpConn.Close()
	for _, v := range m.Channels {
		v.Close(fmt.Errorf("connection is closed"))
	}
	close(m.closeNotify)
}

func (m *Connection) makeNewChannelId() uint32 {
	m.ChannelsLock.Lock()
	defer m.ChannelsLock.Unlock()
	var ret uint32 = 0
	if len(m.FreeChannleId) > 0 {
		for k, _ := range m.FreeChannleId {
			ret = k
			delete(m.FreeChannleId, k)
			return ret
		}
		return ret
	}
	if m.MaxChannelId < math.MaxUint32 {
		ret = m.MaxChannelId + 1
		m.MaxChannelId++
		return ret
	}
	return 0
}

func (m *Connection) newChannel(queueLen uint32) *Channel {
	ret := &Channel{Id: m.makeNewChannelId(), NewTime: time.Now(), Conn: m, ReceivedQueue: make(chan *Packet, queueLen), PacketStatus: 255}
	m.ChannelsLock.Lock()
	defer m.ChannelsLock.Unlock()
	m.Channels[ret.Id] = ret
	if m.Role == RoleServer {
		go ret.handleServerLoop()
	} else if m.Role == RoleClient {
		go ret.handleClientLoop()
	}

	return ret
}

func (m *Connection) getChannel(channelId uint32) *Channel {
	if channelId == 0 {
		if m.Role == RoleClient {
			return
		}
	}
	m.ChannelsLock.RLock()
	defer m.ChannelsLock.RUnlock()
	c, ok := m.Channels[channelId]
	if ok {
		return c
	}
	return nil
}

func (m *Connection) removeChannel(c *Channel) {
	if c != nil {
		m.ChannelsLock.Lock()
		defer m.ChannelsLock.Unlock()
		delete(m.Channels, c.Id)
		m.FreeChannleId[c.Id] = struct{}{}
	}
}

func (m *Connection) clientReadLoop() {
	//利用bufio，每次从内核多读一些数据上来处理，减少对内核内存的读次数
	bufReader := bufio.NewReaderSize(m.tcpConn, int(PacketReadBufSize))
	btsChannelId := make([]byte, 4)
	btsDataLen := make([]byte, 4)
	for {
		if m.err != nil {
			break
		}
		//read status
		status, err := bufReader.ReadByte()
		if err != nil {
			m.Close(fmt.Errorf("read data fail, %s", err.Error()))
			return
		}
		if status == Status8 {
			m.Close(fmt.Errorf("connection closed by peer command"))
			return
		}

		//read path
		path, err := bufReader.ReadSlice(0)
		if err != nil {
			m.Close(fmt.Errorf("read data fail, %s", err.Error()))
			return
		}
		pathStr := string(path[:len(path)-1])

		//read channelID
		if _, err = io.ReadFull(bufReader, btsChannelId); err != nil {
			m.Close(fmt.Errorf("read data fail, %s", err.Error()))
			return
		}
		channelId := binary.BigEndian.Uint32(btsChannelId)
		channel := m.getChannel(channelId)
		if channel == nil {
			m.Close(fmt.Errorf("invalid channel id: %d", channelId))
			return
		}
		if err := CheckServerPacketStatus(channel.PacketStatus, status); err != nil {
			log.Errorf(err.Error())
			m.Close(err)
			return
		}

		//read datalen
		if _, err = io.ReadFull(bufReader, btsDataLen); err != nil {
			m.Close(fmt.Errorf("read data fail, %s", err.Error()))
			return
		}
		dataLen := binary.BigEndian.Uint32(btsDataLen)
		if dataLen > MaxPacketSize {
			m.Close(fmt.Errorf("read data len meta > max-packet-size"))
			return
		}
		if dataLen == 0 {
			m.Close(fmt.Errorf("invalid data len: %d", dataLen))
			return
		}

		//read data
		pkt := &Packet{Type: PacketTypeResponse, Status: status, Path: pathStr, ChannelId: channelId, Data: make([]byte, dataLen), channel: channel}
		if _, err = io.ReadFull(bufReader, pkt.Data); err != nil {
			log.Errorf("read data fail, %s", err.Error())
			m.Close(err)
			return
		}
		channel.PacketStatus = status
		channel.ReceivedQueue <- pkt
	}
}

func (m *Connection) serverReadLoop() {
	//利用bufio，每次从内核多读一些数据上来处理，减少对内核内存的读次数
	bufReader := bufio.NewReaderSize(m.tcpConn, int(PacketReadBufSize))
	btsChannelId := make([]byte, 4)
	btsDataLen := make([]byte, 4)
	for {
		if m.err != nil {
			break
		}
		//read status
		status, err := bufReader.ReadByte()
		if err != nil {
			m.Close(fmt.Errorf("read data fail, %s", err.Error()))
			return
		}
		if status == Status8 {
			m.Close(fmt.Errorf("connection closed by peer command"))
			return
		}

		//read path
		path, err := bufReader.ReadSlice(0)
		if err != nil {
			m.Close(fmt.Errorf("read data fail, %s", err.Error()))
			return
		}
		pathStr := string(path[:len(path)-1])

		//read channelID
		if _, err = io.ReadFull(bufReader, btsChannelId); err != nil {
			m.Close(fmt.Errorf("read data fail, %s", err.Error()))
			return
		}
		channelId := binary.BigEndian.Uint32(btsChannelId)
		channel := m.getChannel(channelId)
		if channel == nil {
			m.Close(fmt.Errorf("invalid channel id: %d", channelId))
			return
		}
		if err := CheckClientPacketStatus(channel.PacketStatus, status); err != nil {
			log.Errorf(err.Error())
			m.Close(err)
			return
		}

		//read datalen
		if _, err = io.ReadFull(bufReader, btsDataLen); err != nil {
			m.Close(fmt.Errorf("read data fail, %s", err.Error()))
			return
		}
		dataLen := binary.BigEndian.Uint32(btsDataLen)
		if dataLen > MaxPacketSize {
			m.Close(fmt.Errorf("read data len meta > max-packet-size"))
			return
		}
		if dataLen == 0 {
			m.Close(fmt.Errorf("invalid data len: %d", dataLen))
			return
		}

		//read data
		pkt := &Packet{Type: PacketTypeResponse, Status: status, Path: pathStr, ChannelId: channelId, Data: make([]byte, dataLen), channel: channel}
		if _, err = io.ReadFull(bufReader, pkt.Data); err != nil {
			log.Errorf("read data fail, %s", err.Error())
			m.Close(err)
			return
		}
		channel.PacketStatus = status
		channel.ReceivedQueue <- pkt
	}
}
