//go:build windows

package proxy

import (
	"net/http"
	"net/url"

	"github.com/mattn/go-ieproxy"
)

func proxyFunc(httpProxy, httpsProxy, noProxy string) func(*http.Request) (*url.URL, error) {
	// 1. Explicit config takes priority
	if httpsProxy != "" {
		if u, err := url.Parse(httpsProxy); err == nil {
			return http.ProxyURL(u)
		}
	}
	if httpProxy != "" {
		if u, err := url.Parse(httpProxy); err == nil {
			return http.ProxyURL(u)
		}
	}
	// 2. Windows system proxy (IE/Edge settings via WinHTTP API + registry + PAC)
	return ieproxy.GetProxyFunc()
}
