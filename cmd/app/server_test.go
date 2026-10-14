package app

import "net/http"

func newTestServer(addr string) *http.Server {
	return &http.Server{Addr: addr, Handler: http.NotFoundHandler()}
}
