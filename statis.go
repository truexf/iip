// Copyright 2021 fangyousong(方友松). All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// channel, connection, client, server的状态统计信息
package iip

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

//耗时区间
type TimeRange int64

const (
	TimeRange1 TimeRange = iota
	TimeRange2
	TimeRange3
	TimeRange4
	TimeRange5
	TimeRange6
	TimeRange7
	TimeRangeOther
)

type EnsureTimeRangeFunc = func(duration int64) TimeRange

func ensureTimeRangeDefault(v int64) TimeRange {
	if v <= 100 {
		return TimeRange1
	} else if v <= 200 {
		return TimeRange2
	} else if v <= 300 {
		return TimeRange3
	} else if v <= 500 {
		return TimeRange4
	} else if v <= 700 {
		return TimeRange5
	} else if v <= 1000 {
		return TimeRange6
	} else if v <= 1500 {
		return TimeRange7
	} else {
		return TimeRangeOther
	}
}

func EnsureTimeRangeMicroSecond(duration int64) TimeRange {
	ms := duration / int64(time.Microsecond)
	return ensureTimeRangeDefault(ms)
}

func EnsureTimeRangeMilliSecond(duration int64) TimeRange {
	ms := duration / int64(time.Millisecond)
	return ensureTimeRangeDefault(ms)
}

func EnsureTimeRangeSecond(duration int64) TimeRange {
	ms := duration / int64(time.Second)
	return ensureTimeRangeDefault(ms)
}

// 分区间统计
type TimeCount struct {
	sync.Mutex
	RangeCount [8]int64 `json:"range_count"`
}

func (m *TimeCount) Clear() {
	m.Lock()
	defer m.Unlock()
	for i := range m.RangeCount {
		m.RangeCount[i] = 0
	}
}

func (m *TimeCount) Record(duration int64, ensureFunc EnsureTimeRangeFunc) {
	if ensureFunc == nil {
		return
	}
	m.Lock()
	defer m.Unlock()
	r := ensureFunc(duration)
	switch r {
	case TimeRange1:
		m.RangeCount[0] += duration
	case TimeRange2:
		m.RangeCount[1] += duration
	case TimeRange3:
		m.RangeCount[2] += duration
	case TimeRange4:
		m.RangeCount[3] += duration
	case TimeRange5:
		m.RangeCount[4] += duration
	case TimeRange6:
		m.RangeCount[5] += duration
	case TimeRange7:
		m.RangeCount[6] += duration
	default:
		m.RangeCount[7] += duration
	}

}

// 分“最近5分钟”和“运行以来全部时间”两个维度的性能测量统计
type Measure struct {
	sync.RWMutex
	FiveMinuteRequests  [301]int64      `json:"five_minute_requests"`  //最近5分钟的请求数
	FiveMinuteDuration  [301]int64      `json:"five_minute_duration"`  //最近5分钟的请求处理耗时
	AllRequests         int64           `json:"all_requests"`          //全部请求数
	AllDuration         int64           `json:"all_duration"`          //全部请求处理耗时
	FiveMinuteTimeCount [301]*TimeCount `json:"five_minute_timecount"` //最近5分钟请求的耗时区间分段统计
	AllTimeCount        *TimeCount      `json:"all_timecount"`         //全部请求的耗时区间分段统计
	currentSecond       int
	timeRangeEnsureFunc EnsureTimeRangeFunc
}

// timeUnit: 时间单位粒度(μs/ms) 1秒=1000ms=1000000μs
func NewMesure(timeRangeEnsureFunc EnsureTimeRangeFunc) *Measure {
	ret := new(Measure)
	ret.timeRangeEnsureFunc = timeRangeEnsureFunc
	if ret.timeRangeEnsureFunc == nil {
		ret.timeRangeEnsureFunc = EnsureTimeRangeMilliSecond
	}
	for i := 0; i < len(ret.FiveMinuteTimeCount); i++ {
		ret.FiveMinuteTimeCount[i] = new(TimeCount)
	}
	ret.AllTimeCount = new(TimeCount)
	go ret.tickTack()
	return ret
}
func (m *Measure) SetTimeRangeFunc(v EnsureTimeRangeFunc) {
	if v != nil {
		m.timeRangeEnsureFunc = v
	}
}
func (m *Measure) tickTack() {
	tm := time.Now().Second()
	for {
		<-time.After(time.Millisecond * 100)
		tmNow := time.Now().Second()
		idx := m.currentSecond
		if tmNow != tm {
			idx++
			if idx > 300 {
				idx = 0
			}
			m.currentSecond = idx
			idxNext := idx + 1
			if idxNext > 300 {
				idxNext = 0
			}
			atomic.StoreInt64(&(m.FiveMinuteRequests[idxNext]), 0)
			atomic.StoreInt64(&(m.FiveMinuteDuration[idxNext]), 0)
			m.FiveMinuteTimeCount[idxNext].Clear()

			tm = tmNow
		}
	}
}

