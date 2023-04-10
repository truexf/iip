// Copyright 2021 fangyousong(方友松). All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package httpadapter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/truexf/goutil"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"
	"time"
)

func BenchmarkBMHttpAdapter(t *testing.B) {
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
	bts := []byte(strings.Repeat(echoData, 10))
	t.ResetTimer()
	for i := 0; i < t.N; i++ {
		resp, err := http.Post("http://localhost:9092/echo", "", bytes.NewReader(bts))
		if err != nil {
			t.Fatalf(err.Error())
		}
		ret, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf(err.Error())
		}
		if !bytes.Equal(bts, ret) {
			t.Fatalf("request response not equal\n%s\n%s\n", goutil.UnsafeBytesToString(bts), goutil.UnsafeBytesToString(ret))
		}
	}
}

// start test server first (in main/http_adapter_test_server.go)
func TestHttpAdapter(t *testing.T) {
	time.Sleep(time.Second * 2)
	bts := []byte(strings.Repeat("hello, http adapater\n", 100))
	resp, err := http.Post("http://localhost:9092/echo", "", bytes.NewReader(bts))
	if err != nil {
		t.Fatalf(err.Error())
	}
	ret, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf(err.Error())
	}
	if !bytes.Equal(bts, ret) {
		t.Fatalf("request response not equal\n%s\n%s\n", goutil.UnsafeBytesToString(bts), goutil.UnsafeBytesToString(ret))
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
