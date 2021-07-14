// Copyright 2021 fangyousong(方友松). All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

//各种处理器(Handler)定义
package iip

import (
	"encoding/json"
	"fmt"
	"sync"
)

type ResponseNewChannel struct {
	Code      int    `json:"code"`
	Message   string `json:"message,omitempty"`
	ChannelId uint32 `json:"channel_id,omitempty"`
}

type ResponseDeleteChannel struct {
	Code    int    `json:"code"`
	Message string `json:"message,omitempty"`
}

type ResponseHandleFail struct {
	Code    int    `json:"code"`
	Message string `json:"message,omitempty"`
}

func (m *ResponseHandleFail) Data() []byte {
	if ret, err := json.Marshal(m); err == nil {
		return ret
	} else {
		return DefaultResponseData
	}
}

func ErrorResponse(err *Error) *ResponseHandleFail {
	return &ResponseHandleFail{Code: err.Code, Message: err.Message}
}

//管理ServerPathHandler,从属于一个server
type ServerPathHandlerManager struct {
	HandlerMap map[string]ServerPathHandler
	sync.Mutex
}

func (m *ServerPathHandlerManager) getHandler(path string) ServerPathHandler {
	m.Lock()
	defer m.Unlock()
	if m.HandlerMap == nil {
		m.HandlerMap = make(map[string]ServerPathHandler)
	}
	if ret, ok := m.HandlerMap[path]; ok {
		return ret
	}
	return nil
}

func (m *ServerPathHandlerManager) registerHandler(path string, handler ServerPathHandler) error {
	if handler == nil {
		return fmt.Errorf("hander is nil")
	}
	if len(path) > int(MaxPathLen) {
		return fmt.Errorf("path is too large, must <= %d", MaxPathLen)
	}
	if !ValidatePath(path) {
		return fmt.Errorf("invalid path")
	}
	m.Lock()
	defer m.Unlock()
	if m.HandlerMap == nil {
		m.HandlerMap = make(map[string]ServerPathHandler)
	}
	m.HandlerMap[path] = handler
	return nil
}

func (m *ServerPathHandlerManager) unRegisterHandler(path string) {
	m.Lock()
	defer m.Unlock()
	if m.HandlerMap == nil {
		m.HandlerMap = make(map[string]ServerPathHandler)
	}
	delete(m.HandlerMap, path)
}

//管理ClientPathHandler,从属于一个client
type ClientPathHandlerManager struct {
	HanderMap map[string]ClientPathHandler
	sync.Mutex
}

func (m *ClientPathHandlerManager) getHandler(path string) ClientPathHandler {
	m.Lock()
	defer m.Unlock()
	if m.HanderMap == nil {
		m.HanderMap = make(map[string]ClientPathHandler)
	}
	if ret, ok := m.HanderMap[path]; ok {
		return ret
	}
	return nil
}

func (m *ClientPathHandlerManager) registerHandler(path string, handler ClientPathHandler) error {
	if handler == nil {
		return fmt.Errorf("hander is nil")
	}
	if len(path) > int(MaxPathLen) {
		return fmt.Errorf("path is too large, must <= %d", MaxPathLen)
	}
	if !ValidatePath(path) {
		return fmt.Errorf("invalid path")
	}
	m.Lock()
	defer m.Unlock()
	if m.HanderMap == nil {
		m.HanderMap = make(map[string]ClientPathHandler)
	}
	m.HanderMap[path] = handler
	return nil
}

func (m *ClientPathHandlerManager) unRegisterHandler(path string) {
	m.Lock()
	defer m.Unlock()
	if m.HanderMap == nil {
		m.HanderMap = make(map[string]ClientPathHandler)
	}
	delete(m.HanderMap, path)
}

//packet handler接口
type Handler interface {
	Handle(c *Channel, request *Packet, dataCompleted bool) ([]byte, error)
}

type ClientPathHandler interface {
	Handle(path string, request Request, responseData []byte, responseDataCompleted bool) error
}

type DefaultClientPathHandler struct {
}

func (m *DefaultClientPathHandler) Handle(path string, request Request, responseData []byte, dataCompleted bool) error {
	return nil
}

// 普通响应，必须实现
type ServerPathHandler interface {
	Handle(path string, requestData []byte, requestDataCompleted bool) (responseData []byte, e error)
}

type serverHandler struct {
	DefaultContext
	pathHandlerManager *ServerPathHandlerManager
}

func (m *serverHandler) Handle(c *Channel, request *Packet, dataCompleted bool) ([]byte, error) {
	if request == nil || request.Path == "" || request.channel == nil || request.channel.conn == nil {
		return nil, fmt.Errorf("invalid request")
	}
	switch request.Path {
	case PathNewChannel:
		c := request.channel.conn.newChannel(false, 100, nil, nil)
		bts, _ := json.Marshal(&ResponseNewChannel{Code: 0, ChannelId: c.Id})
		return bts, nil
	case PathDeleteChannel:
		request.channel.Close(fmt.Errorf("close by peer command"))
		bts, _ := json.Marshal(&ResponseDeleteChannel{Code: 0})
		return bts, nil
	default:
		pathHandler := m.pathHandlerManager.getHandler(request.Path)
		if pathHandler == nil {
			return nil, ErrResponseHandlerNotImplement
		}
		ret, err := pathHandler.Handle(request.Path, request.Data, dataCompleted)
		if err != nil {
			bts, _ := json.Marshal(&ResponseHandleFail{Code: -1, Message: "handler fail:" + err.Error()})
			return bts, nil
		} else {
			return ret, nil
		}
	}
}

type clientHandler struct {
	DefaultContext
	pathHandlerManager *ClientPathHandlerManager
}

func (m *clientHandler) Handle(c *Channel, response *Packet, dataCompleted bool) ([]byte, error) {
	if response == nil || response.Path == "" || response.channel == nil || response.channel.conn == nil {
		return nil, fmt.Errorf("invalid response")
	}
	switch response.Path {
	case PathDeleteChannel:
		response.channel.Close(fmt.Errorf("close by peer command"))
		bts, _ := json.Marshal(&ResponseDeleteChannel{Code: 0})
		return bts, nil
	default:
		pathHandler := m.pathHandlerManager.getHandler(response.Path)
		if pathHandler == nil {
			pathHandler = &DefaultClientPathHandler{}
		}

		err := pathHandler.Handle(response.Path, c.GetCtxData(CtxRequest).(Request), response.Data, dataCompleted)
		if err != nil {
			return nil, err
		} else {
			return nil, nil
		}

	}
}
