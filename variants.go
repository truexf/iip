// Copyright 2021 fangyousong(方友松). All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package iip

//系统变量定义
var (
	DefaultResponseData      = []byte(`{"code": -1, "message": "unknown"}`)
	LogClosing          bool = true //channel, connection关闭的时候记录日志

	ErrPacketContinue   error = &Error{Code: 100, Message: "packet uncompleted"}
	ErrHandleNoResponse error = &Error{Code: 101, Message: "handle no response"}
	ErrHandleError      error = &Error{Code: 102, Message: "handle error"}
	ErrRequestTimeout   error = &Error{Code: 103, Message: "request timtout"}
	ErrUnknown          error = &Error{Code: 104, Message: "unknown"}
)
