// Copyright 2021 fangyousong(方友松). All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package iip

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLoadBalanceClient(t *testing.T) {
	LoadBalanceClientDemo()
}

func LoadBalanceClientDemo() {
	GetLogger().SetLevel(LogLevelDebug)
	lbc, err := NewLoadBalanceClient(100, 1000, ":9090#1,:9090#1,:9090#1")
	if err != nil {
		fmt.Printf("new lbc fail,%s", err.Error())
		return
	}

	fmt.Println("new lbc ok")
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
	var wg sync.WaitGroup
	wg.Add(50)
	tm := time.Now()
	var cnt uint32 = 0
	for i := 0; i < 50; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				bts, err := lbc.DoRequest("/echo", NewDefaultRequest([]byte(echoData)), time.Second*3)
				if err != nil {
					os.Stdout.WriteString(fmt.Sprintf("err: %s\n", err.Error()))
				} else {
					if string(bts) != echoData {
						panic("not equal")
					} else {
						atomic.AddUint32(&cnt, 1)
					}
				}
			}
		}()
	}
	wg.Wait()
	os.Stdout.WriteString(fmt.Sprintf("%d milliseconds, %d requests, everage %d microsecond, \n",
		time.Since(tm)/time.Millisecond,
		cnt,
		time.Since(tm)/time.Microsecond/time.Duration(cnt)))
	time.Sleep(time.Second)
	if bts, err := lbc.DoRequest("/cc", NewDefaultRequest(nil), time.Second); err == nil {
		os.Stdout.WriteString(string(bts) + "\n")
	} else {
		os.Stdout.WriteString(err.Error() + "\n")
	}
	os.Stdout.Sync()
}
