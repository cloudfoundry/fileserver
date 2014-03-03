package router

import (
	"net/http"
)

func New(actions map[string]http.Handler) http.Handler {
	r := http.NewServeMux()
	r.Handle("/", actions["static"])
	return r
}
