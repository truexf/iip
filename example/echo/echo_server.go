package main

import (
	"fmt"

	"github.com/truexf/iip"
)

type EchoServerHandler struct {
}

func (m *EchoServerHandler) Handle(path string, requestData []byte, dataCompleted bool) ([]byte, error) {
	if dataCompleted {
		fmt.Printf("%s received: %s\n", path, string(requestData))
		return requestData, nil
	} else {
		return nil, iip.ErrPacketContinue
	}
}

func main() {
	fmt.Println("start listen at :9090")
	server, err := iip.NewServer(iip.ServerConfig{
		MaxConnections:        1000,
		MaxChannelsPerConn:    10,
		ChannelPacketQueueLen: 1000,
		TcpWriteQueueLen:      1000,
		TcpReadBufferSize:     16 * 1024 * 1024,
		TcpWriteBufferSize:    16 * 1024 * 1024,
	}, ":9090")

	if err != nil {
		fmt.Println(err.Error())
	}
	server.RegisterHandler("/echo", &EchoServerHandler{})
	if err := server.StartListen(); err != nil {
		fmt.Println(err.Error())
	}
	fmt.Println("server started success.")
	c := make(chan int)
	<-c
}
