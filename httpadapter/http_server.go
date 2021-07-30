// Copyright 2021 fangyousong(方友松). All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package httpadapter

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/truexf/iip"
)

type IipBackendConfig struct {
	ServerList      string //ip:port#weight(1~100),ip:port#weight,...
	ServerKeepConns int
	ServerMaxConns  int
}

type RouteFunc interface {
	GetBackendAlias(host string, uri string) string
}
type RegExpRoute struct {
	RegExpStr    string
	expr         *regexp.Regexp
	BackendAlias string
}

/*
  http client(browser,...) <==> HttpAdapterServer <==> iip server(s)
  HttpServer作为一组iip server的反向代理，接收来自客户端的http请求，并转换为iip协议，转发给后端的iip server，接收iip server的响应，
  将iip server的响应转换为http响应，并返回给客户端
*/
type HttpAdapterServer struct {
	httpServer     *http.Server
	addr           string
	tlsCertFile    string
	tlsKeyFile     string
	requestTimeout time.Duration
	backends       map[string]*iip.LoadBalanceClient
	backendLock    sync.RWMutex
	routes         map[string][]RegExpRoute
	routeLock      sync.RWMutex
	routeFuncs     map[string]RouteFunc
	routeFuncsLock sync.RWMutex
}

func NewHttpAdapterServer(listenAddr string, tlsCertFile, tlsKeyFile string, requestTimeout time.Duration) (*HttpAdapterServer, error) {
	ret := &HttpAdapterServer{addr: listenAddr, tlsCertFile: tlsCertFile, tlsKeyFile: tlsKeyFile, requestTimeout: requestTimeout}
	ret.backends = make(map[string]*iip.LoadBalanceClient)
	ret.routes = make(map[string][]RegExpRoute)
	ret.routeFuncs = make(map[string]RouteFunc)
	return ret, nil
}

// 注册backend(iip server), alias必须唯一，config指定一组iip server及相关参数
func (m *HttpAdapterServer) RegisterBackend(alias string, config IipBackendConfig) error {
	m.backendLock.Lock()
	defer m.backendLock.Unlock()
	if _, ok := m.backends[alias]; ok {
		return fmt.Errorf("alias %s exists", alias)
	}
	lbc, err := iip.NewLoadBalanceClient(config.ServerKeepConns, config.ServerMaxConns, config.ServerList)
	if err != nil {
		return err
	}
	m.backends[alias] = lbc

	return nil
}

// 移除backend
func (m *HttpAdapterServer) RemoveBackend(alias string) {
	m.backendLock.Lock()
	defer m.backendLock.Unlock()
	delete(m.backends, alias)
}

func (m *HttpAdapterServer) getBackend(alias string) *iip.LoadBalanceClient {
	m.backendLock.RLock()
	defer m.backendLock.RUnlock()
	ret, ok := m.backends[alias]
	if ok {
		return ret
	}
	return nil
}

func (m *HttpAdapterServer) RegisterRouteFunc(host string, fn RouteFunc) error {
	if host == "" {
		return fmt.Errorf("empty host")
	}
	m.routeFuncsLock.Lock()
	defer m.routeFuncsLock.Unlock()
	if fn != nil {
		m.routeFuncs[host] = fn
	} else {
		delete(m.routeFuncs, host)
	}
	return nil
}

// 注册一个路由（相当于nginx的location）,根据正则表达式的匹配决定是用哪组backend
func (m *HttpAdapterServer) PushRouteTail(host string, regExp string, backendAlias string) error {
	if host == "" {
		return fmt.Errorf("empty host")
	}
	if regExp == "" {
		return fmt.Errorf("empty regexp")
	}
	if backendAlias == "" {
		return fmt.Errorf("empty alias")
	}
	exp, err := regexp.Compile(regExp)
	if err != nil {
		return err
	}
	m.routeLock.Lock()
	defer m.routeLock.Unlock()
	hostRoutes, ok := m.routes[host]
	if !ok {
		hostRoutes = make([]RegExpRoute, 0)
	}
	hostRoutes = append(hostRoutes, RegExpRoute{RegExpStr: regExp, expr: exp, BackendAlias: backendAlias})
	m.routes[host] = hostRoutes

	return nil
}

