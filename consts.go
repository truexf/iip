// Copyright 2021 fangyousong(方友松). All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package iip

//系统常量定义
const (
	MaxPathLen        uint32 = 2048             //packet的path字段最大字节数
	MaxPacketSize     uint32 = 16 * 1024 * 1024 //packet最大字节数
	PacketReadBufSize uint32 = 32 * 1024        //从他tcp fd读取数据用于缓存解析的缓冲区的大小

	//系统路径
	PathNewChannel             string = "/sys/new_channel"
	PathDeleteChannel          string = "/sys/delete_channel"
	PathServerCountJson        string = "/sys/server_count"      //获取服务器统计信息
	PathServerMeasureJson      string = "/sys/server_measure"    //获取服务器测量信息
	PathServerPathCountJson    string = "/sys/path_count"        //获取指定接口统计信息
	PathServerPathMeasureJson  string = "/sys/path_measure"      //获取指定接口测量信息
	PathServerStatis           string = "/sys/statis"            //获取服务器完整测量统计信息
	PathServerConnectionStatis string = "/sys/connection_statis" //获取服务器的所有tcp连接信息

	//角色
	RoleClient byte = 0
	RoleServer byte = 4

	//packet类型
	PacketTypeRequest  byte = 0
	PacketTypeResponse byte = 4

	//packet.status
	StatusC0 byte = 0 //请求首帧，请求未完成
	StatusC1 byte = 1 //请求首帧，请求完成
	StatusC2 byte = 2 //请求后续帧，请求未完成
	StatusC3 byte = 3 //请求后续帧，请求完成
	StatusS4 byte = 4 //响应首帧，响应未完成
	StatusS5 byte = 5 //表示响应首帧，响应完成
	StatusS6 byte = 6 //表示响应后续帧，响应未完成
	StatusS7 byte = 7 //表示响应后续帧，响应完成
	Status8  byte = 8 //关闭连接

	//系统Context常量
	CtxServer                 string = "/ctx/sys/server"
	CtxClient                 string = "/ctx/sys/client"
	CtxResponseChan           string = "/ctx/sys/response_chan"
	CtxUncompletedRequestChan string = "/ctx/sys/uncreq_chan" //见Client.uncompletedRequestQueue
	CtxRequest                string = "/ctx/sys/request"     //在client handle函数里,可以通过channel.GetCtxData(CtxRequest)获得当前响应对应的请求
)
