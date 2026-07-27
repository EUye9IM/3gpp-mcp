//go:build !windows

package proxy

import (
	"net/http"
	"net/url"
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
	// 2. Environment variables (HTTP_PROXY, HTTPS_PROXY, NO_PROXY)
	return http.ProxyFromEnvironment
}
