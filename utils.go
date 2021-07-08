// Copyright 2021 fangyousong(方友松). All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

//通用性函数与类型定义
package iip

import (
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
		m.ctx = make(map[string]interface{})
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
		m.ctx = make(map[string]interface{})
	}
	delete(m.ctx, key)
}
