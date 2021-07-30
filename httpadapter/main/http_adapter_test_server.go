package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/truexf/iip"
	"github.com/truexf/iip/httpadapter"
)

func main() {
	iip.LogClosing = false
	go StartIIPServer()
	go StartAdapterServer()
	ch := make(chan int)
	<-ch
}

type EchoServerHandler struct {
}

func (m *EchoServerHandler) Handle(path string, queryParams url.Values, requestData []byte, dataCompleted bool) ([]byte, error) {
	if path == "/echo" {
		return requestData, nil
	} else if path == "/http_header_echo" {
		hd, err := iip.IIPArgToHttpHeader(queryParams.Get(iip.ArgHttpHeader))
		if err != nil {
			return []byte(err.Error()), nil
		} else {
			if bts, err := json.Marshal(hd); err == nil {
				return bts, nil
			} else {
				return []byte(err.Error()), nil
			}
		}
	}
	return nil, fmt.Errorf("path %s not support", path)

}
func StartIIPServer() {
	server, err := iip.NewServer(iip.ServerConfig{
		MaxConnections:        10000,
		MaxChannelsPerConn:    10,
		ChannelPacketQueueLen: 1000,
		TcpWriteQueueLen:      1000,
		TcpReadBufferSize:     16 * 1024,
		TcpWriteBufferSize:    16 * 1024,
	}, ":9093", nil)

	if err != nil {
		panic(err.Error())
	}
	echoHandler := &EchoServerHandler{}
	server.RegisterHandler("/echo", echoHandler, nil)
	server.RegisterHandler("/http_header_echo", echoHandler, nil)
	if err := server.StartListen(); err != nil {
		panic(err.Error())
	}
}

func StartAdapterServer() {
	adapterServer, err := httpadapter.NewHttpAdapterServer(":9092", "", "", time.Second)
	if err != nil {
		panic(err.Error())
	}
	if err := adapterServer.RegisterBackend("echo_server", httpadapter.IipBackendConfig{ServerList: ":9093#1,:9093#1", ServerKeepConns: 10, ServerMaxConns: 10}); err != nil {
		panic(err.Error())
	}
	if err := adapterServer.PushRouteTail("localhost", "^/echo", "echo_server"); err != nil {
		panic(err.Error())
	}
	if err := adapterServer.PushRouteTail("localhost", "^/http_header", "echo_server"); err != nil {
		panic(err.Error())
	}
	if err := adapterServer.ListenAndServe(); err != nil {
		panic(err.Error())
	}
}
