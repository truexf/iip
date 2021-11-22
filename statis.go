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

type RangeStatis struct {
	Duration int64
	Count    int64
}

func (m *RangeStatis) Add(duration, count int64) {
	atomic.AddInt64(&m.Duration, duration)
	atomic.AddInt64(&m.Count, count)
}

func (m *RangeStatis) AddObj(rs RangeStatis) {
	atomic.AddInt64(&m.Duration, rs.Duration)
	atomic.AddInt64(&m.Count, rs.Count)
}

// 分区间统计
type TimeCount struct {
	sync.Mutex
	RangeCount [8]RangeStatis `json:"range_count"`
}

func (m *TimeCount) Clear() {
	m.Lock()
	defer m.Unlock()
	for i := range m.RangeCount {
		m.RangeCount[i] = RangeStatis{}
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
		m.RangeCount[0].Add(duration, 1)
	case TimeRange2:
		m.RangeCount[1].Add(duration, 1)
	case TimeRange3:
		m.RangeCount[2].Add(duration, 1)
	case TimeRange4:
		m.RangeCount[3].Add(duration, 1)
	case TimeRange5:
		m.RangeCount[4].Add(duration, 1)
	case TimeRange6:
		m.RangeCount[5].Add(duration, 1)
	case TimeRange7:
		m.RangeCount[6].Add(duration, 1)
	default:
		m.RangeCount[7].Add(duration, 1)
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

	if ret, err := json.Marshal(m); err == nil {
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
5 minute timecount    : range1(%d,%d%%, average %d), range2(%d,%d%%, average %d), range3(%d,%d%%, average %d), range4(%d,%d%%, average %d), range5(%d,%d%%, average %d), range6(%d,%d%%, average %d), range7(%d,%d%%, average %d), other(%d,%d%%, average %d)
all timecount         : range1(%d,%d%%, average %d), range2(%d,%d%%, average %d), range3(%d,%d%%, average %d), range4(%d,%d%%, average %d), range5(%d,%d%%, average %d), range6(%d,%d%%, average %d), range7(%d,%d%%, average %d), other(%d,%d%%, average %d)
`
	idx := m.currentSecond
	var dur5 int64
	var req5 int64
	var rangeCount1 RangeStatis
	var rangeCount2 RangeStatis
	var rangeCount3 RangeStatis
	var rangeCount4 RangeStatis
	var rangeCount5 RangeStatis
	var rangeCount6 RangeStatis
	var rangeCount7 RangeStatis
	var rangeCountOther RangeStatis
	var allRangeCount1 RangeStatis
	var allRangeCount2 RangeStatis
	var allRangeCount3 RangeStatis
	var allRangeCount4 RangeStatis
	var allRangeCount5 RangeStatis
	var allRangeCount6 RangeStatis
	var allRangeCount7 RangeStatis
	var allRangeCountOther RangeStatis

	for i := 0; i < 300; i++ {
		dur5 += m.FiveMinuteDuration[idx]
		req5 += m.FiveMinuteRequests[idx]
		m.FiveMinuteTimeCount[idx].Lock()
		rangeCount1.AddObj(m.FiveMinuteTimeCount[idx].RangeCount[0])
		rangeCount2.AddObj(m.FiveMinuteTimeCount[idx].RangeCount[1])
		rangeCount3.AddObj(m.FiveMinuteTimeCount[idx].RangeCount[2])
		rangeCount4.AddObj(m.FiveMinuteTimeCount[idx].RangeCount[3])
		rangeCount5.AddObj(m.FiveMinuteTimeCount[idx].RangeCount[4])
		rangeCount6.AddObj(m.FiveMinuteTimeCount[idx].RangeCount[5])
		rangeCount7.AddObj(m.FiveMinuteTimeCount[idx].RangeCount[6])
		rangeCountOther.AddObj(m.FiveMinuteTimeCount[idx].RangeCount[7])
		m.FiveMinuteTimeCount[idx].Unlock()

		idx++
		if idx >= 300 {
			idx = 0
		}
	}

	m.AllTimeCount.Lock()
	allRangeCount1.AddObj(m.AllTimeCount.RangeCount[0])
	allRangeCount2.AddObj(m.AllTimeCount.RangeCount[1])
	allRangeCount3.AddObj(m.AllTimeCount.RangeCount[2])
	allRangeCount4.AddObj(m.AllTimeCount.RangeCount[3])
	allRangeCount5.AddObj(m.AllTimeCount.RangeCount[4])
	allRangeCount6.AddObj(m.AllTimeCount.RangeCount[5])
	allRangeCount7.AddObj(m.AllTimeCount.RangeCount[6])
	allRangeCountOther.AddObj(m.AllTimeCount.RangeCount[7])
	m.AllTimeCount.Unlock()

	AllTimeCount := allRangeCount1.Count + allRangeCount2.Count + allRangeCount3.Count + allRangeCount4.Count + allRangeCount5.Count + allRangeCount6.Count + allRangeCount7.Count + allRangeCountOther.Count
	fTimeCount := rangeCount1.Count + rangeCount2.Count + rangeCount3.Count + rangeCount4.Count + rangeCount5.Count + rangeCount6.Count + rangeCount7.Count + rangeCountOther.Count
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

	// averageDur5 := dur5 / req5
	reqAll := m.AllRequests
	if reqAll <= 0 {
		reqAll = 1
	}
	averageDurAll := m.AllDuration / reqAll

	return fmt.Sprintf(ret, req5Ori, dur5/fTimeCount/int64(timeUnit), m.AllRequests, averageDurAll/int64(timeUnit),
		rangeCount1.Count, rangeCount1.Count*100/fTimeCount, rangeCount1.Duration/rangeCount1.Count,
		rangeCount2.Count, rangeCount2.Count*100/fTimeCount, rangeCount2.Duration/rangeCount2.Count,
		rangeCount3.Count, rangeCount3.Count*100/fTimeCount, rangeCount3.Duration/rangeCount3.Count,
		rangeCount4.Count, rangeCount4.Count*100/fTimeCount, rangeCount4.Duration/rangeCount4.Count,
		rangeCount5.Count, rangeCount5.Count*100/fTimeCount, rangeCount5.Duration/rangeCount5.Count,
		rangeCount6.Count, rangeCount6.Count*100/fTimeCount, rangeCount6.Duration/rangeCount6.Count,
		rangeCount7.Count, rangeCount7.Count*100/fTimeCount, rangeCount7.Duration/rangeCount7.Count,
		rangeCountOther.Count, rangeCountOther.Count*100/fTimeCount, rangeCountOther.Duration/rangeCountOther.Count,

		allRangeCount1.Count, allRangeCount1.Count*100/AllTimeCount, rangeCount1.Duration/rangeCount1.Count,
		allRangeCount2.Count, allRangeCount2.Count*100/AllTimeCount, rangeCount2.Duration/rangeCount2.Count,
		allRangeCount3.Count, allRangeCount3.Count*100/AllTimeCount, rangeCount3.Duration/rangeCount3.Count,
		allRangeCount4.Count, allRangeCount4.Count*100/AllTimeCount, rangeCount4.Duration/rangeCount4.Count,
		allRangeCount5.Count, allRangeCount5.Count*100/AllTimeCount, rangeCount5.Duration/rangeCount5.Count,
		allRangeCount6.Count, allRangeCount6.Count*100/AllTimeCount, rangeCount6.Duration/rangeCount6.Count,
		allRangeCount7.Count, allRangeCount7.Count*100/AllTimeCount, rangeCount7.Duration/rangeCount7.Count,
		allRangeCountOther.Count, allRangeCountOther.Count*100/AllTimeCount, rangeCountOther.Duration/rangeCountOther.Count,
	)
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

type ConnectionSatis struct {
	Channels map[uint32]struct {
		ReceiveQueue int
		Count        *Count
	}
	WriteQueue int
	Count      *Count
}
