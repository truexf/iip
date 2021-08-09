// Copyright 2021 fangyousong(方友松). All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

//协议实现的核心代码
package iip

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"net"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/truexf/goutil"
)

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
	Type          byte   `json:"type"` //0 request, 4 response
	Status        byte   `json:"status"`
	Path          string `json:"path"`
	ChannelId     uint32 `json:"channel_id"`
	Data          []byte `json:"data"`
	DontChunk     bool   //禁止分成多个packet传输
	channel       *Channel
	params        url.Values
	parsePathLock sync.Mutex
}

func (m *Packet) parsePath() {
	m.parsePathLock.Lock()
	defer m.parsePathLock.Unlock()
	if m.params != nil {
		return
	}
	if u, err := url.Parse(m.Path); err != nil {
		m.params = make(url.Values)
	} else {
		m.params = u.Query()
		m.Path = u.Path
	}
}

func (m *Packet) PathParam(key string) string {
	if m.params == nil {
		m.parsePath()
	}
	return m.params.Get(key)
}

// 根据一个Packet对象创建一个用于tcp发送的网络数据包
func CreateNetPacket(pkt *Packet) ([]byte, error) {
	if len(pkt.Path) > int(MaxPathLen) {
		return nil, fmt.Errorf("path is too large, must be <= %d bytes", MaxPathLen)
	}
	if len(pkt.Data) > int(MaxPacketSize) {
		return nil, fmt.Errorf("data is too large, must be <= %d bytes", MaxPacketSize)
	}
	if pkt.Status != Status8 && pkt.Path == "" {
		return nil, fmt.Errorf("invalid path: %s", pkt.Path)
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

// 从网络数据中读取生成Packet对象
func ReadPacket(reader io.Reader) (*Packet, error) {
	bufReader := bufio.NewReaderSize(reader, int(PacketReadBufSize))
	btsChannelId := make([]byte, 4)
	btsDataLen := make([]byte, 4)
	//read status
	status, err := bufReader.ReadByte()
	if err != nil {
		return nil, fmt.Errorf("read error")
	}

	//read path
	path, err := bufReader.ReadSlice(0)
	if err != nil {
		return nil, fmt.Errorf("read error")
	}
	pathStr := string(path[:len(path)-1])

	//read channelID
	if _, err = io.ReadFull(bufReader, btsChannelId); err != nil {
		return nil, fmt.Errorf("read error")
	}
	channelId := binary.BigEndian.Uint32(btsChannelId)

	//read datalen
	if _, err = io.ReadFull(bufReader, btsDataLen); err != nil {
		return nil, fmt.Errorf("read error")
	}
	dataLen := binary.BigEndian.Uint32(btsDataLen)

	//read data
	pkt := &Packet{Type: PacketTypeResponse, Status: status, Path: pathStr, ChannelId: channelId, Data: make([]byte, dataLen)}
	if _, err = io.ReadFull(bufReader, pkt.Data); err != nil {
		return nil, fmt.Errorf("read error")
	}
	return pkt, nil
}

// 向网络发送Packet
func WritePacket(pkt *Packet, writer io.Writer) (int, error) {
	data, err := CreateNetPacket(pkt)
	if err != nil {
		return 0, err
	}
	n, err := writer.Write(data)
	if err != nil {
		log.Errorf("write packet fail, %s", err.Error())
		return n, err
	}
	if n != len(data) {
		return n, fmt.Errorf("writepacket not complete, totoal %d bytes, %d bytes writted. ", len(data), n)
	}
	if pkt.channel != nil {
		cnt := Count{BytesSent: int64(n)}
		pkt.channel.Count.Add(cnt)
		pkt.channel.conn.Count.Add(cnt)
		if pkt.channel.conn.Role == RoleClient {
			pkt.channel.conn.Client.Count.Add(cnt)
		} else {
			pkt.channel.conn.Server.AddCount(pkt.Path, cnt)
		}
	}
	return n, nil
}

// 检查来自client端的Packet的Status是否合法
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

// 检查来自server端的Packet的Status是否合法
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

// channel的实现
// chuannel由框架内部使用
type Channel struct {
	DefaultErrorHolder
	DefaultContext
	Id            uint32
	NewTime       time.Time
	sendLock      sync.Mutex
	conn          *Connection
	receivedQueue chan *Packet //received streamed packet from peer side
	packetStatus  byte         //recent received packet status
	closeNotify   chan int
	closedMark    uint32
	Count         *Count
}

func (m *Channel) SendPacket(pkt *Packet) error {
	if m.err != nil {
		return fmt.Errorf("current channel is invalid, %s", m.err.Error())
	}
	err := m.sendPacket(pkt)
	if err != nil {
		log.Errorf("send packet fail, %s, close connection", err.Error())
		m.Close(err)
		m.conn.Close(err)
	}
	return err
}

func (m *Channel) packetToSendQueue(pkt *Packet) error {
	select {
	case <-time.After(time.Second):
		return fmt.Errorf("push packet to sendQueue timeout")
	case m.conn.tcpWriteQueue <- pkt:
		return nil
	}
}

func (m *Channel) sendPacket(pkt *Packet) error {
	m.sendLock.Lock()
	defer m.sendLock.Unlock()
	if m.conn.Closed() {
		return fmt.Errorf("connect is closed")
	}

	if pkt.Status == Status8 {
		m.packetToSendQueue(pkt)
		return nil
	}

	if len(pkt.Data) <= int(MaxPacketSize) || pkt.DontChunk {
		if m.conn.Role == RoleClient {
			pkt.Status = StatusC1
		} else if m.conn.Role == RoleServer {
			pkt.Status = StatusS5
		}
		m.packetToSendQueue(pkt)

		cnt := Count{PacketsSent: 1, WholePacketSent: 1}
		m.Count.Add(cnt)
		m.conn.Count.Add(cnt)
		if m.conn.Role == RoleClient {
			m.conn.Client.Count.Add(cnt)
		} else {
			m.conn.Server.AddCount(pkt.Path, cnt)
		}
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
			if m.conn.Role == RoleClient {
				if firstSend {
					chunk.Status = StatusC1
				} else {
					chunk.Status = StatusC3
				}
			} else if m.conn.Role == RoleServer {
				if firstSend {
					chunk.Status = StatusS5
				} else {
					chunk.Status = StatusS7
				}
			} else {
				return fmt.Errorf("protocol error")
			}
		} else if chunkSize < remainDataSize {
			if m.conn.Role == RoleClient {
				if firstSend {
					chunk.Status = StatusC0
				} else {
					chunk.Status = StatusC2
				}
			} else if m.conn.Role == RoleServer {
				if firstSend {
					chunk.Status = StatusS4
				} else {
					chunk.Status = StatusS6
				}
			} else {
				return fmt.Errorf("protocol error")
			}
		} else {
			return fmt.Errorf("protocol error")
		}
		m.packetToSendQueue(pkt)
		cnt := Count{PacketsSent: 1}
		m.Count.Add(cnt)
		m.conn.Count.Add(cnt)
		if m.conn.Role == RoleClient {
			m.conn.Client.Count.Add(cnt)
		} else {
			m.conn.Server.AddCount(pkt.Path, cnt)
		}

		firstSend = false
		remainDataSize -= chunkSize
		if remainDataSize <= 0 {
			break
		}
	}

	cnt := Count{WholePacketSent: 1}
	m.Count.Add(cnt)
	m.conn.Count.Add(cnt)
	if m.conn.Role == RoleClient {
		m.conn.Client.Count.Add(cnt)
	} else {
		m.conn.Server.AddCount(pkt.Path, cnt)
	}
	return nil
}

func (m *Channel) handleServerLoop() {
	var pktWholeRequest *Packet
	handler := m.conn.GetCtxData(CtxServer).(*Server).handler
	for {
		select {
		case <-m.closeNotify:
			return
		case pkt := <-m.receivedQueue:
			if pkt.Status == Status8 {
				m.Close(fmt.Errorf("closed by peer command"))
				return
			}

			//merge
			if pktWholeRequest == nil {
				pktWholeRequest = pkt
			} else {
				pktWholeRequest.Data = append(pktWholeRequest.Data, pkt.Data...)
				pktWholeRequest.Status = pkt.Status
			}

			//handle
			handleBeginTime := time.Now()
			ret, err := handler.Handle(m, pktWholeRequest, isClientStatusCompleted(pktWholeRequest.Status))
			m.conn.Server.AddMeasure(pkt.Path, 1, time.Since(handleBeginTime))

			if isClientStatusCompleted(pktWholeRequest.Status) {
				if err != nil {
					log.Errorf("handle pkt %s fail, %s", pkt.Path, err.Error())
					err = ErrHandleError
				} else if len(ret) == 0 {
					log.Errorf("handle pkt %s fail, %s", pkt.Path, "no response data")
					err = ErrHandleNoResponse
				}
			} else {
				if err != nil {
					log.Errorf("handle pkt %s fail, %s", pkt.Path, err.Error())
					err = ErrHandleError
				}
			}

			// 有响应或发生错误
			if (err == nil && len(ret) > 0) || err != nil {
				retPkt := &Packet{
					Type:      PacketTypeResponse,
					Path:      pkt.Path,
					ChannelId: pkt.ChannelId,
					// Data:      ret,
					channel: m,
				}
				if err == nil {
					retPkt.Data = ret
				} else if err != nil {
					retPkt.Data = ErrorResponse(err.(*Error)).Data()
				}
				if err := m.SendPacket(retPkt); err != nil {
					log.Errorf("channel.SendPacket fail, %s", err.Error())
				}
			}

			if isClientStatusCompleted(pkt.Status) {
				pktWholeRequest = nil
				cnt := Count{WholePacketReceived: 1}
				m.Count.Add(cnt)
				m.conn.Count.Add(cnt)
				if m.conn.Role == RoleClient {
					m.conn.Client.Count.Add(cnt)
				} else {
					m.conn.Server.AddCount(pkt.Path, cnt)
				}

			}

		}
	}
}

func (m *Channel) handleClientLoop() {
	// merge 1 or 1+ packet into an whole response
	var pktWholeResponse *Packet
	handler := m.GetCtxData(CtxClient).(*Client).handler
	uncompletedReqQ := m.GetCtxData(CtxUncompletedRequestChan).(*goutil.LinkedList)
	tkt := time.NewTicker(time.Second)
	for {
		select {
		case <-tkt.C:
			if m.conn.Closed() {
				return
			}
		case <-m.closeNotify:
			return
		case pkt := <-m.receivedQueue:
			if pkt.Status == Status8 {
				m.Close(fmt.Errorf("closed by peer command"))
				return
			}

			//merge
			if pktWholeResponse == nil {
				pktWholeResponse = pkt
			} else {
				pktWholeResponse.Data = append(pktWholeResponse.Data, pkt.Data...)
				pktWholeResponse.Status = pkt.Status
			}

			//handle
			reqi := uncompletedReqQ.PopHead(true)
			if reqi == nil {
				m.Close(fmt.Errorf("request not found"))
				return
			}
			req := reqi.(Request)
			m.SetCtxData(CtxRequest, req)
			handleBeginTime := time.Now()
			_, err := handler.Handle(m, pktWholeResponse, isServerStatusCompleted(pkt.Status))
			m.conn.Client.Measure.Add(1, time.Since(handleBeginTime))
			if err != nil {
				log.Errorf("handle pkt %s fail, %s", pkt.Path, err.Error())
			}

			if isServerStatusCompleted(pkt.Status) {
				respCompletedChan := req.GetCtxData(CtxResponseChan)
				if respCompletedChan != nil {
					notifyChan := respCompletedChan.(chan []byte)
					select {
					case <-notifyChan:
					default:
						notifyChan <- pktWholeResponse.Data
					}
				}
				cnt := Count{WholePacketReceived: 1}
				m.Count.Add(cnt)
				m.conn.Count.Add(cnt)
				if m.conn.Role == RoleClient {
					m.conn.Client.Count.Add(cnt)
				} else {
					m.conn.Server.AddCount(pkt.Path, cnt)
				}
				pktWholeResponse = nil
			} else {
				uncompletedReqQ.PushHead(req, true)
			}
		}
	}
}

//关闭channel
func (m *Channel) Close(err error) {
	if !atomic.CompareAndSwapUint32(&m.closedMark, 0, 1) {
		return
	}
	// m.SendPacket(&Packet{Status: 8, ChannelId: m.Id, channel: m})
	m.conn.removeChannel(m)
	if err != nil {
		m.err = err
	} else {
		m.err = fmt.Errorf("unknown")
	}
	if LogClosing {
		log.Errorf("channel closed: %s", m.err.Error())
	}
	if m.closeNotify != nil {
		close(m.closeNotify)
		m.closeNotify = nil
	}
}

type Connection struct {
	DefaultErrorHolder
	DefaultContext
	Role          byte    //0 client, 4 server
	Client        *Client //not nil if client side
	Server        *Server //not nil if server side
	Channels      map[uint32]*Channel
	channelCount  int32
	MaxChannelId  uint32
	FreeChannleId map[uint32]struct{}
	ChannelsLock  sync.RWMutex
	tcpConn       net.Conn
	tcpWriteQueue chan *Packet
	closeNotify   chan int
	closedMark    uint32
	Count         *Count
}

// 创建一个Connection对象，由Client或Server内部调用
func NewConnection(client *Client, server *Server, netConn net.Conn, role byte, writeQueueLen int) (*Connection, error) {
	if role != RoleClient && role != RoleServer {
		return nil, fmt.Errorf("invalid role value")
	}
	ret := &Connection{
		Role:          role,
		Client:        client,
		Server:        server,
		Channels:      make(map[uint32]*Channel),
		FreeChannleId: make(map[uint32]struct{}),
		tcpConn:       netConn,
		tcpWriteQueue: make(chan *Packet, writeQueueLen),
		closeNotify:   make(chan int, 1),
		Count:         &Count{},
	}
	if client != nil {
		ret.SetCtxData(CtxClient, client)
	} else if server != nil {
		ret.SetCtxData(CtxServer, server)
	}
	ucrq := goutil.NewLinkedList(true)
	if role == RoleClient {
		c := ret.newChannel(true, 100, map[string]interface{}{CtxUncompletedRequestChan: ucrq, CtxClient: client}, nil)
		if c == nil {
			log.Errorf("client new sys channel fail")
		}
	} else {
		c := ret.newChannel(true, 100, nil, nil)
		if c == nil {
			log.Errorf("server new sys channel fail")
		}
	}
	if role == RoleClient {
		go ret.clientReadLoop()
	} else {
		go ret.serverReadLoop()
	}
	go ret.writeLoop()

	return ret, nil
}

func (m *Connection) Closed() bool {
	return atomic.LoadUint32(&m.closedMark) == 1
}

func (m *Connection) writeLoop() {
	for {
		select {
		case pkt := <-m.tcpWriteQueue:
			if m.Closed() {
				return
			}
			m.tcpConn.SetWriteDeadline(time.Now().Add(time.Second * 3))
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
	if !atomic.CompareAndSwapUint32(&m.closedMark, 0, 1) {
		return
	}
	defer func() {
		if m.tcpConn != nil {
			m.tcpConn.Close()
			m.tcpConn = nil
		}
	}()
	if err != nil {
		m.err = err
	} else {
		m.err = fmt.Errorf("unknown")
	}
	if LogClosing && m.tcpConn != nil {
		log.Errorf("connection closed, role %d, remote addr: %s, error: %s", m.Role, m.tcpConn.RemoteAddr().String(), m.err.Error())
	}

	svr := m.GetCtxData(CtxServer)
	if svr != nil {
		svr.(*Server).removeConn(m.tcpConn.RemoteAddr().String())
	} else {
		client := m.GetCtxData(CtxClient)
		if client != nil {
			client.(*Client).removeConnection(m)
		}
	}

	for _, v := range m.Channels {
		v.Close(fmt.Errorf("connection is closed"))
	}
	if m.closeNotify != nil {
		close(m.closeNotify)
		m.closeNotify = nil
	}

	//clean write chan
	for {
		select {
		case <-m.tcpWriteQueue:
		default:
			return
		}
	}
}

func (m *Connection) makeNewChannelId() uint32 {
	m.ChannelsLock.Lock()
	defer m.ChannelsLock.Unlock()
	var ret uint32 = 0
	if len(m.FreeChannleId) > 0 {
		for k := range m.FreeChannleId {
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

func (m *Connection) newChannel(sys bool, queueLen uint32, clientCtx map[string]interface{}, serverCtx map[string]interface{}) *Channel {
	ret := &Channel{
		Id:            0,
		NewTime:       time.Now(),
		conn:          m,
		receivedQueue: make(chan *Packet, queueLen),
		packetStatus:  255,
		closeNotify:   make(chan int, 1),
		Count:         &Count{},
	}
	if !sys {
		ret.Id = m.makeNewChannelId()
	}

	m.ChannelsLock.Lock()
	defer m.ChannelsLock.Unlock()
	m.Channels[ret.Id] = ret
	atomic.AddInt32(&m.channelCount, 1)
	if m.Role == RoleServer {
		for k, v := range serverCtx {
			ret.SetCtxData(k, v)
		}
		ret.SetCtxData(CtxServer, m.GetCtxData(CtxServer))
		go ret.handleServerLoop()
	} else if m.Role == RoleClient {
		for k, v := range clientCtx {
			ret.SetCtxData(k, v)
		}
		ret.SetCtxData(CtxClient, m.GetCtxData(CtxClient))
		go ret.handleClientLoop()
	}

	return ret
}

func (m *Connection) getChannel(channelId uint32) *Channel {
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
		atomic.AddInt32(&m.channelCount, -1)
		m.FreeChannleId[c.Id] = struct{}{}
	}
}

func (m *Connection) clientReadLoop() {
	//利用bufio，每次从内核多读一些数据上来处理，减少对内核内存的读次数
	bufReader := bufio.NewReaderSize(m.tcpConn, int(PacketReadBufSize))
	btsChannelId := make([]byte, 4)
	btsDataLen := make([]byte, 4)
	for {
		if m.Closed() {
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
		if err := CheckServerPacketStatus(channel.packetStatus, status); err != nil {
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
		channel.packetStatus = status
		cnt := Count{PacketReceived: 1, BytesReceived: int64(len(pkt.Data) + 1 + len(pkt.Path) + 1 + 4 + 4)}
		channel.Count.Add(cnt)
		channel.conn.Count.Add(cnt)
		if channel.conn.Role == RoleClient {
			channel.conn.Client.Count.Add(cnt)
		} else {
			channel.conn.Server.AddCount(pkt.Path, cnt)
		}
		channel.receivedQueue <- pkt
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
		if err := CheckClientPacketStatus(channel.packetStatus, status); err != nil {
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
		channel.packetStatus = status
		channel.Count.Add(Count{PacketReceived: 1, BytesReceived: int64(len(pkt.Data) + 1 + len(pkt.Path) + 1 + 4 + 4)})
		channel.receivedQueue <- pkt
	}
}
