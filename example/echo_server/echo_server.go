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
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/truexf/iip"
	"github.com/truexf/iip/httpadapter"
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
	//became a daemon process
	if err := daemonize(); err != nil {
		os.Stdout.WriteString(err.Error() + "\n")
		return
	}

	//ignore SIGHUP
	cSignal := make(chan os.Signal, 1)
	signal.Notify(cSignal, syscall.SIGHUP)
	go func() {
		for {
			<-cSignal
		}
	}()

	//start net/http server	for benchmark comparison
	go startNetHttpServer()
	//start http reverse proxy, so you can query iip server as a http server,use client tool by curl,etc
	go startAdapterServer()
	// start iip server
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
		return
	}
	fmt.Printf("start listen %s iip server at :9090\n", tp)
	c := make(chan int)
	<-c
}

func startNetHttpServer() {
	echoHandler := func(w http.ResponseWriter, req *http.Request) {
		if bts, err := ioutil.ReadAll(req.Body); err == nil {
			w.Write(bts)
		} else {
			io.WriteString(w, err.Error())
		}

	}
	http.HandleFunc("/echo_benchmark", echoHandler)
	log.Fatal(http.ListenAndServe(":9091", nil))
}

func startAdapterServer() {
	adapterServer, err := httpadapter.NewHttpAdapterServer(":9092", "", "", time.Second)
	if err != nil {
		return
	}
	if err := adapterServer.RegisterBackend("echo_server", httpadapter.IipBackendConfig{ServerList: ":9090#1,:9090#1", ServerKeepConns: 10, ServerMaxConns: 10}); err != nil {
		return
	}
	if err := adapterServer.PushRouteTail("localhost", "^/echo", "echo_server"); err != nil {
		return
	}
	if err := adapterServer.ListenAndServe(); err != nil {
		return
	}
}

func daemonize() error {
	envs := os.Environ()
	for _, v := range envs {
		kv := strings.Split(v, "=")
		if len(kv) == 2 && kv[0] == "ppid" {
			return nil
		}
	}
	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	exePath, _ = filepath.EvalSymlinks(exePath)
	//daemonize
	envs = os.Environ()
	envs = append(envs, fmt.Sprintf("ppid=%d", os.Getpid()))
	workDir, _ := os.Getwd()
	_, err = os.StartProcess(exePath, os.Args, &os.ProcAttr{Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
		Dir: workDir,
		Env: envs,
		// Sys: &syscall.SysProcAttr{Setsid: true, Noctty: true},
	})
	if err != nil {
		return err
	}
	envs = os.Environ()
	envPpid := -100
	for _, v := range envs {
		kv := strings.Split(v, "=")
		if len(kv) == 2 && kv[0] == "ppid" {
			envPpid, _ = strconv.Atoi(kv[1])
			break
		}
	}
	if envPpid == -100 {
		//parent process
		os.Exit(0)
	}

	return nil
}
