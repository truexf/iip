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
	LogClosing = false
	for i := 0; i < t.N; i++ {
		//同时3个并发，测试channel是否在并发情况下是否有串扰
		c := make(chan error, 3)
		for j := 0; j < 3; j++ {
			go func() {
				client, err := NewClient(ClientConfig{
					MaxConnections:        1000,
					MaxChannelsPerConn:    10,
					ChannelPacketQueueLen: 1000,
					TcpWriteQueueLen:      1000,
					TcpReadBufferSize:     16 * 1024 * 1024,
					TcpWriteBufferSize:    16 * 1024 * 1024,
					TcpConnectTimeout:     time.Second * 3,
				}, ":9090", nil)
				if err != nil {
					c <- fmt.Errorf("connect server fail")
					return
				}
				client.RegisterHandler("/echo_benchmark", &EchoClientHandlerTest{})
				channel, err := client.NewChannel()
				if err != nil {
					c <- fmt.Errorf("new channel fail, %s", err.Error())
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
					c <- fmt.Errorf(err.Error())
					return
				}
				if !bytes.Equal(bts, echoData) {
					c <- fmt.Errorf("response not same as request")
					return
				}
				channel.Close(nil)
				client.Close()
				c <- nil
			}()
		}
		for j := 0; j < 3; j++ {
			err := <-c
			if err != nil {
				t.Fatalf(err.Error())
			}
		}
	}
}
