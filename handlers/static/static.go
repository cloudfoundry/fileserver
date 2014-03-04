package static

import (
	"github.com/gorilla/handlers"
	"net/http"
	"os"
)

func New(dir string) http.Handler {
	fileServer := http.FileServer(http.Dir(dir))
	stripped := http.StripPrefix("/static", fileServer)
	return handlers.LoggingHandler(os.Stdout, stripped)
}
