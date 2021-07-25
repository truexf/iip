// Copyright 2021 fangyousong(方友松). All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package iip

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"
	"time"
)

type EchoClientHandlerTest struct {
}

func (m *EchoClientHandlerTest) Handle(path string, request Request, responseData []byte, dataCompleted bool) error {
	return nil
}

var (
	pfNetHttpServerAddr = flag.String("naddr", ":9091", "")
	pfIIPServerAddr     = flag.String("iipaddr", ":9090", "")
)

//跑这个测试前须先在9090端口启动echo_server, echo_server在example/echo_server/echo_server.go
func BenchmarkPFEchoClientServer(t *testing.B) {
	flag.Parse()
	LogClosing = false
	client, err := NewClient(ClientConfig{
		MaxConnections:        1000,
		MaxChannelsPerConn:    10,
		ChannelPacketQueueLen: 1000,
		TcpWriteQueueLen:      1000,
		TcpReadBufferSize:     16 * 1024,
		TcpWriteBufferSize:    16 * 1024,
		TcpConnectTimeout:     time.Second * 3,
	}, *pfIIPServerAddr, nil)
	if err != nil {
		t.Fatalf("connect server fail")
		return
	}
	client.RegisterHandler("/echo_benchmark", &EchoClientHandlerTest{})
	channel, err := client.NewChannel()
	if err != nil {
		t.Fatalf("new channel fail, %s", err.Error())
		return
	}
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
	echoData = strings.Repeat(echoData, 10)
	t.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			bts, err := channel.DoRequest("/echo_benchmark", NewDefaultRequest([]byte(echoData)), time.Second)
			if err != nil {
				t.Fatalf(err.Error())
			}
			if !bytes.Equal(bts, []byte(echoData)) {
				t.Fatalf("response not same as request")
			}
		}
	})
}

func BenchmarkPFEchoNetHttp(t *testing.B) {
	flag.Parse()
	LogClosing = false
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
	echoData = strings.Repeat(echoData, 10)
	t.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			resp, err := http.Post(fmt.Sprintf("http://%s/echo_benchmark", *pfNetHttpServerAddr), "application/octet-stream", bytes.NewReader([]byte(echoData)))
			if err != nil {
				t.Fatalf(err.Error())
			}
			if bts, err := ioutil.ReadAll(resp.Body); err == nil {
				if !bytes.Equal(bts, bts) {
					t.Fatalf("response not same as request")
				}
			}
		}
	})
}
