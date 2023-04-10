// Copyright 2021 fangyousong(方友松). All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sync/atomic"
	"time"

	"github.com/truexf/iip"
)

const (
	PathUploadFile = "/upload_file"
)

type FileUploadTaskClientSide struct {
	taskId        int64  //unique
	serverFile    string //文件远程路径
	localFile     string //
	uploadedBytes int    //已上传字节数
	completed     bool   //上传完成
}

type FileUploadRequest struct {
	iip.DefaultContext `json:"-"`
	TaskId             int64
	FilePath           string
	UploadStartPos     int
	Completed          bool
	ChunkedData        []byte `json:"-"` //do not marshal
}

type FileUploadResponse struct {
	TaskId    int64
	DoneBytes int64 //上载成功字节数
}

func NewFileUploadRequest(task *FileUploadTaskClientSide) (*FileUploadRequest, error) {
	if task.completed {
		return nil, fmt.Errorf("file upload completed, task id %d", task.taskId)
	}
	ret := &FileUploadRequest{TaskId: task.taskId, FilePath: task.serverFile, UploadStartPos: task.uploadedBytes}
	fd, err := os.Open(task.localFile)
	if err != nil {
		panic(fmt.Sprintf("open file %s fail, %s", task.localFile, err.Error()))
	}
	defer fd.Close()
	ret.ChunkedData = make([]byte, 1024*1024*10) //每个请求最多传输10M
	fileReadCompleted := false
	if _, err := fd.Seek(int64(task.uploadedBytes), io.SeekStart); err != nil {
		if err == io.EOF {
			fileReadCompleted = true
		} else {
			panic(fmt.Sprintf("seek file %s fail, %s", task.localFile, err.Error()))
		}
	}
	n, err := fd.Read(ret.ChunkedData)
	if err != nil {
		if err != io.EOF {
			panic(fmt.Sprintf("read file %s fail, %s", task.localFile, err.Error()))
		} else {
			fileReadCompleted = true
		}
	}
	ret.ChunkedData = ret.ChunkedData[:n]
	ret.Completed = fileReadCompleted
	task.completed = fileReadCompleted
	ret.SetCtxData("task", task)
	return ret, nil
}

// implement Request interface
func (m *FileUploadRequest) Data() []byte {
	// 4字节元数据信息长度+文件数据
	// 元数据信息：json.Marshal(FileUploadRequest)
	bts, _ := json.Marshal(m)
	ret := make([]byte, 4, 4+len(bts)+len(m.ChunkedData))
	binary.BigEndian.PutUint32(ret, uint32(len(bts)))
	ret = append(ret, bts...)
	if len(m.ChunkedData) > 0 {
		ret = append(ret, m.ChunkedData...)
	}
	return ret
}

type FileUploadClient struct {
	maxTaskId  int64
	iipChannel *iip.ClientChannel
}

func NewFileUploadClient(serverAddr string) (*FileUploadClient, error) {
	ret := &FileUploadClient{}
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
	client.RegisterHandler(PathUploadFile, ret)
	channel, err := client.NewChannel()
	if err != nil {
		return nil, err
	}
	ret.iipChannel = channel
	return ret, nil
}

func (m *FileUploadClient) newTaskId() int64 {
	return atomic.AddInt64(&m.maxTaskId, 1)
}

func (m *FileUploadClient) handleFileUploadResponse(req iip.Request, netResponse []byte) error {
	task := req.(*FileUploadRequest).GetCtxData("task").(*FileUploadTaskClientSide)
	var resp FileUploadResponse
	if err := json.Unmarshal(netResponse, &resp); err != nil {
		panic(fmt.Sprintf("response invalid, %s", err.Error()))
	}
	task.uploadedBytes += int(resp.DoneBytes)
	if !task.completed {
		//not completed, continue
		return m.RunTask(task)
	} else {
		fmt.Printf("upload file %s to server completed.\n", task.localFile)
	}

	return nil
}

func (m *FileUploadClient) RunTask(task *FileUploadTaskClientSide) error {
	request, err := NewFileUploadRequest(task)
	if err != nil {
		return err
	}

	return m.iipChannel.DoStreamRequest(PathUploadFile, request)
}

func (m *FileUploadClient) Handle(path string, request iip.Request, responseData []byte, dataCompleted bool) error {
	fmt.Printf("received response(%d bytes): %s\n", len(responseData), goutil.UnsafeBytesToString(responseData))
	if !dataCompleted {
		return nil
	}
	err := m.handleFileUploadResponse(request, responseData)
	if err != nil {
		fmt.Println(err.Error())
	}
	return err
}

var (
	serverAddr = flag.String("server", ":9092", "upload_client -server [server-addr] -file [local-file]")
	fileLocal  = flag.String("file", "", "upload_client -server [server-addr] -file [local-file]")
)

func main() {
	flag.Parse()
	if *fileLocal == "" || *serverAddr == "" {
		panic("invaid params, see --help")
	}
	fileUploadClient, err := NewFileUploadClient(*serverAddr)
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	if err := fileUploadClient.RunTask(&FileUploadTaskClientSide{
		taskId:     fileUploadClient.newTaskId(),
		serverFile: *fileLocal,
		localFile:  *fileLocal,
	}); err != nil {
		fmt.Println(err.Error())
		return
	}

	waitChan := make(chan int)
	<-waitChan
}
