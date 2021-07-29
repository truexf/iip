// Copyright 2021 fangyousong(方友松). All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

//通用性函数与类型定义
package iip

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"sync"
	"time"
)

type Error struct {
	Code    int
	Message string
	Tm      time.Time
}

func (m *Error) Error() string {
	return m.Message
}

type ErrorHolder interface {
	GetError() error
	SetError(err error)
}

type DefaultErrorHolder struct {
	err error
}

func (m *DefaultErrorHolder) GetError() error {
	return m.err
}

func (m *DefaultErrorHolder) SetError(err error) {
	m.err = err
}

type Context interface {
	GetCtxData(key string) interface{}
	SetCtxData(key string, value interface{})
	RemoveCtxData(key string)
}

type DefaultContext struct {
	ctx     map[string]interface{}
	ctxLock sync.RWMutex
}

func (m *DefaultContext) GetCtxData(key string) interface{} {
	if key == "" {
		return nil
	}
	m.ctxLock.RLock()
	defer m.ctxLock.RUnlock()
	if m.ctx == nil {
		return nil
	}
	if ret, ok := m.ctx[key]; ok {
		return ret
	}
	return nil
}

func (m *DefaultContext) SetCtxData(key string, value interface{}) {
	if key == "" {
		return
	}
	m.ctxLock.Lock()
	defer m.ctxLock.Unlock()
	if m.ctx == nil {
		m.ctx = make(map[string]interface{})
	}
	m.ctx[key] = value
}

func (m *DefaultContext) RemoveCtxData(key string) {
	if key == "" {
		return
	}
	m.ctxLock.Lock()
	defer m.ctxLock.Unlock()
	if m.ctx == nil {
		return
	}
	delete(m.ctx, key)
}

type Request interface {
	Context
	Data() []byte
}

type DefaultRequest struct {
	DefaultContext
	data []byte
}

type Response struct {
	Request Request
	Data    []byte
}

// 构建一个iip请求，请求数据从data传入，如果传入nil,则函数自动填入"{}", iip协议不允许请求数据空白
func NewDefaultRequest(data []byte) Request {
	ret := &DefaultRequest{data: data}
	if len(ret.data) == 0 {
		ret.data = []byte("{}")
	}
	return ret
}

func (m *DefaultRequest) Data() []byte {
	return m.data
}

func ValidatePath(path string) bool {
	if path == "" {
		return false
	}
	if _, err := url.Parse(path); err != nil {
		return false
	}
	return true
}

var responseNotifyChanPool sync.Pool

func acquireNotifyChan() chan []byte {
	v := responseNotifyChanPool.Get()
	if v == nil {
		return make(chan []byte, 1)
	} else {
		ret := v.(chan []byte)
		select {
		case <-ret:
		default:
		}
		return ret
	}

}

func releaseNotifyChan(c chan []byte) {
	select {
	case <-c:
	default:
	}
	responseNotifyChanPool.Put(c)
}

func HttpHeaderToIIPArg(h http.Header) (string, error) {
	bts, err := json.Marshal(h)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bts), nil
}

func IIPArgToHttpHeader(argValue string) (http.Header, error) {
	bts, err := base64.URLEncoding.DecodeString(argValue)
	if err != nil {
		return nil, err
	}
	ret := make(http.Header)
	if err := json.Unmarshal(bts, &ret); err != nil {
		return nil, err
	} else {
		return ret, nil
	}
}
