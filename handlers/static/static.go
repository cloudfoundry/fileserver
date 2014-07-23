package static

import (
	"net/http"

	"github.com/pivotal-golang/lager"
)

func New(dir, pathPrefix string, logger lager.Logger) http.Handler {
	fileServer := http.FileServer(http.Dir(dir))
	stripped := http.StripPrefix(pathPrefix, fileServer)
	return loggingHandler{
		logger:          logger,
		originalHandler: stripped,
	}
}
