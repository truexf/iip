// Copyright 2021 fangyousong(方友松). All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package iip

import (
	"fmt"
	"testing"
	"time"
)

func TestServerCount(t *testing.T) {
	client, err := NewClient(ClientConfig{
		MaxConnections:        1000,
		MaxChannelsPerConn:    10,
		ChannelPacketQueueLen: 1000,
		TcpWriteQueueLen:      1000,
		TcpReadBufferSize:     16 * 1024,
		TcpWriteBufferSize:    16 * 1024,
		TcpConnectTimeout:     time.Second * 3,
	}, ":9090", nil)
	if err != nil {
		t.Fatalf("connect server fail")
		return
	}
	channel, err := client.NewChannel()
	if err != nil {
		t.Fatalf("new channel fail, %s", err.Error())
		return
	}
	// echoData := []byte("{}")
	// bts, err := channel.DoRequest(PathServerCountJson, NewDefaultRequest(echoData), time.Second)
	// if err != nil {
	// 	t.Fatalf(err.Error())
	// }
	// fmt.Println(string(bts))
	// bts, err = channel.DoRequest(PathServerMeasureJson, NewDefaultRequest(echoData), time.Second)
	// if err != nil {
	// 	t.Fatalf(err.Error())
	// }
	// fmt.Println(string(bts))
	// echoData = []byte("/echo")
	// bts, err = channel.DoRequest(PathServerPathCountJson, NewDefaultRequest(echoData), time.Second)
	// if err != nil {
	// 	t.Fatalf(err.Error())
	// }
	// fmt.Println(string(bts))
	// echoData = []byte("/echo")
	// bts, err = channel.DoRequest(PathServerPathMeasureJson, NewDefaultRequest(echoData), time.Second)
	// if err != nil {
	// 	t.Fatalf(err.Error())
	// }
	// fmt.Println(string(bts))
	echoData := []byte(`{"time_unit": "microsecond"}`)
	bts, err := channel.DoRequest(PathServerStatis, NewDefaultRequest(echoData), time.Second)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println(string(bts))
	bts, err = channel.DoRequest(PathServerConnectionStatis, NewDefaultRequest(echoData), time.Second*3)
	if err != nil {
		t.Fatalf(err.Error())
	}
	fmt.Println(string(bts))
}
