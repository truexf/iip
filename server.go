package iip

import (
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
		handler:     &serverHandler{pathHandlerManager: &PathHandlerManager{}},
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
		tcpConn := netConn.(*net.TCPConn)
		if conn, err := NewConnection(tcpConn, RoleServer, int(m.config.TcpWriteQueueLen)); err == nil {
			m.connLock.Lock()
			m.connections[tcpConn.RemoteAddr().String()] = conn
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

func (m *Server) Stop(err error) {
	log.Errorf("server stopped, %s", err.Error())
	m.SetError(err)
	m.tcpListener.Close()

	m.connLock.Lock()
	defer m.connLock.Unlock()
	for _, conn := range m.connections {
		conn.SetCtxData(CtxServer, nil)
		if conn.tcpConn != nil {
			conn.tcpConn.CloseWrite()
			conn.tcpConn.CloseRead()
			conn.tcpConn.Close()
		}
	}
	m.connections = make(map[string]*Connection)

	close(m.closeNotify)
}

func (m *Server) RegisterHandler(path string, handler PathHandler) error {
	return m.handler.pathHandlerManager.registerHandler(path, handler)
}

func (m *Server) UnRegisterHandler(path string) {
	m.handler.pathHandlerManager.unRegisterHandler(path)
}
