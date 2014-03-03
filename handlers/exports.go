package handlers

import (
	"github.com/cloudfoundry-incubator/file-server/handlers/static"
	"net/http"
)

func Exports(dir string) map[string]http.Handler {
	return map[string]http.Handler{
		"static": static.New(dir),
	}
}
