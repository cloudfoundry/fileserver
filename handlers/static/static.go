package static

import (
	"github.com/gorilla/handlers"
	"net/http"
	"os"
)

func New(dir, pathPrefix string) http.Handler {
	fileServer := http.FileServer(http.Dir(dir))
	stripped := http.StripPrefix(pathPrefix, fileServer)
	return handlers.LoggingHandler(os.Stdout, stripped)
}
