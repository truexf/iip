package iip

import (
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"math/rand"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/truexf/goutil/jsonexp"
)

type LoadBalanceMethod uint32

const (
	MethodRoundrobin LoadBalanceMethod = iota
	MethodRandom
	MethodMinPending
	MethodIpHash
	MethodURIParam
	MethodJsonExp // very powerful
)

const (
	JsonExpGroupTarget     string = "LB_TARGET"
	JsonExpVarTargetServer string = "$LB_TARGET_SERVER"
	JsonExpUrlPath         string = "__PATH__"
	JsonExpObjectURI       string = "$REQUEST_URI"
)

var (
	ErrorAliasExists       = errors.New("backend's alias of load balance client exists")
	ErrorAliasNotExist     = errors.New("backend's alias not found")
	ErrorJsonExpNotFound   = errors.New("jsonexp for target select not found")
	ErrorNoServerDefined   = errors.New("no backend server defined")
	ErrorInvalidRequestURI = errors.New("invalid request URI")
	jsonExpDict            *jsonexp.Dictionary
)

type UrlValuesForJsonExp struct {
	Path      string
	UrlValues url.Values
}

// implements jsonexp.Object
func (m *UrlValuesForJsonExp) SetPropertyValue(property string, value interface{}, context jsonexp.Context) {
}

func (m *UrlValuesForJsonExp) GetPropertyValue(property string, context jsonexp.Context) interface{} {
	if property == JsonExpUrlPath {
		return m.Path
	}
	return m.UrlValues.Get(property)
}

type HealthCheck func(req Request, resp []byte, err error) bool

func DefaultHealthCheck(req Request, resp []byte, err error) bool {
	return err == nil
}

func doInit() {
	var once sync.Once
	once.Do(func() {
		jsonExpDict = jsonexp.NewDictionary()
		jsonExpDict.RegisterVar(JsonExpVarTargetServer, nil)
	})
}

type EvaluatedClient struct {
	alias                string
	blc                  *LoadBalanceClient
	paused               bool
	lastRequest          time.Time
	client               *Client
	connIndex            int
	requestErrorCount    int
	err                  error
	getChannelLock       sync.Mutex
	pendingRequests      int64
	healthCheck          HealthCheck
	healthCheckFailCount int64
}

func (m *EvaluatedClient) PendingRequests() int64 {
	return atomic.LoadInt64(&m.pendingRequests) + atomic.LoadInt64(&m.healthCheckFailCount)
}

func (m *EvaluatedClient) getTaskChannel() (*ClientChannel, error) {
	m.getChannelLock.Lock()
	defer m.getChannelLock.Unlock()

	if m.client == nil {
		return nil, fmt.Errorf("not init yet")
	}

	if m.client.connCount < int32(m.blc.serverKeepConns) {
		if conn, err := m.client.newConnection(); err != nil {
			return nil, fmt.Errorf("connect to %s fail, %s", m.client.serverAddr, err.Error())
		} else {
			if c, err := m.client.newChannel(conn); err != nil {
				m.client.removeConnection(conn)
				return nil, fmt.Errorf("new channel to %s fail, %s", m.client.serverAddr, err.Error())
			} else {
				conn.SetCtxData(CtxLblClientChannel, c)
				return c, nil
			}
		}
	} else {
		m.client.connLock.RLock()
		defer m.client.connLock.RUnlock()
		m.connIndex++
		if m.connIndex >= int(m.client.connCount) {
			m.connIndex = 0
		}
		ret := m.client.connections[m.connIndex].GetCtxData(CtxLblClientChannel)
		if ret == nil {
			return nil, fmt.Errorf("connection have no lbl-client-channel")
		}
		return ret.(*ClientChannel), nil

	}
}