// 头部添加路由，路由的选择从头到尾进行，先匹配到的先得， 因此分别提供了头部注册和尾部注册，方便开发者决定优先级
func (m *HttpAdapterServer) PushRouteHead(host string, regExp string, backendAlias string) error {
	if host == "" {
		return fmt.Errorf("empty host")
	}
	if regExp == "" {
		return fmt.Errorf("empty regexp")
	}
	if backendAlias == "" {
		return fmt.Errorf("empty alias")
	}
	exp, err := regexp.Compile(regExp)
	if err != nil {
		return err
	}
	m.routeLock.Lock()
	defer m.routeLock.Unlock()
	hostRoutes, ok := m.routes[host]
	if !ok {
		hostRoutes = make([]RegExpRoute, 0)
	}
	var ret []RegExpRoute
	ret = append(ret, RegExpRoute{RegExpStr: regExp, expr: exp, BackendAlias: backendAlias})
	ret = append(ret, hostRoutes...)
	m.routes[host] = ret
	return nil
}

// 移除路由
func (m *HttpAdapterServer) RemoveRoute(host, regExp string) {
	m.routeLock.Lock()
	defer m.routeLock.Unlock()
	hostRoute, ok := m.routes[host]
	if !ok {
		return
	}
	for {
		found := false
		for i, v := range hostRoute {
			if v.RegExpStr == regExp {
				ret := hostRoute[:i]
				if i < len(m.routes)-1 {
					ret = append(ret, hostRoute[i+1:]...)
				}
				m.routes[host] = ret
				found = true
				break
			}
		}
		if !found {
			break
		}
	}
}

func (m *HttpAdapterServer) findRegExpRoute(host string, uri string) string {
	m.routeLock.RLock()
	defer m.routeLock.RUnlock()
	hostRoute, ok := m.routes[host]
	if !ok {
		return ""
	}
	//第一次完全匹配
	for _, v := range hostRoute {
		if v.expr.FindString(uri) == uri {
			return v.BackendAlias
		}
	}

	//第二次部分匹配
	for _, v := range hostRoute {
		if v.expr.MatchString(uri) {
			return v.BackendAlias
		}
	}

	return ""
}

func (m *HttpAdapterServer) findFuncRoute(host, uri string) string {
	m.routeFuncsLock.RLock()
	defer m.routeFuncsLock.RUnlock()
	if fn, ok := m.routeFuncs[host]; ok {
		return fn.GetBackendAlias(host, uri)
	}
	return ""
}

func (m *HttpAdapterServer) findBackend(host, uri string) *iip.LoadBalanceClient {
	alias := m.findFuncRoute(host, uri)
	if alias == "" {
		alias = m.findRegExpRoute(host, uri)
	}
	if alias != "" {
		return m.getBackend(alias)
	}
	return nil
}

func (m *HttpAdapterServer) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	hostSeg := strings.Split(req.Host, ":")
	backend := m.findBackend(hostSeg[0], req.RequestURI)
	if backend == nil {
		os.Stdout.WriteString("route not found\n")
		w.WriteHeader(http.StatusNotImplemented)
		return
	}
	bts, err := ioutil.ReadAll(req.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	uri := req.RequestURI
	if headerArg, err := iip.HttpHeaderToIIPArg(req.Header); err == nil {
		if req.URL.RawQuery != "" {
			uri = uri + fmt.Sprintf("&%s=%s", iip.ArgHttpHeader, headerArg)
		} else {
			uri = uri + fmt.Sprintf("?%s=%s", iip.ArgHttpHeader, headerArg)
		}
	} else {
		iip.GetLogger().Errorf("httpHeaderToIIPArg fail, %s", err.Error())
	}

	if ret, err := backend.DoRequest(uri, iip.NewDefaultRequest(bts), m.requestTimeout); err != nil {
		iip.GetLogger().Errorf("error %s , when call backend of path: %s", err.Error(), req.RequestURI)
		w.WriteHeader(http.StatusInternalServerError)
		return
	} else {
		w.WriteHeader(http.StatusOK)
		w.Write(ret)
	}
}

func (m *HttpAdapterServer) Serve(lsn net.Listener) error {
	m.httpServer = &http.Server{Addr: m.addr, Handler: m}
	if m.tlsCertFile != "" && m.tlsKeyFile != "" {
		return m.httpServer.ServeTLS(lsn, m.tlsCertFile, m.tlsKeyFile)
	} else {
		return m.httpServer.Serve(lsn)
	}
}

func (m *HttpAdapterServer) ListenAndServe() error {
	m.httpServer = &http.Server{Addr: m.addr, Handler: m}
	if m.tlsCertFile != "" && m.tlsKeyFile != "" {
		return m.httpServer.ListenAndServeTLS(m.tlsCertFile, m.tlsKeyFile)
	} else {
		return m.httpServer.ListenAndServe()
	}
}
