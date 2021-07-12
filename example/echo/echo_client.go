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
)

func main() {
	fmt.Println("connect server: 9090")
	var client *iip.Client
	var err error
	flag.Parse()
	fmt.Printf("certfile: %s, keyfile: %s\n", *certFileClient)
	if *certFileClient != "" && *keyFileClient != "" {
		client, err = iip.NewClientTLS(
			iip.ClientConfig{
				MaxConnections:        1000,
				MaxChannelsPerConn:    10,
				ChannelPacketQueueLen: 1000,
				TcpWriteQueueLen:      1000,
				TcpReadBufferSize:     16 * 1024 * 1024,
				TcpWriteBufferSize:    16 * 1024 * 1024,
				TcpConnectTimeout:     time.Second * 3,
			},
			":9090",
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
				TcpReadBufferSize:     16 * 1024 * 1024,
				TcpWriteBufferSize:    16 * 1024 * 1024,
				TcpConnectTimeout:     time.Second * 3,
			},
			":9090",
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