func (m *EvaluatedClient) DoRequest(path string, request Request, timeout time.Duration) ([]byte, error) {
	atomic.AddInt64(&m.pendingRequests, 1)
	defer atomic.AddInt64(&m.pendingRequests, -1)

	channel, err := m.getTaskChannel()
	if err != nil {
		m.err = err
		m.paused = true
		return nil, err
	}
	m.paused = false
	m.lastRequest = time.Now()
	if ret, err := channel.DoRequest(path, request, timeout); err == nil {
		return ret, err
	} else {
		channel.Close(err)
		channel.internalChannel.conn.Close(fmt.Errorf("client do request fail, %s", err.Error()))
		return ret, fmt.Errorf("request [%s], %s", m.client.serverAddr, err.Error())
	}
}

// 由一组server提供无状态服务的场景下，LoadBalanceClient根据可配置的负载权重、keepalive检测，自动调节对不同server的请求频率
// LoadBalanceClient内部管理多个client
type LoadBalanceClient struct {
	randObj         *rand.Rand
	serverMaxConns  int
	serverKeepConns int
	activeClients   []*EvaluatedClient
	activeClientMap map[string]*EvaluatedClient
	clientsLock     sync.RWMutex

	method          LoadBalanceMethod
	roundrobinIndex int64
	methodParamKey  string

	// method MethodJsonExp
	jsonExpLock   sync.RWMutex
	jsonExpConfig *jsonexp.Configuration
}

// close all connections & channels to the server
func (m *LoadBalanceClient) CloseAll() {
	m.clientsLock.Lock()
	defer m.clientsLock.Unlock()

	for _, v := range m.activeClients {
		if v != nil && v.client != nil {
			v.client.Close()
		}
	}
}

func (m *LoadBalanceClient) AddBackend(addr string, alias string, healthCheck HealthCheck) error {
	if _, ok := m.activeClientMap[alias]; ok {
		return ErrorAliasExists
	}

	cfg := ClientConfig{MaxConnections: m.serverMaxConns,
		MaxChannelsPerConn:    3,
		ChannelPacketQueueLen: 1000,
		TcpWriteQueueLen:      1000,
		TcpConnectTimeout:     time.Second * 3,
		TcpReadBufferSize:     1024 * 32,
		TcpWriteBufferSize:    1024 * 32,
	}
	if client, err := NewClient(cfg, addr, nil); err == nil {
		m.clientsLock.Lock()
		defer m.clientsLock.Unlock()
		if healthCheck == nil {
			healthCheck = DefaultHealthCheck
		}
		backend := &EvaluatedClient{blc: m, client: client, alias: alias, healthCheck: healthCheck}
		m.activeClients = append(m.activeClients, backend)
		m.activeClientMap[alias] = backend
		return nil
	} else {
		return err
	}

}

func (m *LoadBalanceClient) RemoveBackend(alias string) error {
	m.clientsLock.Lock()
	defer m.clientsLock.Unlock()
	if _, ok := m.activeClientMap[alias]; ok {
		delete(m.activeClientMap, alias)
		for i, v := range m.activeClients {
			if v.alias == alias {
				newList := m.activeClients[:i]
				if i != len(m.activeClients)-1 {
					newList = append(newList, m.activeClients[i+1:]...)
				}
				m.activeClients = newList
			}
		}
		return nil
	} else {
		return ErrorAliasNotExist
	}
}

func (m *LoadBalanceClient) SetBalanceParamKey(key string) {
	m.methodParamKey = key
}

func (m *LoadBalanceClient) SetJsonExp(jsonExpJson []byte) error {
	config, err := jsonexp.NewConfiguration(jsonExpJson, jsonExpDict)
	if err != nil {
		return err
	}
	m.jsonExpLock.Lock()
	defer m.jsonExpLock.Unlock()
	m.jsonExpConfig = config
	return nil
}

func (m *LoadBalanceClient) SetMethod(method LoadBalanceMethod) {
	m.method = method
}

func (m *LoadBalanceClient) getJsonExp() *jsonexp.JsonExpGroup {
	m.jsonExpLock.RLock()
	defer m.jsonExpLock.RUnlock()
	if m.jsonExpConfig == nil {
		return nil
	}
	if ret, ok := m.jsonExpConfig.GetJsonExpGroup(JsonExpGroupTarget); ok {
		return ret
	} else {
		return nil
	}
}

