// Copyright 2021 fangyousong(方友松). All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package iip

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func BenchmarkPFBalanceClient(b *testing.B) {
	lbc, err := NewLoadBalanceClient(100, 1000, ":9090#1,:9090#1,:9090#1")
	if err != nil {
		b.Fatalf("new lbc fail,%s", err.Error())
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
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			bts, err := lbc.DoRequest("/echo_benchmark", NewDefaultRequest([]byte(echoData)), time.Second*3)
			if err != nil {
				b.Fatalf(err.Error())
			} else {
				if string(bts) != echoData {
					b.Fatalf("not equal")
				}
			}
		}
	})

}
