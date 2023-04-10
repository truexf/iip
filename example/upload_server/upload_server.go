// Copyright 2021 fangyousong(方友松). All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/truexf/goutil"
	"hash/fnv"
	"io"
	"net/url"
	"os"

	"github.com/truexf/iip"
)

const (
	PathUploadFile = "/upload_file"
	ListenAddr     = ":9092"
)

type FileUploadRequest struct {
	TaskId         int64
	FilePath       string
	UploadStartPos int
	Completed      bool
}

type FileUploadResponse struct {
	TaskId    int64
	DoneBytes int64 //上载成功字节数
}

type FileServer struct {
}

func (m *FileServer) Handle(path string, queryParams url.Values, requestData []byte, requestDataCompleted bool) ([]byte, error) {
	fmt.Printf("request %d bytes\n", len(requestData))
	if len(requestData) <= 4 {
		return nil, fmt.Errorf("invalid request: %s", goutil.UnsafeBytesToString(requestData))
	}
	if !requestDataCompleted {
		return nil, nil
	}
	// 4字节元数据信息长度+文件数据
	// 元数据信息：json.Marshal(FileUploadRequest)
	metaLen := binary.BigEndian.Uint32(requestData[:4])
	var meta FileUploadRequest
	if err := json.Unmarshal(requestData[4:4+metaLen], &meta); err != nil {
		return nil, fmt.Errorf("invalid request, %s", err.Error())
	}
	fmt.Printf("read request meta: %v\n", meta)
	//上传请求中指定服务器文件路径是为了作示例，存在安全隐患，这里转换为一个临时目录文件
	sn := fnv.New64()
	sn.Write(goutil.UnsafeStringToBytes(meta.FilePath))
	fn := fmt.Sprintf("/tmp.upload.%d", sn.Sum64())

	openFlag := os.O_CREATE | os.O_APPEND | os.O_RDWR
	if meta.UploadStartPos == 0 {
		openFlag |= os.O_TRUNC
	}
	fd, err := os.OpenFile(fn, openFlag, 0666)
	if err != nil {
		fmt.Printf("openfile %s fail, %s\n", fn, err.Error())
		return nil, err
	}
	defer fd.Close()
	if _, err := fd.Seek(int64(meta.UploadStartPos), io.SeekStart); err != nil {
		if err != io.EOF {
			return nil, err
		}
	}

	if n, err := fd.Write(requestData[4+metaLen:]); err != nil {
		return nil, err
	} else {
		ret := &FileUploadResponse{TaskId: meta.TaskId, DoneBytes: int64(n)}
		if meta.Completed && n == len(requestData)-4-int(metaLen) {
			fmt.Printf("update file %s completed.\n", fn)
		}
		return json.Marshal(ret)
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
	server.RegisterHandler(PathUploadFile, svr, nil)

	err = server.StartListen()

	if err != nil {
		fmt.Println(err.Error())
	}

	waitChan := make(chan int)
	<-waitChan
}