func (m *LoadBalanceClient) selectBackendRoundrobin() (*EvaluatedClient, error) {
	idx := atomic.LoadInt64(&m.roundrobinIndex)
	atomic.AddInt64(&m.roundrobinIndex, 1)
	idx %= int64(len(m.activeClients))
	ret := m.activeClients[idx]
	return ret, nil
}

func (m *LoadBalanceClient) selectBackendRandom() (*EvaluatedClient, error) {
	idx := m.randObj.Intn(len(m.activeClients))
	ret := m.activeClients[idx]
	return ret, nil
}

func (m *LoadBalanceClient) selectBackendMinPending() (*EvaluatedClient, error) {
	idx := atomic.LoadInt64(&m.roundrobinIndex)
	atomic.AddInt64(&m.roundrobinIndex, 1)
	idx %= int64(len(m.activeClients))
	minIdx := idx
	minPending := m.activeClients[minIdx].PendingRequests()
	if minPending > 0 {
		for i := 0; i < len(m.activeClients); i++ {
			idx++
			if idx >= int64(len(m.activeClients)) {
				idx = 0
			}
			pr := m.activeClients[idx].PendingRequests()
			if pr < minPending {
				minPending = pr
				minIdx = idx
			}
		}
	}
	return m.activeClients[minIdx], nil
}

func (m *LoadBalanceClient) selectBackendIpHash(clientIp string) (*EvaluatedClient, error) {
	fnv32 := fnv.New32()
	io.WriteString(fnv32, clientIp)
	idx := fnv32.Sum32()
	idx = idx % uint32(len(m.activeClients))
	return m.activeClients[int(idx)], nil
}

func (m *LoadBalanceClient) selectBackendParam(paramValue string) (*EvaluatedClient, error) {
	fnv32 := fnv.New32()
	io.WriteString(fnv32, paramValue)
	idx := fnv32.Sum32()
	idx = idx % uint32(len(m.activeClients))
	return m.activeClients[int(idx)], nil
}

func (m *LoadBalanceClient) selectBackendJsonExp() (*EvaluatedClient, error) {
	jsonExp := m.getJsonExp()
	if jsonExp == nil {
		return nil, ErrorJsonExpNotFound
	}

	context := &jsonexp.DefaultContext{WithLock: false}
	if err := jsonExp.Execute(context); err != nil {
		return nil, err
	}
	targetAlias, ok := context.GetCtxData(JsonExpVarTargetServer)
	if !ok {
		return nil, ErrorJsonExpNotFound
	}
	targetAliasStr, _ := jsonexp.GetStringValue(targetAlias)
	if backend, ok := m.activeClientMap[targetAliasStr]; ok {
		return backend, nil
	}
	return nil, ErrorJsonExpNotFound
}

func (m *LoadBalanceClient) selectBackend(clientIp string, requestURI string) (*EvaluatedClient, error) {
	m.clientsLock.RLock()
	defer m.clientsLock.RUnlock()
	if len(m.activeClients) == 0 {
		return nil, ErrorNoServerDefined
	}
	switch m.method {
	case MethodRoundrobin:
		return m.selectBackendRoundrobin()
	case MethodRandom:
		return m.selectBackendRandom()
	case MethodMinPending:
		return m.selectBackendMinPending()
	case MethodIpHash:
		return m.selectBackendIpHash(clientIp)
	case MethodURIParam:
		if u, err := url.ParseRequestURI(requestURI); err != nil {
			return nil, ErrorInvalidRequestURI
		} else {
			paramValue := u.Query().Get(m.methodParamKey)
			return m.selectBackendParam(paramValue)
		}
	case MethodJsonExp:
		if u, err := url.ParseRequestURI(requestURI); err != nil {
			return nil, ErrorInvalidRequestURI
		} else {
			jsonExpDict.RegisterObject(JsonExpObjectURI, &UrlValuesForJsonExp{Path: u.Path, UrlValues: u.Query()})
			return m.selectBackendJsonExp()
		}
	default:
		return m.selectBackendMinPending()
	}
}

