// Copyright 2021 fangyousong(方友松). All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

//echo client
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/truexf/iip"
)

type EchoClientHandler struct {
}

func (m *EchoClientHandler) Handle(path string, request iip.Request, responseData []byte, dataCompleted bool) error {
	fmt.Printf("response in handler: %s\n for request: %s\n", string(responseData), string(request.Data()))

	return nil
}

var (
	certFileClient = flag.String("cert", "", "echo_client -cert certFile -key keyFile")
	keyFileClient  = flag.String("key", "", "echo_client -cert certFile -key keyFile")
	tls            = flag.String("tls", "0", "-tls=1")
)

func main() {
	fmt.Println("connect server: 9090")
	var client *iip.Client
	var err error
	flag.Parse()
	if *certFileClient != "" && *keyFileClient != "" {
		fmt.Printf("certfile: %s, keyfile: %s\n", *certFileClient, *keyFileClient)
		client, err = iip.NewClientTLS(
			iip.ClientConfig{
				MaxConnections:        1000,
				MaxChannelsPerConn:    10,
				ChannelPacketQueueLen: 1000,
				TcpWriteQueueLen:      1000,
				TcpReadBufferSize:     16 * 1024,
				TcpWriteBufferSize:    16 * 1024,
				TcpConnectTimeout:     time.Second * 3,
			},
			":9090",
			nil,
			*certFileClient,
			*keyFileClient,
		)
	} else if *tls != "0" {
		fmt.Println("new tls client")
		client, err = iip.NewClientTLS(
			iip.ClientConfig{
				MaxConnections:        1000,
				MaxChannelsPerConn:    10,
				ChannelPacketQueueLen: 1000,
				TcpWriteQueueLen:      1000,
				TcpReadBufferSize:     16 * 1024,
				TcpWriteBufferSize:    16 * 1024,
				TcpConnectTimeout:     time.Second * 3,
			},
			":9090",
			nil,
			"",
			"",
		)
	} else {
		client, err = iip.NewClient(
			iip.ClientConfig{
				MaxConnections:        1000,
				MaxChannelsPerConn:    10,
				ChannelPacketQueueLen: 1000,
				TcpWriteQueueLen:      1000,
				TcpReadBufferSize:     16 * 1024,
				TcpWriteBufferSize:    16 * 1024,
				TcpConnectTimeout:     time.Second * 3,
			},
			":9090",
			nil,
		)
	}
	if err != nil {
		fmt.Printf("new client fail, %s\n", err.Error())
		return
	}
	// 这里并非必要，只是展示Handler作为一个响应回调处理的功能，client handler一般一般用于流式(分段)响应的应用场景，见Client.DoStreamRequest
	client.RegisterHandler("/echo", &EchoClientHandler{})
	channel, err := client.NewChannel()
	if err != nil {
		fmt.Printf("new channel fail, %s\n", err.Error())
		return
	}
	fmt.Println("input some words:")
	lineReader := bufio.NewScanner(os.Stdin)
	for {
		if !lineReader.Scan() {
			break
		}
		response, err := channel.DoRequest("/echo", iip.NewDefaultRequest(lineReader.Bytes()), time.Second)
		if err != nil {
			fmt.Println(err.Error())
		} else {
			fmt.Printf("response ret: %s\n", string(response))
		}
		fmt.Println("input some words:")
	}

}
