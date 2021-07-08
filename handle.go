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

//管理PathHandler,从属于一个client或server
type PathHandlerManager struct {
	HanderMap map[string]PathHandler
	sync.Mutex
}

func (m *PathHandlerManager) getHandler(path string) PathHandler {
	m.Lock()
	defer m.Unlock()
	if m.HanderMap == nil {
		m.HanderMap = make(map[string]PathHandler)
	}
	if ret, ok := m.HanderMap[path]; ok {
		return ret
	}
	return nil
}

func (m *PathHandlerManager) registerHandler(path string, handler PathHandler) error {
	if handler == nil {
		return fmt.Errorf("hander is nil")
	}
	if len(path) > int(MaxPathLen) {
		return fmt.Errorf("path is too large, must <= %d", MaxPathLen)
	}
	m.Lock()
	defer m.Unlock()
	if m.HanderMap == nil {
		m.HanderMap = make(map[string]PathHandler)
	}
	m.HanderMap[path] = handler
	return nil
}

func (m *PathHandlerManager) unRegisterHandler(path string) {
	m.Lock()
	defer m.Unlock()
	if m.HanderMap == nil {
		m.HanderMap = make(map[string]PathHandler)
	}
	delete(m.HanderMap, path)
}

//packet handler接口
type Handler interface {
	Handle(request *Packet, dataCompleted bool) ([]byte, error)
}

//path-handler接口，PathHandler在packet-handler基础上执行，有serverHandler或clientHandler在Handle函数内部调用
type PathHandler interface {
	Handle(path string, requestData []byte, dataCompleted bool) ([]byte, error)
}

type serverHandler struct {
	DefaultContext
	pathHandlerManager *PathHandlerManager
}

func (m *serverHandler) Handle(request *Packet, dataCompleted bool) ([]byte, error) {
	if request == nil || request.Path == "" || request.channel == nil || request.channel.Conn == nil {
		return nil, fmt.Errorf("invalid request")
	}
	switch request.Path {
	case PathNewChannel:
		c := request.channel.Conn.newChannel(false, 100)
		bts, _ := json.Marshal(&ResponseNewChannel{Code: 0, ChannelId: c.Id})
		return bts, nil
	case PathDeleteChannel:
		request.channel.Close(fmt.Errorf("close by peer command"))
		bts, _ := json.Marshal(&ResponseDeleteChannel{Code: 0})
		return bts, nil
	default:
		pathHandler := m.pathHandlerManager.getHandler(request.Path)
		if pathHandler == nil {
			bts, _ := json.Marshal(&ResponseHandleFail{Code: -1, Message: "no handler"})
			return bts, nil
		} else {
			ret, err := pathHandler.Handle(request.Path, request.Data, dataCompleted)
			if err != nil {
				bts, _ := json.Marshal(&ResponseHandleFail{Code: -1, Message: "handler fail:" + err.Error()})
				return bts, nil
			} else {
				return ret, nil
			}
		}
	}
}

type clientHandler struct {
	DefaultContext
	pathHandlerManager *PathHandlerManager
}

func (m *clientHandler) Handle(response *Packet, dataCompleted bool) ([]byte, error) {
	if response == nil || response.Path == "" || response.channel == nil || response.channel.Conn == nil {
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
			bts, _ := json.Marshal(&ResponseHandleFail{Code: -1, Message: "no handler"})
			return bts, nil
		} else {
			ret, err := pathHandler.Handle(response.Path, response.Data, dataCompleted)
			if err != nil {
				bts, _ := json.Marshal(&ResponseHandleFail{Code: -1, Message: "handler fail:" + err.Error()})
				return bts, nil
			} else {
				return ret, nil
			}
		}
	}
}
