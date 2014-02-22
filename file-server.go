package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	Bbs "github.com/cloudfoundry-incubator/runtime-schema/bbs"
	steno "github.com/cloudfoundry/gosteno"
	"github.com/cloudfoundry/storeadapter/etcdstoreadapter"
	"github.com/cloudfoundry/storeadapter/workerpool"
	uuid "github.com/nu7hatch/gouuid"
)

var address string
var port int
var directory string
var logLevel string
var etcdMachines string
var heartbeatInterval uint64

var presence *Bbs.Presence

func init() {
	flag.StringVar(&address, "address", "127.0.0.1", "Specifies the address to bind to")
	flag.IntVar(&port, "port", 8080, "Specifies the port of the file server")
	flag.StringVar(&directory, "directory", "", "Specifies the directory to serve")
	flag.StringVar(&logLevel, "logLevel", "info", "Logging level (none, fatal, error, warn, info, debug, debug1, debug2, all)")
	flag.StringVar(&etcdMachines, "etcdMachines", "http://127.0.0.1:4001", "comma-separated list of etcd addresses (http://ip:port)")
	flag.Uint64Var(&heartbeatInterval, "heartbeatInterval", 60, "the interval, in seconds, between heartbeats for maintaining presence")
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

	etcdAdapter := etcdstoreadapter.NewETCDStoreAdapter(
		strings.Split(etcdMachines, ","),
		workerpool.NewWorkerPool(10),
	)

	err = etcdAdapter.Connect()
	if err != nil {
		log.Fatalf("Error connecting to etcd: %s\n", err)
	}

	fileServerURL := fmt.Sprintf("http://%s:%d/", address, port)
	fileServerId, err := uuid.NewV4()
	if err != nil {
		logger.Error("Could not create a UUID")
		os.Exit(1)
	}

	bbs := Bbs.New(etcdAdapter)
	maintainingPresence, maintainingPresenceErrors, err := bbs.MaintainFileServerPresence(heartbeatInterval, fileServerURL, fileServerId.String())
	if err != nil {
		logger.Errorf("Failed to maintain presence: %s", err.Error())
		os.Exit(1)
	}

	registerSignalHandler(maintainingPresence, logger)

	go func() {
		select {
		case <-maintainingPresenceErrors:
			logger.Error("file-server.maintaining-presence.failed")
			os.Exit(1)
		}
	}()

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

func registerSignalHandler(maintainingPresence *Bbs.Presence, logger *steno.Logger) {
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)

		select {
		case <-c:
			logger.Info("Removing the key")
			err := maintainingPresence.Remove()
			logger.Info("Removed the key")
			if err != nil {
				println("failed to stop maintaining")
			}
			os.Exit(0)
		}
	}()
}
