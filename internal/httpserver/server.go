package httpserver

import (
	"fmt"
	"net/http"
	"time"
)

// New builds an http.Server bound to host:port serving handler.
func New(host string, port int, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              fmt.Sprintf("%s:%d", host, port),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
}
