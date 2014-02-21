package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	steno "github.com/cloudfoundry/gosteno"
)

var port int
var directory string
var logLevel string

func init() {
	flag.IntVar(&port, "port", 8080, "Specifies the port of the file server")
	flag.StringVar(&directory, "directory", "", "Specifies the directory to serve")
	flag.StringVar(&logLevel, "logLevel", "info", "Logging level (none, fatal, error, warn, info, debug, debug1, debug2, all)")
}

func main() {
	flag.Parse()

	l, err := steno.GetLogLevel(logLevel)
	if err != nil {
		log.Fatalf("Invalid loglevel: %s\n", logLevel)
	}

	stenoConfig := steno.Config{
		Level: l,
		Sinks: []steno.Sink{steno.NewIOSink(os.Stdout)},
	}

	steno.Init(&stenoConfig)
	logger := steno.NewLogger("file-server")

	if directory == "" {
		logger.Error("-directory must be specified")
		os.Exit(1)
	}

	handler := &LoggingHandler{
		wrappedHandler: http.FileServer(http.Dir(directory)),
		logger:         *logger,
	}

	logger.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), handler).Error())
}

type LoggingHandler struct {
	wrappedHandler http.Handler
	logger         steno.Logger
}

func (h *LoggingHandler) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	rw := &LoggingResponseWriter{
		ResponseWriter: resp,
		status:         200,
	}

	h.wrappedHandler.ServeHTTP(rw, req)
	h.logger.Infof("Got: %s, response status %d", req.URL.String(), rw.status)
}

type LoggingResponseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *LoggingResponseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}
