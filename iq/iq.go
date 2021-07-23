// Copyright 2021 fangyousong(方友松). All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// IQ is an IIP client tool that makes requests to the IIP Server and outputs responses, just as Curl is an HTTP client
package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/truexf/iip"
)

var (
	argData        = flag.String("data", "", `-data="Request data to send"`)
	argDataFile    = flag.String("data-file", "", `-data-file="The request data file to send"`)
	argTimeout     = flag.String("timeout", "1s", "-t|-timeout N[s|ms]")
	argTlsCertFile = flag.String("tlscert", "", "-tlscert")
	argTlsKeyFile  = flag.String("tlskey", "", "-tlskey")
	argTls         = flag.String("tls", "0", "-tls=1")
)

func usage() {
	fmt.Println(`IQ is an IIP client tool that makes requests to the IIP Server and outputs responses, just as Curl is an HTTP client
usage:
iq [OPTION] SERVER-URI

SERVER-URI: Server address in the format "IP:port/path?Arg1=value1&Arg2=value2",  path is required, args is optional
OPTION list:
  -data="Request data to send"
  -data-file="The request data file"
  -t|-timeout="N[s|ms]" Sets the timeout, seconds or milliseconds, to wait for a response
  -tlscert The certificate file, pem format
  -tlskey  The private key file
  -tls=1 Connect to the server with TLS
 `)
}

func init() {
	flag.Parse()
	flag.Usage = usage
}
func main() {
	flag.Parse()
	if len(flag.Args()) == 0 {
		fmt.Println("see --help")
		return
	}
	uri := flag.Arg(0)
	pathIndex := strings.Index(uri, "/")
	if pathIndex <= 0 {
		fmt.Printf("invalid uri %s, see --help", uri)
		return
	}
	svrAddr := uri[:pathIndex]
	path := uri[pathIndex:]

	var data []byte
	if *argData != "" {
		data = []byte(*argData)
	} else if *argDataFile != "" {
		if bts, err := ioutil.ReadFile(*argDataFile); err == nil {
			data = bts
		} else {
			fmt.Printf("read file %s fail, %s", *argDataFile, err.Error())
			return
		}
	}

	var timeoutDuration time.Duration = time.Second
	timeOut := *argTimeout
	for i, c := range timeOut {
		if c < '0' || c > '9' {
			if n, err := strconv.Atoi(timeOut[:i]); err != nil {
				fmt.Printf("invalid timeout arg, see --help")
				return
			} else {
				if timeOut[i:] == "ms" {
					timeoutDuration = time.Duration(n) * time.Millisecond
				} else if timeOut[i:] == "s" {
					timeoutDuration = time.Duration(n) * time.Second
				} else {
					fmt.Printf("invalid timeout arg, see --help")
					return
				}
			}
		}
	}

	var client *iip.Client
	var err error
	if *argTlsCertFile != "" && *argTlsKeyFile != "" {
		client, err = iip.NewClientTLS(
			iip.ClientConfig{
				MaxConnections:        1000,
				MaxChannelsPerConn:    10,
				ChannelPacketQueueLen: 1000,
				TcpWriteQueueLen:      1000,
				TcpReadBufferSize:     16 * 1024,
				TcpWriteBufferSize:    16 * 1024,
				TcpConnectTimeout:     time.Second * 3,
			},
			svrAddr,
			nil,
			*argTlsCertFile,
			*argTlsKeyFile,
		)
	} else if *argTls != "0" {
		client, err = iip.NewClientTLS(
			iip.ClientConfig{
				MaxConnections:        1000,
				MaxChannelsPerConn:    10,
				ChannelPacketQueueLen: 1000,
				TcpWriteQueueLen:      1000,
				TcpReadBufferSize:     16 * 1024,
				TcpWriteBufferSize:    16 * 1024,
				TcpConnectTimeout:     time.Second * 3,
			},
			svrAddr,
			nil,
			"",
			"",
		)
	} else {
		client, err = iip.NewClient(
			iip.ClientConfig{
				MaxConnections:        1000,
				MaxChannelsPerConn:    10,
				ChannelPacketQueueLen: 1000,
				TcpWriteQueueLen:      1000,
				TcpReadBufferSize:     16 * 1024,
				TcpWriteBufferSize:    16 * 1024,
				TcpConnectTimeout:     time.Second * 3,
			},
			svrAddr,
			nil,
		)
	}

	if err != nil {
		fmt.Printf("new client fail, %s\n", err.Error())
		return
	}
	channel, err := client.NewChannel()
	if err != nil {
		fmt.Printf("new channel fail, %s\n", err.Error())
		return
	}
	req := iip.NewDefaultRequest(data)
	bts, err := channel.DoRequest(path, req, timeoutDuration)
	if err != nil {
		fmt.Println(err.Error())
	} else {
		os.Stdout.Write(bts)
	}
}
