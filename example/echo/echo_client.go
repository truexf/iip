package main

import (
	"bufio"
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

func main() {
	fmt.Println("connect server: 9090")
	client, err := iip.NewClient(iip.ClientConfig{
		MaxConnections:        1000,
		MaxChannelsPerConn:    10,
		ChannelPacketQueueLen: 1000,
		TcpWriteQueueLen:      1000,
		TcpReadBufferSize:     16 * 1024 * 1024,
		TcpWriteBufferSize:    16 * 1024 * 1024,
		TcpConnectTimeout:     time.Second * 3,
	}, ":9090")
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	client.RegisterHandler("/echo", &EchoClientHandler{})
	fmt.Println("server connected.")
	channel, err := client.NewChannel()
	if err != nil {
		fmt.Println(err.Error())
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
