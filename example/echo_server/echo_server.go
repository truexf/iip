// Copyright 2021 fangyousong(方友松). All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"

	"github.com/truexf/iip"
)

type EchoServerHandler struct {
}

func (m *EchoServerHandler) Handle(path string, requestData []byte, dataCompleted bool) ([]byte, error) {
	if path == "/echo" {
		if dataCompleted {
			fmt.Printf("%s received: %s\n", path, string(requestData))
			return requestData, nil
		} else {
			return nil, iip.ErrPacketContinue
		}
	} else if path == "/echo_benchmark" {
		return requestData, nil
	}
	return nil, fmt.Errorf("path %s not support", path)

}

var (
	certFile = flag.String("cert", "", "echo_server -cert certFile -key keyFile")
	keyFile  = flag.String("key", "", "echo_server -cert certFile -key keyFile")
)

func main() {
	fmt.Println("start listen at :9090")
	server, err := iip.NewServer(iip.ServerConfig{
		MaxConnections:        1000,
		MaxChannelsPerConn:    10,
		ChannelPacketQueueLen: 1000,
		TcpWriteQueueLen:      1000,
		TcpReadBufferSize:     16 * 1024 * 1024,
		TcpWriteBufferSize:    16 * 1024 * 1024,
	}, ":9090", nil)

	if err != nil {
		fmt.Println(err.Error())
	}
	echoHandler := &EchoServerHandler{}
	server.RegisterHandler("/echo", echoHandler, nil)
	server.RegisterHandler("/echo_benchmark", echoHandler, nil)
	flag.Parse()
	tp := ""
	if *certFile != "" && *keyFile != "" {
		fmt.Printf("certfile: %s, keyfile: %s\n", *certFile, *keyFile)
		err = server.StartListenTLS(*certFile, *keyFile)
		tp = "tls"
	} else {
		err = server.StartListen()
	}
	if err != nil {
		fmt.Println(err.Error())
	}
	fmt.Printf("%s server started success.\n", tp)
	c := make(chan int)
	<-c
}
