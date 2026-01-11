package http

import "net/http"

type T struct {
	mux *http.ServeMux
}

func (t *T) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	t.mux.ServeHTTP(w, r)
}

func New() *T {
	return &T{mux: http.NewServeMux()}
}

func (t *T) handle(pattern string, handler http.Handler) {
	t.mux.Handle(pattern, handler)
}
