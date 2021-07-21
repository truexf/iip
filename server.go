// Copyright 2021 fangyousong(方友松). All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

//服务器实现
package iip

import (
	"crypto/rand"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"sync"
	"time"
)

type ServerConfig struct {
	MaxConnections        int
	MaxChannelsPerConn    int
	ChannelPacketQueueLen uint32
	TcpWriteQueueLen      uint32
	TcpReadBufferSize     int
	TcpWriteBufferSize    int
}

type Server struct {
	DefaultErrorHolder
	DefaultContext
	isTls       bool
	tlsCertFile string
	tlsKeyFile  string
	config      ServerConfig
	listenAddr  string
	tcpListener net.Listener
	connections map[string]*Connection //key: remote addr for client
	connLock    sync.Mutex
	closeNotify chan int
	handler     *serverHandler

	//statis
	count       *Count
	measure     *Measure
	pathCount   map[string]*Count   //key: path
	pathMeasure map[string]*Measure //key: path
	statisLock  sync.RWMutex
}

func NewServer(config ServerConfig, listenAddr string, timeCountRangeFunc EnsureTimeRangeFunc) (*Server, error) {
	ret := &Server{
		config:      config,
		listenAddr:  listenAddr,
		connections: make(map[string]*Connection),
		handler:     &serverHandler{pathHandlerManager: &ServerPathHandlerManager{}},
		count:       &Count{},
		measure:     NewMesure(timeCountRangeFunc),
		pathCount:   make(map[string]*Count),
		pathMeasure: make(map[string]*Measure),
	}

	ret.RegisterHandler(PathServerCountJson, ret, EnsureTimeRangeMicroSecond)
	ret.RegisterHandler(PathServerMeasureJson, ret, EnsureTimeRangeMicroSecond)
	ret.RegisterHandler(PathServerPathCountJson, ret, EnsureTimeRangeMicroSecond)
	ret.RegisterHandler(PathServerPathMeasureJson, ret, EnsureTimeRangeMicroSecond)
	ret.RegisterHandler(PathServerStatis, ret, EnsureTimeRangeMicroSecond)
	ret.RegisterHandler(PathServerConnectionStatis, ret, EnsureTimeRangeMicroSecond)

	return ret, nil
}

func (m *Server) GetListener() net.Listener {
	return m.tcpListener
}

func (m *Server) AddCount(path string, count Count) {
	m.statisLock.RLock()
	defer m.statisLock.RLock()
	if cnt, ok := m.pathCount[path]; ok {
		cnt.Add(count)
		m.count.Add(count)
	}
}

func (m *Server) AddMeasure(path string, reqCount int64, duration time.Duration) {
	m.statisLock.RLock()
	defer m.statisLock.RLock()
	if me, ok := m.pathMeasure[path]; ok {
		me.Add(reqCount, duration)
		m.measure.Add(reqCount, duration)
	}
}

func (m *Server) acceptConn() (*Connection, error) {
	for {
		netConn, err := m.tcpListener.Accept()
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Temporary() {
				time.Sleep(time.Second)
				continue
			} else {
				return nil, err
			}
		}
		m.connLock.Lock()
		if len(m.connections) >= m.config.MaxConnections {
			netConn.Close()
			log.Errorf("accept connection fail, %s", ErrServerConnectionsLimited.Error())
			m.connLock.Unlock()
			continue
		}
		if conn, err := NewConnection(nil, m, netConn, RoleServer, int(m.config.TcpWriteQueueLen)); err == nil {
			m.connections[netConn.RemoteAddr().String()] = conn
			conn.SetCtxData(CtxServer, m)
			m.connLock.Unlock()
			return conn, nil
		} else {
			netConn.Close()
			m.connLock.Unlock()
			return nil, err
		}
	}
}

func (m *Server) removeConn(addr string) {
	log.Logf("connection: %s disconnected.", addr)
	m.connLock.Lock()
	defer m.connLock.Unlock()
	delete(m.connections, addr)
}

func (m *Server) Listener() net.Listener {
	return m.tcpListener
}

func (m *Server) Serve(listener net.Listener, isTls bool) error {
	m.tcpListener = listener
	m.isTls = isTls
	m.closeNotify = make(chan int)

	go func() {
		for {
			select {
			case <-m.closeNotify:
				return
			default:
				if conn, err := m.acceptConn(); err != nil {
					m.Stop(fmt.Errorf("accept connection fail, %s", err.Error()))
					return
				} else {
					log.Logf("accepted new connection: %s", conn.tcpConn.RemoteAddr().String())
				}
			}
		}
	}()

	return nil
}

// listen socket and start server process
func (m *Server) StartListen() error {
	lsn, err := net.Listen("tcp4", m.listenAddr)
	if err != nil {
		return err
	}
	m.Serve(lsn, false)
	return nil
}

