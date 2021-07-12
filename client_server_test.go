package iip

import (
	"bytes"
	"fmt"
	"testing"
	"time"
)

type EchoClientHandlerTest struct {
}

func (m *EchoClientHandlerTest) Handle(path string, request Request, responseData []byte, dataCompleted bool) error {
	return nil
}

//跑这个测试前须先在9090端口启动echo_server, echo_server在example/echo_server.go
func BenchmarkEchoClientServer(t *testing.B) {
	for i := 0; i < t.N; i++ {
		client, err := NewClient(ClientConfig{
			MaxConnections:        1000,
			MaxChannelsPerConn:    10,
			ChannelPacketQueueLen: 1000,
			TcpWriteQueueLen:      1000,
			TcpReadBufferSize:     16 * 1024 * 1024,
			TcpWriteBufferSize:    16 * 1024 * 1024,
			TcpConnectTimeout:     time.Second * 3,
		}, ":9090")
		if err != nil {
			t.Fatalf("connect server fail")
			return
		}
		client.RegisterHandler("/echo_benchmark", &EchoClientHandlerTest{})
		channel, err := client.NewChannel()
		if err != nil {
			t.Fatalf("new channel fail")
			return
		}
		echoData := []byte(`1testtesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttest
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
			1testtesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttesttest` + fmt.Sprintf("%d", time.Now().UnixNano()))

		bts, err := channel.DoRequest("/echo_benchmark", NewDefaultRequest(echoData), time.Second)
		if err != nil {
			t.Fatalf(err.Error())
			return
		}
		if !bytes.Equal(bts, echoData) {
			t.Fatalf("response not same as request")
			return
		}
		channel.Close(nil)
		client.Close()
	}
}
