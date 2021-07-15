// Copyright 2021 fangyousong(方友松). All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"github.com/truexf/iip"
)

const (
	PathDownloadFile = "/download_file"
)

type FileDownloadTaskClientSide struct {
	taskId     int64  //unique
	serverFile string //文件远程路径
	localFile  string //存储路径
	// fd              *os.File
	downloadedBytes int  //已下载字节数
	completed       bool //下载完成
	err             error
}

type FileDownloadRequest struct {
	iip.DefaultContext
	TaskId           int64
	FilePath         string
	DownloadStartPos int
}

func NewFileDownloadRequest(task *FileDownloadTaskClientSide) (*FileDownloadRequest, error) {
	if task.completed {
		return nil, fmt.Errorf("file downloaded completed, task id %d", task.taskId)
	}
	ret := &FileDownloadRequest{TaskId: task.taskId, FilePath: task.serverFile, DownloadStartPos: task.downloadedBytes}
	ret.SetCtxData("task", task)
	return ret, nil
}

// implement Request interface
func (m *FileDownloadRequest) Data() []byte {
	bts, _ := json.Marshal(m)
	return bts
}

type FileDownloadClient struct {
	maxTaskId  int64
	iipChannel *iip.ClientChannel
}

func NewFileDownloadClient(serverAddr string) (*FileDownloadClient, error) {
	ret := &FileDownloadClient{}
	client, err := iip.NewClient(
		iip.ClientConfig{
			MaxConnections:        1000,
			MaxChannelsPerConn:    10,
			ChannelPacketQueueLen: 1000,
			TcpWriteQueueLen:      1000,
			TcpReadBufferSize:     16 * 1024,
			TcpWriteBufferSize:    16 * 1024,
			TcpConnectTimeout:     time.Second * 3,
		},
		serverAddr,
		nil,
	)

	if err != nil {
		return nil, err
	}
	client.RegisterHandler(PathDownloadFile, ret)
	channel, err := client.NewChannel()
	if err != nil {
		return nil, err
	}
	ret.iipChannel = channel
	return ret, nil
}

func (m *FileDownloadClient) newTaskId() int64 {
	return atomic.AddInt64(&m.maxTaskId, 1)
}

// 其格式为： 4字节下载完成标记+文件数据
func (m *FileDownloadClient) unmarshalFileDownloadResponseChunk(req iip.Request, netResponse []byte) error {
	task := req.(*FileDownloadRequest).GetCtxData("task").(*FileDownloadTaskClientSide)
	if len(netResponse) < 4 {
		task.err = fmt.Errorf("invalid response data, len: %d", len(netResponse))
		return task.err
	}

	completed := binary.BigEndian.Uint32(netResponse[:4]) == 1
	if fd, err := os.OpenFile(task.localFile, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0666); err != nil {
		panic(fmt.Sprintf("open local file %s fail, %s", task.localFile, err.Error()))
	} else {
		fmt.Printf("open file %s ok\n", task.localFile)
		defer fd.Close()
		if _, err := fd.Write(netResponse[4:]); err != nil {
			panic(fmt.Sprintf("write local file %s fail, %s", task.localFile, err.Error()))
		}
	}

	task.completed = completed
	task.downloadedBytes += (len(netResponse) - 4)
	if completed {
		fmt.Printf("download save to %s completed\n", task.localFile)
	} else {
		//not completed, continue
		return m.RunTask(task)
	}

	return nil
}

func (m *FileDownloadClient) RunTask(task *FileDownloadTaskClientSide) error {
	request, err := NewFileDownloadRequest(task)
	if err != nil {
		return err
	}

	return m.iipChannel.DoStreamRequest(PathDownloadFile, request)
}

func (m *FileDownloadClient) Handle(path string, request iip.Request, responseData []byte, dataCompleted bool) error {
	fmt.Printf("received response chunk %d bytes\n", len(responseData))
	if !dataCompleted {
		return nil
	}
	err := m.unmarshalFileDownloadResponseChunk(request, responseData)
	if err != nil {
		fmt.Println(err.Error())
	}
	return err
}

var (
	fileServerAddr = flag.String("server", ":9091", "file_client -server [server-addr] -file [server-file]")
	serverFile     = flag.String("file", "", "file_client -server [server-addr] -file [server-file]")
)

func main() {
	flag.Parse()
	if *fileServerAddr == "" || *serverFile == "" {
		panic("invaid params")
	}
	fileDownloadClient, err := NewFileDownloadClient(*fileServerAddr)
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	if err := fileDownloadClient.RunTask(&FileDownloadTaskClientSide{
		taskId:     fileDownloadClient.newTaskId(),
		serverFile: *serverFile,
		localFile:  fmt.Sprintf("/tmp/downloaded.file.%d", time.Now().Unix()),
	}); err != nil {
		fmt.Println(err.Error())
		return
	}

	waitChan := make(chan int)
	<-waitChan
}