func (m *Measure) Add(cnt int64, dur time.Duration) {
	m.RLock()
	defer m.RUnlock()

	idx := m.currentSecond
	atomic.AddInt64(&(m.FiveMinuteRequests[idx]), cnt)
	atomic.AddInt64(&m.AllRequests, cnt)
	atomic.AddInt64(&(m.FiveMinuteDuration[idx]), int64(dur))
	atomic.AddInt64(&m.AllDuration, int64(dur))
	m.FiveMinuteTimeCount[idx].Record(int64(dur), m.timeRangeEnsureFunc)
	m.AllTimeCount.Record(int64(dur), m.timeRangeEnsureFunc)
}

func (m *Measure) Json() []byte {
	// 参见String()的注释
	m.Lock()
	defer m.Unlock()

	if ret, err := json.MarshalIndent(m, "", "    "); err == nil {
		return ret
	}
	return nil
}

// timeUnit: time.Second,time.MicroSecond, time.MilliSecond,etc.
func (m *Measure) String(timeUnit time.Duration) string {
	// 这里用rlock看起来很奇怪：rwlock.lock主要是用于json marshal，而更频繁发生Add用rwlock.rlock, Add()内部用了atomic.
	// 这个解释可能也很怪，还是看代码吧:)
	m.RLock()
	defer m.RUnlock()

	ret := `
5 minute requests     : %d, average duration: %d
all requests          : %d, average duration: %d
5 minute timecount    : range1(%d,%d%%), range2(%d,%d%%), range3(%d,%d%%), range4(%d,%d%%), range5(%d,%d%%), range6(%d,%d%%), range7(%d,%d%%), other(%d,%d%%)
all timecount         : range1(%d,%d%%), range2(%d,%d%%), range3(%d,%d%%), range4(%d,%d%%), range5(%d,%d%%), range6(%d,%d%%), range7(%d,%d%%), other(%d,%d%%)
`
	idx := m.currentSecond
	var dur5 int64
	var req5 int64
	var rangeCount1 int64 = 0
	var rangeCount2 int64 = 0
	var rangeCount3 int64 = 0
	var rangeCount4 int64 = 0
	var rangeCount5 int64 = 0
	var rangeCount6 int64 = 0
	var rangeCount7 int64 = 0
	var rangeCountOther int64 = 0
	var allRangeCount1 int64 = 0
	var allRangeCount2 int64 = 0
	var allRangeCount3 int64 = 0
	var allRangeCount4 int64 = 0
	var allRangeCount5 int64 = 0
	var allRangeCount6 int64 = 0
	var allRangeCount7 int64 = 0
	var allRangeCountOther int64 = 0

	for i := 0; i < 300; i++ {
		dur5 += m.FiveMinuteDuration[idx]
		req5 += m.FiveMinuteRequests[idx]
		m.FiveMinuteTimeCount[idx].Lock()
		rangeCount1 += m.FiveMinuteTimeCount[idx].RangeCount[0]
		rangeCount2 += m.FiveMinuteTimeCount[idx].RangeCount[1]
		rangeCount3 += m.FiveMinuteTimeCount[idx].RangeCount[2]
		rangeCount4 += m.FiveMinuteTimeCount[idx].RangeCount[3]
		rangeCount5 += m.FiveMinuteTimeCount[idx].RangeCount[4]
		rangeCount6 += m.FiveMinuteTimeCount[idx].RangeCount[5]
		rangeCount7 += m.FiveMinuteTimeCount[idx].RangeCount[6]
		rangeCountOther += m.FiveMinuteTimeCount[idx].RangeCount[7]
		m.FiveMinuteTimeCount[idx].Unlock()

		idx++
		if idx >= 300 {
			idx = 0
		}
	}

	m.AllTimeCount.Lock()
	allRangeCount1 += m.AllTimeCount.RangeCount[0]
	allRangeCount2 += m.AllTimeCount.RangeCount[1]
	allRangeCount3 += m.AllTimeCount.RangeCount[2]
	allRangeCount4 += m.AllTimeCount.RangeCount[3]
	allRangeCount5 += m.AllTimeCount.RangeCount[4]
	allRangeCount6 += m.AllTimeCount.RangeCount[5]
	allRangeCount7 += m.AllTimeCount.RangeCount[6]
	allRangeCountOther += m.AllTimeCount.RangeCount[7]
	m.AllTimeCount.Unlock()

	AllTimeCount := allRangeCount1 + allRangeCount2 + allRangeCount3 + allRangeCount4 + allRangeCount5 + allRangeCount6 + allRangeCount7 + allRangeCountOther
	fTimeCount := rangeCount1 + rangeCount2 + rangeCount3 + rangeCount4 + rangeCount5 + rangeCount6 + rangeCount7 + rangeCountOther
	if AllTimeCount == 0 {
		AllTimeCount = 1
	}
	if fTimeCount == 0 {
		fTimeCount = 1
	}

	req5Ori := req5
	if req5 <= 0 {
		req5 = 1
	}

	averageDur5 := dur5 / req5
	reqAll := m.AllRequests
	if reqAll <= 0 {
		reqAll = 1
	}
	averageDurAll := m.AllDuration / reqAll

	return fmt.Sprintf(ret, req5Ori, averageDur5/fTimeCount/int64(timeUnit), m.AllRequests, averageDurAll/int64(timeUnit),
		rangeCount1, rangeCount1*100/fTimeCount,
		rangeCount2, rangeCount2*100/fTimeCount,
		rangeCount3, rangeCount3*100/fTimeCount,
		rangeCount4, rangeCount4*100/fTimeCount,
		rangeCount5, rangeCount5*100/fTimeCount,
		rangeCount6, rangeCount6*100/fTimeCount,
		rangeCount7, rangeCount7*100/fTimeCount,
		rangeCountOther, rangeCountOther*100/fTimeCount,

		allRangeCount1, allRangeCount1*100/AllTimeCount,
		allRangeCount2, allRangeCount2*100/AllTimeCount,
		allRangeCount3, allRangeCount3*100/AllTimeCount,
		allRangeCount4, allRangeCount4*100/AllTimeCount,
		allRangeCount5, allRangeCount5*100/AllTimeCount,
		allRangeCount6, allRangeCount6*100/AllTimeCount,
		allRangeCount7, allRangeCount7*100/AllTimeCount,
		allRangeCountOther, allRangeCountOther*100/AllTimeCount)
}

type Count struct {
	PacketsSent         int64
	PacketReceived      int64
	WholePacketSent     int64
	WholePacketReceived int64
	BytesSent           int64
	BytesReceived       int64
}

func (m *Count) AddPtr(v *Count) {
	if v == nil {
		return
	}
	atomic.AddInt64(&m.PacketsSent, v.PacketsSent)
	atomic.AddInt64(&m.PacketReceived, v.PacketReceived)
	atomic.AddInt64(&m.WholePacketSent, v.WholePacketSent)
	atomic.AddInt64(&m.WholePacketReceived, v.WholePacketReceived)
	atomic.AddInt64(&m.BytesSent, v.BytesSent)
	atomic.AddInt64(&m.BytesReceived, v.BytesReceived)

}
func (m *Count) Add(v Count) {
	atomic.AddInt64(&m.PacketsSent, v.PacketsSent)
	atomic.AddInt64(&m.PacketReceived, v.PacketReceived)
	atomic.AddInt64(&m.WholePacketSent, v.WholePacketSent)
	atomic.AddInt64(&m.WholePacketReceived, v.WholePacketReceived)
	atomic.AddInt64(&m.BytesSent, v.BytesSent)
	atomic.AddInt64(&m.BytesReceived, v.BytesReceived)
}
