package server

import (
	"net/http"
)

func (*Controller) healthz(w http.ResponseWriter, _ *http.Request) {
	w.Write([]byte("server is ready"))
}