// listen socket and start server process in TLS mode
func (m *Server) StartListenTLS(certFile, keyFile string) error {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return err
	}
	config := tls.Config{Certificates: []tls.Certificate{cert}}
	config.Rand = rand.Reader
	listener, err := tls.Listen("tcp4", m.listenAddr, &config)
	if err != nil {
		return err
	}
	m.isTls = true
	m.tlsCertFile = certFile
	m.tlsKeyFile = keyFile

	m.Serve(listener, true)

	return nil
}

//stop server
func (m *Server) Stop(err error) {
	close(m.closeNotify)
	if err != nil {
		log.Errorf("server stopped, %s", err.Error())
	} else {
		log.Logf("server stopped")
	}
	m.SetError(err)
	m.tcpListener.Close()

	m.connLock.Lock()
	defer m.connLock.Unlock()
	for _, conn := range m.connections {
		if conn.tcpConn != nil {
			conn.tcpConn.Close()
		}
	}
	m.connections = make(map[string]*Connection)
}

func (m *Server) RegisterHandler(path string, handler ServerPathHandler, timeCountRangeFunc EnsureTimeRangeFunc) error {
	ret := m.handler.pathHandlerManager.registerHandler(path, handler)
	m.statisLock.Lock()
	defer m.statisLock.Unlock()
	m.pathCount[path] = &Count{}
	m.pathMeasure[path] = NewMesure(timeCountRangeFunc)
	return ret
}

func (m *Server) UnRegisterHandler(path string) {
	m.handler.pathHandlerManager.unRegisterHandler(path)
}

func (m *Server) GetConnectionStatis() (respData []byte, e error) {
	conns := make(map[string]*Connection)
	m.connLock.Lock()
	for k, v := range m.connections {
		conns[k] = v
	}
	m.connLock.Unlock()

	ret := make(map[string]*ConnectionSatis)
	for k, v := range conns {
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
		ret[k] = rec
	}
	return json.Marshal(&ret)
}

// requestData format:
// {"time_unit": "microsecond|millisecond|second|nanosecond"}
func (m *Server) GetStatis(timeUnitJson []byte) (respData []byte, e error) {
	m.statisLock.RLock()
	defer m.statisLock.RUnlock()
	pathList := make([]string, 0, len(m.pathCount))
	for path := range m.pathCount {
		pathList = append(pathList, path)
	}

	// 性能统计信息中请求处理耗时的时间单位，默认以微秒计
	tmUnit := time.Microsecond
	reqMap := make(map[string]interface{})
	if err := json.Unmarshal(timeUnitJson, &reqMap); err == nil {
		if ut, ok := reqMap["time_unit"]; ok {
			if utStr, ok := ut.(string); ok {
				if utStr == "microsecond" {
					tmUnit = time.Microsecond
				} else if utStr == "millisecond" {
					tmUnit = time.Millisecond
				} else if utStr == "second" {
					tmUnit = time.Second
				} else if utStr == "nanosecond" {
					tmUnit = time.Nanosecond
				}
			}
		}
	}

	ret := ""
	bts, _ := json.Marshal(m.count)
	ret += fmt.Sprintf("total count: %s\n", string(bts))
	ret += fmt.Sprintf("total measure: \n%s\n-----------------------------------\n", m.measure.String(tmUnit))
	for _, v := range pathList {
		cnt := m.pathCount[v]
		mea := m.pathMeasure[v]
		bts, _ := json.Marshal(cnt)
		ret += fmt.Sprintf("%s count: %s\n", v, string(bts))
		ret += fmt.Sprintf("%s measure: \n%s\n\n", v, mea.String(tmUnit))
	}
	return []byte(ret), nil
}

func (m *Server) Handle(path string, queryParams url.Values, requestData []byte, dataCompleted bool) (respData []byte, e error) {
	switch path {
	case PathServerCountJson:
		return json.Marshal(m.count)
	case PathServerMeasureJson:
		return m.measure.Json(), nil
	case PathServerPathCountJson:
		if !dataCompleted {
			return nil, nil
		}
		m.statisLock.RLock()
		defer m.statisLock.RUnlock()
		if cnt, ok := m.pathCount[string(requestData)]; ok {
			return json.Marshal(cnt)
		}
	case PathServerPathMeasureJson:
		if !dataCompleted {
			return nil, nil
		}
		m.statisLock.RLock()
		defer m.statisLock.RUnlock()
		if ms, ok := m.pathMeasure[string(requestData)]; ok {
			return ms.Json(), nil
		}
	case PathServerStatis:
		if !dataCompleted {
			return nil, nil
		}
		return m.GetStatis(requestData)
	case PathServerConnectionStatis:
		return m.GetConnectionStatis()
	}
	return nil, fmt.Errorf("path [%s] not support", path)
}
