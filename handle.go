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

type Handler interface {
	Handle(request *Packet) ([]byte, error)
}

type PathHandler interface {
	Handle(path string, requestData []byte) ([]byte, error)
}

type SysHandler struct {
}

var sysHandler = &SysHandler{}

func (m *SysHandler) Handle(request *Packet) ([]byte, error) {
	if request == nil || request.Path == "" || request.channel == nil || request.channel.Conn == nil {
		return nil, fmt.Errorf("invalid request")
	}
	switch request.Path {
	case NewChannelPath:
		c, err := request.channel.Conn.newChannel(ChannelPacketQueueLen)
		if err != nil {
			bts, _ := json.Marshal(&ResponseNewChannel{Code: -1, Message: err.Error()})
			return bts, nil
		}
		bts, _ := json.Marshal(&ResponseNewChannel{Code: 0, ChannelId: c.Id})
		return bts, nil
	case DeleteChannelPath:
		request.channel.Close(fmt.Errorf("close by peer command"))
		bts, _ := json.Marshal(&ResponseDeleteChannel{Code: 0})
		return bts, nil
	default:
		pathHandler := pathHandlerMan.getHandler(request.Path)
		if pathHandler == nil {
			bts, _ := json.Marshal(&ResponseHandleFail{Code: -1, Message: "no handler"})
			return bts, nil
		} else {
			ret, err := pathHandler.Handle(request.Path, request.Data)
			if err != nil {
				bts, _ := json.Marshal(&ResponseHandleFail{Code: -1, Message: "handler fail:" + err.Error()})
				return bts, nil
			} else {
				return ret, nil
			}
		}
	}
}

type PathHandlerManager struct {
	HanderMap map[string]PathHandler
	sync.Mutex
}

var pathHandlerMan = &PathHandlerManager{HanderMap: make(map[string]PathHandler)}

func (m *PathHandlerManager) getHandler(path string) PathHandler {
	m.Lock()
	defer m.Unlock()
	if ret, ok := m.HanderMap[path]; ok {
		return ret
	}
	return nil
}

func (m *PathHandlerManager) RegisterHandler(path string, handler PathHandler) error {
	if handler == nil {
		return fmt.Errorf("hander is nil")
	}
	if len(path) > int(MaxPathLen) {
		return fmt.Errorf("path is too large, must <= %d", MaxPathLen)
	}
	m.Lock()
	defer m.Unlock()
	m.HanderMap[path] = handler
	return nil
}

func (m *PathHandlerManager) UnRegisterHandler(path string) {
	m.Lock()
	defer m.Unlock()
	delete(m.HanderMap, path)
}
