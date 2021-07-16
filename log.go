// Copyright 2021 fangyousong(方友松). All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

//定义logger接口，并提供默认实现：输出到stdout,stderr
package iip

import (
	"fmt"
	"os"
)

type LogLevel int

const (
	LogLevelError LogLevel = iota
	LoglevelWarn
	LogLevelLog
	LogLevelDebug
)

type Logger interface {
	GetLevel() LogLevel
	SetLevel(v LogLevel)
	Error(s string)
	Errorf(format string, args ...interface{})
	Warn(s string)
	Warnf(format string, args ...interface{})
	Log(s string)
	Logf(format string, args ...interface{})
	Debug(s string)
	Debugf(format string, args ...interface{})
}

type DefaultLogger struct {
	level LogLevel
}

func (m *DefaultLogger) GetLevel() LogLevel {
	return m.level
}

func (m *DefaultLogger) SetLevel(v LogLevel) {
	m.level = v
}

func (m *DefaultLogger) Debug(s string) {
	if m.level < LogLevelLog {
		return
	}
	fmt.Print(s)
	if len(s) == 0 || s[len(s)-1] != '\n' {
		fmt.Println("")
	}
}

func (m *DefaultLogger) Debugf(format string, args ...interface{}) {
	if m.level < LogLevelLog {
		return
	}
	s := fmt.Sprintf(format, args...)
	if len(s) == 0 || s[len(s)-1] != '\n' {
		s += "\n"
	}
	fmt.Print(s)

}

func (m *DefaultLogger) Log(s string) {
	if m.level < LogLevelLog {
		return
	}
	fmt.Print(s)
	if len(s) == 0 || s[len(s)-1] != '\n' {
		fmt.Println("")
	}
}
func (m *DefaultLogger) Logf(format string, args ...interface{}) {
	if m.level < LogLevelLog {
		return
	}
	s := fmt.Sprintf(format, args...)
	if len(s) == 0 || s[len(s)-1] != '\n' {
		s += "\n"
	}
	fmt.Print(s)

}
func (m *DefaultLogger) Warn(s string) {
	if m.level < LoglevelWarn {
		return
	}
	if len(s) == 0 || s[len(s)-1] != '\n' {
		s += "\n"
	}
	os.Stderr.WriteString(s)
}
func (m *DefaultLogger) Warnf(format string, args ...interface{}) {
	if m.level < LoglevelWarn {
		return
	}
	s := fmt.Sprintf(format, args...)
	if len(s) == 0 || s[len(s)-1] != '\n' {
		s += "\n"
	}
	os.Stderr.WriteString(s)
}
func (m *DefaultLogger) Error(s string) {
	os.Stderr.WriteString(s)
	if len(s) == 0 || s[len(s)-1] != '\n' {
		os.Stderr.WriteString("\n")
	}

}
func (m *DefaultLogger) Errorf(format string, args ...interface{}) {
	s := fmt.Sprintf(format, args...)
	if len(s) == 0 || s[len(s)-1] != '\n' {
		s += "\n"
	}
	os.Stderr.WriteString(s)
}

var log Logger = &DefaultLogger{}

func SetLogger(logger Logger) {
	log = logger
}
