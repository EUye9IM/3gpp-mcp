package proxy

import (
	"net/http"
	"time"
)

// NewHTTPClient creates an HTTP client with proxy support.
// Proxy detection is platform-specific (see proxy_windows.go / proxy_default.go).
func NewHTTPClient(timeout time.Duration, httpProxy, httpsProxy, noProxy string) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy: proxyFunc(httpProxy, httpsProxy, noProxy),
		},
	}
}
