// Copyright 2021 fangyousong(方友松). All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"

	"github.com/truexf/iip"
)

const (
	PathDownloadFile = "/download_file"
	ListenAddr       = ":9091"
)

type FileDownloadRequest struct {
	TaskId           int64
	FilePath         string
	DownloadStartPos int
}

type FileServer struct {
}

func (m *FileServer) Handle(path string, queryParams url.Values, requestData []byte, requestDataCompleted bool) ([]byte, error) {
	fmt.Printf("request: %s\n", string(requestData))
	if !requestDataCompleted {
		return nil, nil
	}
	fmt.Println("request data completed")
	var req FileDownloadRequest
	if err := json.Unmarshal(requestData, &req); err != nil {
		fmt.Printf("unmarshal request fail, %s\n", err.Error())
		return nil, err
	}
	fd, err := os.Open(req.FilePath)
	if err != nil {
		fmt.Printf("openfile %s fail, %s\n", req.FilePath, err.Error())
		return nil, err
	}

	defer fd.Close()
	fd.Seek(int64(req.DownloadStartPos), io.SeekStart)
	for {
		buf := make([]byte, 1024*10)
		n, err := fd.Read(buf[4:])
		if err != nil {
			fmt.Printf("read file fail,%s\n", err.Error())
			// 读取完成
			binary.BigEndian.PutUint32(buf, uint32(1))
			fmt.Println("EOF, send response")
			return buf[:4], nil
		} else {
			// 4字节下载完成标记+文件数据
			binary.BigEndian.PutUint32(buf, uint32(0))
			fmt.Println("continue, send response")
			return buf[:n+4], nil
		}
	}
}

func main() {
	fmt.Printf("start listen at %s\n", ListenAddr)
	server, err := iip.NewServer(iip.ServerConfig{
		MaxConnections:        1000,
		MaxChannelsPerConn:    10,
		ChannelPacketQueueLen: 1000,
		TcpWriteQueueLen:      1000,
		TcpReadBufferSize:     16 * 1024,
		TcpWriteBufferSize:    16 * 1024,
	}, ListenAddr, nil)

	if err != nil {
		fmt.Println(err.Error())
	}
	svr := &FileServer{}
	server.RegisterHandler(PathDownloadFile, svr, nil)

	err = server.StartListen()

	if err != nil {
		fmt.Println(err.Error())
	}

	waitChan := make(chan int)
	<-waitChan
}
