package router

import (
	"net/http"
)

func New(actions map[string]http.Handler) http.Handler {
	r := http.NewServeMux()
	r.Handle("/", actions["static"])
	r.Handle("/droplet", actions["upload_droplet"])
	return r
}
