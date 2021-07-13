// Copyright 2021 fangyousong(方友松). All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

//定义logger接口，并提供默认实现：输出到stdout,stderr
package iip

import (
	"fmt"
	"os"
)

type Logger interface {
	Log(s string)
	Logf(format string, args ...interface{})
	Warn(s string)
	Warnf(format string, args ...interface{})
	Error(s string)
	Errorf(format string, args ...interface{})
}

type DefaultLogger struct {
}

func (m *DefaultLogger) Log(s string) {
	fmt.Print(s)
	if len(s) == 0 || s[len(s)-1] != '\n' {
		fmt.Println("")
	}
}
func (m *DefaultLogger) Logf(format string, args ...interface{}) {
	s := fmt.Sprintf(format, args...)
	if len(s) == 0 || s[len(s)-1] != '\n' {
		s += "\n"
	}
	fmt.Print(s)

}
func (m *DefaultLogger) Warn(s string) {
	fmt.Print(s)
	if len(s) == 0 || s[len(s)-1] != '\n' {
		fmt.Println("")
	}
}
func (m *DefaultLogger) Warnf(format string, args ...interface{}) {
	m.Logf(format, args...)
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
