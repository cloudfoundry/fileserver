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
	"time"

	"github.com/cloudfoundry-incubator/file-server/handlers/static"
	Bbs "github.com/cloudfoundry-incubator/runtime-schema/bbs"
	steno "github.com/cloudfoundry/gosteno"
	"github.com/cloudfoundry/gunk/localip"
	"github.com/cloudfoundry/storeadapter/etcdstoreadapter"
	"github.com/cloudfoundry/storeadapter/workerpool"
	uuid "github.com/nu7hatch/gouuid"
)

var address string
var port int
var directory string
var logLevel string
var etcdMachines string
var heartbeatInterval time.Duration

var presence *Bbs.Presence

func init() {
	flag.StringVar(&address, "address", "", "Specifies the address to bind to")
	flag.IntVar(&port, "port", 8080, "Specifies the port of the file server")
	flag.StringVar(&directory, "directory", "", "Specifies the directory to serve")
	flag.StringVar(&logLevel, "logLevel", "info", "Logging level (none, fatal, error, warn, info, debug, debug1, debug2, all)")
	flag.StringVar(&etcdMachines, "etcdMachines", "http://127.0.0.1:4001", "comma-separated list of etcd addresses (http://ip:port)")
	flag.DurationVar(&heartbeatInterval, "heartbeatInterval", 60*time.Second, "the interval between heartbeats for maintaining presence")
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
		logger.Errorf("Error connecting to etcd: %s\n", err.Error())
		os.Exit(1)
	}

	if address == "" {
		address, err = localip.LocalIP()
		if err != nil {
			logger.Errorf("Error obtaining local ip address: %s\n", err.Error())
			os.Exit(1)
		}
	}

	fileServerURL := fmt.Sprintf("http://%s:%d/", address, port)
	fileServerId, err := uuid.NewV4()
	if err != nil {
		logger.Error("Could not create a UUID")
		os.Exit(1)
	}

	bbs := Bbs.New(etcdAdapter)
	maintainingPresence, lostPresence, err := bbs.MaintainFileServerPresence(heartbeatInterval, fileServerURL, fileServerId.String())
	if err != nil {
		logger.Errorf("Failed to maintain presence: %s", err.Error())
		os.Exit(1)
	}

	registerSignalHandler(maintainingPresence, logger)

	go func() {
		select {
		case <-lostPresence:
			logger.Error("file-server.maintaining-presence.failed")
			os.Exit(1)
		}
	}()

	staticFileServer := static.New(directory)
	logger.Infof("Serving files on %s", fileServerURL)
	logger.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), staticFileServer).Error())
}

func registerSignalHandler(maintainingPresence Bbs.PresenceInterface, logger *steno.Logger) {
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)

		select {
		case <-c:
			maintainingPresence.Remove()
			os.Exit(0)
		}
	}()
}