func (m *LoadBalanceClient) getTaskClient() (*EvaluatedClient, error) {
	m.clientsLock.RLock()
	defer m.clientsLock.RUnlock()
	idx := m.roundrobinIndex
	iterCnt := 0
	var minN int64 = 0
	var minC *EvaluatedClient = nil
	for {
		iterCnt++
		idx++
		if int(idx) >= len(m.activeClients) {
			idx = 0
		}
		m.roundrobinIndex = idx
		if !m.activeClients[idx].paused || time.Since(m.activeClients[idx].lastRequest) > time.Second*5 {
			if minC == nil {
				minC = m.activeClients[idx]
				minN = minC.PendingRequests()
			} else {
				pr := m.activeClients[idx].PendingRequests()
				if minN > pr {
					minC = m.activeClients[idx]
					minN = pr
				}
			}
			if minN == 0 {
				break
			}
		}
		if iterCnt >= len(m.activeClients) {
			break
		}
	}
	if minC == nil {
		log.Error("no alived client")
		return nil, fmt.Errorf("no alived client")
	} else {
		return minC, nil
	}
}

func (m *LoadBalanceClient) Status() string {
	m.clientsLock.RLock()
	defer m.clientsLock.RUnlock()
	ret := ""
	for _, v := range m.activeClients {
		ret += fmt.Sprintf("%s\n", v.client.serverAddr)
	}
	return ret
}

func (m *LoadBalanceClient) GetPendingRequests() ([]byte, error) {
	ret := make(map[string]int64)
	m.clientsLock.RLock()
	defer m.clientsLock.RUnlock()
	for _, ec := range m.activeClients {
		ret[ec.client.serverAddr] += ec.PendingRequests()
	}
	return json.Marshal(ret)
}

func (m *LoadBalanceClient) GetConnectionStatis() ([]byte, error) {
	retFinal := make(map[string]map[int]*ConnectionSatis)
	m.clientsLock.RLock()
	defer m.clientsLock.RUnlock()
	for _, ec := range m.activeClients {
		c := ec.client
		conns := make(map[int]*Connection)
		c.connLock.Lock()
		for i, v := range c.connections {
			conns[i] = v
		}
		c.connLock.Unlock()

		ret := make(map[int]*ConnectionSatis)
		for i, v := range conns {
			rec := &ConnectionSatis{WriteQueue: len(v.tcpWriteQueue), Count: v.Count, Channels: make(map[uint32]struct {
				ReceiveQueue int
				Count        *Count
			})}
			v.ChannelsLock.RLock()
			for cId, c := range v.Channels {
				rec.Channels[cId] = struct {
					ReceiveQueue int
					Count        *Count
				}{ReceiveQueue: len(c.receivedQueue), Count: c.Count}
			}
			v.ChannelsLock.RUnlock()
			ret[i] = rec
		}

		retFinal[c.serverAddr] = ret
	}
	return json.Marshal(retFinal)
}

func (m *LoadBalanceClient) DoRequest2(clientIp string, path string, request Request, timeout time.Duration) ([]byte, error) {
	backend, err := m.selectBackend(clientIp, path)
	if err != nil {
		return nil, err
	}

	atomic.AddInt64(&backend.pendingRequests, 1)
	ret, err := backend.DoRequest(path, request, timeout)
	atomic.AddInt64(&backend.pendingRequests, -1)
	healthCheck := backend.healthCheck
	if healthCheck == nil {
		healthCheck = DefaultHealthCheck
	}
	if !healthCheck(request, ret, err) {
		atomic.AddInt64(&backend.healthCheckFailCount, 1)
	} else {
		atomic.StoreInt64(&backend.healthCheckFailCount, backend.healthCheckFailCount/2)
	}

	return ret, err
}

