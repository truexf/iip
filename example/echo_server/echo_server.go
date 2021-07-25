// Copyright 2021 fangyousong(方友松). All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"

	"github.com/truexf/iip"
)

type EchoServerHandler struct {
}

func (m *EchoServerHandler) Handle(path string, queryParams url.Values, requestData []byte, dataCompleted bool) ([]byte, error) {
	if path == "/echo" {
		if dataCompleted {
			// fmt.Printf("%s received: %s\n", path, string(requestData))
			return requestData, nil
		} else {
			return nil, iip.ErrPacketContinue
		}
	} else if path == "/echo_benchmark" {
		return requestData, nil
	}
	return nil, fmt.Errorf("path %s not support", path)

}

var (
	certFile = flag.String("cert", "", "echo_server -cert certFile -key keyFile")
	keyFile  = flag.String("key", "", "echo_server -cert certFile -key keyFile")
)

func main() {
	go startNetHttpServer()
	fmt.Println("start listen at :9090")
	server, err := iip.NewServer(iip.ServerConfig{
		MaxConnections:        10000,
		MaxChannelsPerConn:    10,
		ChannelPacketQueueLen: 1000,
		TcpWriteQueueLen:      1000,
		TcpReadBufferSize:     16 * 1024,
		TcpWriteBufferSize:    16 * 1024,
	}, ":9090", nil)

	if err != nil {
		fmt.Println(err.Error())
	}
	echoHandler := &EchoServerHandler{}
	server.RegisterHandler("/echo", echoHandler, nil)
	server.RegisterHandler("/echo_benchmark", echoHandler, nil)
	flag.Parse()
	tp := ""
	if *certFile != "" && *keyFile != "" {
		fmt.Printf("certfile: %s, keyfile: %s\n", *certFile, *keyFile)
		err = server.StartListenTLS(*certFile, *keyFile)
		tp = "tls"
	} else {
		err = server.StartListen()
	}
	if err != nil {
		fmt.Println(err.Error())
	}
	fmt.Printf("%s server started success.\n", tp)
	c := make(chan int)
	<-c
}

func startNetHttpServer() {
	fmt.Println("start listen at :9091")
	echoHandler := func(w http.ResponseWriter, req *http.Request) {
		if bts, err := ioutil.ReadAll(req.Body); err == nil {
			w.Write(bts)
		} else {
			io.WriteString(w, err.Error())
		}

	}

	http.HandleFunc("/echo_benchmark", echoHandler)
	log.Fatal(http.ListenAndServe(":9091", nil))
	fmt.Println("net/http server started success.")
	c := make(chan int)
	<-c
}
