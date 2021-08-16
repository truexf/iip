// Copyright 2021 fangyousong(方友松). All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package httpadapter

import (
	"fmt"
	"net/url"

	"github.com/truexf/goutil/jsonexp"
)

type JsonExpRoute struct {
	jeDict         *jsonexp.Dictionary
	jeConfig       *jsonexp.Configuration
	routeGroupName string
}

// Json expression route, see https://github.com/truexf/goutil/blob/master/jsonexp/README.md
func NewJsonExpRoute(configJson []byte, routeJsonExpGroupName string) (*JsonExpRoute, error) {
	if routeJsonExpGroupName == "" {
		return nil, fmt.Errorf("routeJsonExpGroupName is empty")
	}
	dict := jsonexp.NewDictionary()
	conf, err := jsonexp.NewConfiguration(configJson, dict)
	if err != nil {
		return nil, err
	}
	dict.RegisterVar("$host", nil)
	dict.RegisterVar("$uri", nil)
	dict.RegisterVar("$iip_backend", nil)
	return &JsonExpRoute{jeDict: dict, jeConfig: conf, routeGroupName: routeJsonExpGroupName}, nil
}

type UrlQueryParams map[string]string

func (m UrlQueryParams) GetPropertyValue(PropertyName string, context jsonexp.Context) interface{} {
	if ret, ok := m[PropertyName]; ok {
		return ret
	}
	return nil
}

func (m UrlQueryParams) SetPropertyValue(property string, value interface{}, context jsonexp.Context) {
	if s, ok := jsonexp.GetStringValue(value); ok {
		m[property] = s
	}
}

func (m *JsonExpRoute) GetBackendAlias(host, uri string) string {
	if g, ok := m.jeConfig.GetJsonExpGroup(m.routeGroupName); ok {
		ctx := &jsonexp.DefaultContext{}
		ctx.SetCtxData("$host", host)
		ctx.SetCtxData("$uri", uri)
		if u, err := url.ParseRequestURI(uri); err == nil {
			if urlValues, err := url.ParseQuery(u.RawQuery); err == nil {
				params := make(UrlQueryParams)
				m.jeDict.RegisterObject("$query", params)
				for k, v := range urlValues {
					if len(v) > 0 {
						params[k] = v[0]
					}
				}
			}
		}
		if err := g.Execute(ctx); err == nil {
			if ret, ok := ctx.GetCtxData("$iip_backend"); ok {
				if retStr, ok := jsonexp.GetStringValue(ret); ok {
					return retStr
				}
			}
		}
	}
	return ""
}
