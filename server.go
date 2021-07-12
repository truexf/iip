// Copyright 2021 fangyousong(方友松). All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

//服务器实现
package iip

import (
	"crypto/rand"
	"crypto/tls"
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

	handler *serverHandler
}

func NewServer(config ServerConfig, listenAddr string) (*Server, error) {
	ret := &Server{
		config:      config,
		listenAddr:  listenAddr,
		connections: make(map[string]*Connection),
		handler:     &serverHandler{pathHandlerManager: &ServerPathHandlerManager{}},
	}
	return ret, nil
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

func (m *Server) RegisterHandler(path string, handler ServerPathHandler) error {
	return m.handler.pathHandlerManager.registerHandler(path, handler)
}

func (m *Server) UnRegisterHandler(path string) {
	m.handler.pathHandlerManager.unRegisterHandler(path)
}
