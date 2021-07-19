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
	"strings"
	"sync"
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
)

func main() {
	LoadBalanceClientDemo()
	return

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

func LoadBalanceClientDemo() {
	iip.GetLogger().SetLevel(iip.LogLevelDebug)
	// lbc, err := iip.NewLoadBalanceClient(iip.ClientConfig{
	// 	MaxConnections:        100,
	// 	MaxChannelsPerConn:    100,
	// 	ChannelPacketQueueLen: 1000,
	// 	TcpWriteQueueLen:      1000,
	// 	TcpReadBufferSize:     16 * 1024,
	// 	TcpWriteBufferSize:    16 * 1024,
	// 	TcpConnectTimeout:     time.Second * 3,
	// }, ":9090#1,:9090#2,:9090#3")
	// if err != nil {
	// 	fmt.Printf("new lbc fail,%s", err.Error())
	// 	return
	// }
	lbc, err := iip.NewClient(iip.ClientConfig{
		MaxConnections:        100,
		MaxChannelsPerConn:    100,
		ChannelPacketQueueLen: 1000,
		TcpWriteQueueLen:      1000,
		TcpReadBufferSize:     16 * 1024,
		TcpWriteBufferSize:    16 * 1024,
		TcpConnectTimeout:     time.Second * 3,
	}, ":9090", nil)
	if err != nil {
		os.Stdout.WriteString(err.Error())
		return
	}

	fmt.Println("new lbc ok")
	echoData := `1testtesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttest
					1testtesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttest
					1testtesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttest
					1testtesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttest
					1testtesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttest
					1testtesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttest
					1testtesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttest
					1testtesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttest
					1testtesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttest
					1testtesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttest
					1testtesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttest
					1testtesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttest` + fmt.Sprintf("%d", time.Now().UnixNano())
	// echoData = strings.Dup(echoData, 10)
	echoData = strings.Repeat(echoData, 10)
	var wg sync.WaitGroup
	wg.Add(50)
	os.Stdout.WriteString(time.Now().String() + "\n")
	for i := 0; i < 50; i++ {
		go func() {
			defer wg.Done()
			ch, err := lbc.NewChannel()
			if err != nil {
				os.Stdout.WriteString(err.Error() + "\n")
				return
			}
			for i := 0; i < 10; i++ {
				bts, err := ch.DoRequest("/echo", iip.NewDefaultRequest([]byte(echoData)), time.Second)
				if err != nil {
					os.Stdout.WriteString(fmt.Sprintf("err: %s\n", err.Error()))
				} else {
					if string(bts) != echoData {
						panic("not equal")
					}
				}
			}
		}()
	}
	wg.Wait()
	os.Stdout.WriteString(time.Now().String() + "\n")
	os.Stdout.Sync()
	time.Sleep(time.Second)
}