func (m *LoadBalanceClient) DoRequest(path string, request Request, timeout time.Duration) ([]byte, error) {
	doInit()
	client, err := m.getTaskClient()
	if err != nil {
		return nil, err
	}
	ret, err := client.DoRequest(path, request, timeout)
	if err != nil {
		client.requestErrorCount++
		if client.requestErrorCount > 5 {
			log.Errorf("[%s] error 5+ times, paused, latest error: %s", client.client.serverAddr, err.Error())
			client.paused = true
		}
	} else {
		if client.requestErrorCount > 0 {
			client.requestErrorCount = 0
		}
		if client.requestErrorCount == 0 && client.paused {
			client.err = nil
			client.paused = false
		}
	}
	return ret, err
}

type AddrWeightClient struct {
	Addr   string
	Weight int
	client *Client
}

// 创建一个LoadBalanceClient，
// severList格式：ip:port#weight,ip:port#weight,ip:port#weight,...，
// 0<weight<100,表明server的权重。weight大于1，相当于增加weight-1个相同地址的server
// 每个server地址保持serverKeepConns个活跃连接，如果并发突增超过serverKeepConns，则自动创建新连接。
// serverMaxConns指定单个server最大连接数，所有的server都超出serverMaxConns，则返回ErrClientConnectionsLimited错误,否则以其他有空的server代其服务
// server任务分配以轮转模式运行
// 由于load balance客户端一般式针对同类型的业务，因此没有必要区分多个channel,内部每个connection开启一个channel.
func NewLoadBalanceClient(serverKeepConns int, serverMaxConns int, serverList string) (*LoadBalanceClient, error) {
	log.Debugf("new iip blc, serverlist: %s", serverList)
	svrs := strings.Split(serverList, ",")
	var addrList []*AddrWeightClient
	for _, v := range svrs {
		s := strings.TrimSpace(v)
		if s == "" {
			continue
		}
		aw := strings.Split(s, "#")
		if len(aw) != 2 {
			continue
		}
		if aw[0] == "" {
			continue
		}
		if weight, err := strconv.Atoi(aw[1]); err != nil {
			continue
		} else {
			if weight < 1 || weight > 100 {
				continue
			}
			addrList = append(addrList, &AddrWeightClient{Addr: aw[0], Weight: weight})
		}
	}
	if len(addrList) == 0 {
		return nil, fmt.Errorf("invalid param: serverList")
	}

	var addrListFinal []*AddrWeightClient
	for _, v := range addrList {
		cfg := ClientConfig{MaxConnections: serverMaxConns,
			MaxChannelsPerConn:    3,
			ChannelPacketQueueLen: 1000,
			TcpWriteQueueLen:      1000,
			TcpConnectTimeout:     time.Second * 3,
			TcpReadBufferSize:     1024 * 32,
			TcpWriteBufferSize:    1024 * 32,
		}
		if client, err := NewClient(cfg, v.Addr, nil); err == nil {
			v.client = client
			addrListFinal = append(addrListFinal, v)
		}
	}

	ret := &LoadBalanceClient{serverMaxConns: serverMaxConns, serverKeepConns: serverKeepConns, method: MethodMinPending}
	ret.randObj = rand.New(rand.NewSource(time.Now().UnixNano()))
	ret.activeClientMap = make(map[string]*EvaluatedClient)
	if ret.serverKeepConns < 1 {
		ret.serverKeepConns = 1
	}
	if ret.serverMaxConns < 1 {
		ret.serverMaxConns = 1
	}

	exists := false
	j := 0
	for {
		for i, v := range addrListFinal {
			if v.Weight > 0 {
				v.Weight--
				ret.activeClients = append(ret.activeClients, &EvaluatedClient{client: v.client, blc: ret, alias: fmt.Sprintf("%s-%d-%d", v.client.serverAddr, i, j)})
				if v.Weight > 0 {
					exists = true
				}
			}
		}
		if !exists {
			break
		} else {
			exists = false
		}
	}
	if log.GetLevel() == LogLevelDebug {
		log.Debug(ret.Status())
	}
	return ret, nil
}
