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

	return ret, nil
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
		if conn, err := NewConnection(nil, m, netConn, RoleServer, int(m.config.TcpWriteQueueLen)); err == nil {
			m.connLock.Lock()
			m.connections[netConn.RemoteAddr().String()] = conn
			m.connLock.Unlock()
			conn.SetCtxData(CtxServer, m)
			return conn, nil
		} else {
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

// listen socket and start server process
func (m *Server) StartListen() error {
	lsn, err := net.Listen("tcp4", m.listenAddr)
	if err != nil {
		return err
	}
	m.tcpListener = lsn
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

	m.tcpListener = listener
	m.isTls = true
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

//stop server
func (m *Server) Stop(err error) {
	log.Errorf("server stopped, %s", err.Error())
	m.SetError(err)
	m.tcpListener.Close()

	m.connLock.Lock()
	defer m.connLock.Unlock()
	for _, conn := range m.connections {
		conn.SetCtxData(CtxServer, nil)
		if conn.tcpConn != nil {
			conn.tcpConn.Close()
		}
	}
	m.connections = make(map[string]*Connection)

	close(m.closeNotify)
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

func (m *Server) Handle(path string, requestData []byte, dataCompleted bool) (respData []byte, e error) {
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
	}
	return nil, fmt.Errorf("path [%s] not support", path)
}
