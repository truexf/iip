// Copyright 2021 fangyousong(方友松). All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package httpadapter

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/truexf/iip"
)

func TestHttpAdapter(t *testing.T) {
	go startIIPServer(t)
	go startAdapterServer(t)
	time.Sleep(time.Second * 2)
	bts := []byte("hello, http adapater\n")
	resp, err := http.Post("http://localhost:9092/echo", "", bytes.NewReader(bts))
	if err != nil {
		t.Fatalf(err.Error())
	}
	ret, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf(err.Error())
	}
	if !bytes.Equal(bts, ret) {
		t.Fatalf("request response not equal\n%s\n%s\n", string(bts), string(ret))
	}
}

type EchoServerHandler struct {
}

func (m *EchoServerHandler) Handle(path string, queryParams url.Values, requestData []byte, dataCompleted bool) ([]byte, error) {
	if path == "/echo" {
		return requestData, nil
	}
	return nil, fmt.Errorf("path %s not support", path)

}
func startIIPServer(t *testing.T) {
	server, err := iip.NewServer(iip.ServerConfig{
		MaxConnections:        10000,
		MaxChannelsPerConn:    10,
		ChannelPacketQueueLen: 1000,
		TcpWriteQueueLen:      1000,
		TcpReadBufferSize:     16 * 1024,
		TcpWriteBufferSize:    16 * 1024,
	}, ":9093", nil)

	if err != nil {
		t.Fatalf(err.Error())
	}
	echoHandler := &EchoServerHandler{}
	server.RegisterHandler("/echo", echoHandler, nil)
	if err := server.StartListen(); err != nil {
		t.Fatalf(err.Error())
	}
}

func startAdapterServer(t *testing.T) {
	adapterServer, err := NewHttpAdapterServer(":9092", "", "", time.Second)
	if err != nil {
		t.Fatalf(err.Error())
	}
	if err := adapterServer.RegisterBackend("echo_server", IipBackendConfig{ServerList: ":9093#1,:9093#1", ServerKeepConns: 10, ServerMaxConns: 10}); err != nil {
		t.Fatalf(err.Error())
	}
	if err := adapterServer.PushRouteTail("localhost", "^/echo", "echo_server"); err != nil {
		t.Fatalf(err.Error())
	}
	if err := adapterServer.ListenAndServe(); err != nil {
		t.Fatalf(err.Error())
	}
}
