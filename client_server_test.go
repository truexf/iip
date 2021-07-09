package iip

import (
	"fmt"
	"testing"
	"time"
)

type EchoClientHandlerTest struct {
}

func (m *EchoClientHandlerTest) Handle(c *Channel, path string, responseData []byte, dataCompleted bool) ([]byte, error) {
	return nil, nil
}

func BenchmarkEchoClientServer(t *testing.B) {
	c := make(chan error, 3)
	for i := 0; i < 3; i++ {
		go func() {
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
				c <- fmt.Errorf("connect server fail")
				return
			}
			client.RegisterHandler("/echo_benchmark", &EchoClientHandlerTest{})
			channel, err := client.NewChannel()
			if err != nil {
				c <- fmt.Errorf("new channel fail")
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

			bts, err := channel.DoRequest("/echo_benchmark", echoData, time.Second)
			if err != nil {
				c <- err
				return
			}
			if !SameBytes(bts, echoData) {
				c <- fmt.Errorf("response not same as request")
				return
			}
			c <- nil
		}()
	}
	for i := 0; i < 3; i++ {
		e := <-c
		if e != nil {
			t.Fatalf("fail")
		}
	}
}
