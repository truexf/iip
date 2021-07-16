// Copyright 2021 fangyousong(方友松). All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package iip

import (
	"fmt"
	"os"
	"testing"
	"time"
)

func TestLoadBalanceClient(t *testing.T) {
	log.SetLevel(LogLevelDebug)
	lbc, err := NewLoadBalanceClient(ClientConfig{
		MaxConnections:        2,
		MaxChannelsPerConn:    2,
		ChannelPacketQueueLen: 1000,
		TcpWriteQueueLen:      1000,
		TcpReadBufferSize:     16 * 1024 * 1024,
		TcpWriteBufferSize:    16 * 1024 * 1024,
		TcpConnectTimeout:     time.Second * 3,
	}, ":9090#1,:9090#2,:9090#3")
	if err != nil {
		fmt.Errorf("new lbc fail,%s", err.Error())
		return
	}
	fmt.Println("new lbc ok")
	var btsPrev []byte
	defer fmt.Println(lbc.Status())
	for i := 0; i < 50; i++ {
		bts, err := lbc.DoRequest(PathServerConnectionStatis, NewDefaultRequest(nil), time.Second)
		if err != nil {
			os.Stdout.WriteString(fmt.Sprintf("err: %s\n", err.Error()))
			os.Stdout.WriteString(string(btsPrev))
			return
		} else {
			btsPrev = bts
			os.Stdout.WriteString(fmt.Sprintf("%d, %d\n", i, len(bts)))
			if i == 49 {
				os.Stdout.WriteString(string(bts))
			}
		}
	}
	os.Stdout.Sync()
	time.Sleep(time.Second)

}
