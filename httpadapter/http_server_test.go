// Copyright 2021 fangyousong(方友松). All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package httpadapter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/truexf/iip"
)

func TestHttpAdapter(t *testing.T) {
	go startIIPServer()
	go startAdapterServer()
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

	//test httpheader adapter
	{
		req, _ := http.NewRequest("GET", "http://localhost:9092/http_header_echo", nil)
		req.Header.Set("test-header-name", "test header value")
		reqHeaderJson, _ := json.Marshal(req.Header)
		fmt.Printf("request header: \n%s\n", reqHeaderJson)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf(err.Error())
		}
		ret, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf(err.Error())
		}
		fmt.Printf("response header: \n%s\n", ret)
		retHdr := make(http.Header)
		if err := json.Unmarshal(ret, &retHdr); err != nil {
			t.Fatalf(err.Error())
		} else {
			if retHdr.Get("test-header-name") != req.Header.Get("test-header-name") {
				t.Fatalf("response header not equal to request")
			}
		}
	}

}

type EchoServerHandler struct {
}

func (m *EchoServerHandler) Handle(path string, queryParams url.Values, requestData []byte, dataCompleted bool) ([]byte, error) {
	if path == "/echo" {
		return requestData, nil
	} else if path == "/http_header_echo" {
		hd, err := iip.IIPArgToHttpHeader(queryParams.Get(iip.ArgHttpHeader))
		if err != nil {
			return []byte(err.Error()), nil
		} else {
			if bts, err := json.Marshal(hd); err == nil {
				return bts, nil
			} else {
				return []byte(err.Error()), nil
			}
		}
	}
	return nil, fmt.Errorf("path %s not support", path)

}
func startIIPServer() {
	server, err := iip.NewServer(iip.ServerConfig{
		MaxConnections:        10000,
		MaxChannelsPerConn:    10,
		ChannelPacketQueueLen: 1000,
		TcpWriteQueueLen:      1000,
		TcpReadBufferSize:     16 * 1024,
		TcpWriteBufferSize:    16 * 1024,
	}, ":9093", nil)

	if err != nil {
		panic(err.Error())
	}
	echoHandler := &EchoServerHandler{}
	server.RegisterHandler("/echo", echoHandler, nil)
	server.RegisterHandler("/http_header_echo", echoHandler, nil)
	if err := server.StartListen(); err != nil {
		panic(err.Error())
	}
}

func startAdapterServer() {
	adapterServer, err := NewHttpAdapterServer(":9092", "", "", time.Second)
	if err != nil {
		panic(err.Error())
	}
	if err := adapterServer.RegisterBackend("echo_server", IipBackendConfig{ServerList: ":9093#1,:9093#1", ServerKeepConns: 10, ServerMaxConns: 10}); err != nil {
		panic(err.Error())
	}
	if err := adapterServer.PushRouteTail("localhost", "^/echo", "echo_server"); err != nil {
		panic(err.Error())
	}
	if err := adapterServer.PushRouteTail("localhost", "^/http_header", "echo_server"); err != nil {
		panic(err.Error())
	}
	if err := adapterServer.ListenAndServe(); err != nil {
		panic(err.Error())
	}
}
