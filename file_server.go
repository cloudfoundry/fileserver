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

	conf "github.com/cloudfoundry-incubator/file-server/config"
	"github.com/cloudfoundry-incubator/file-server/handlers"
	Bbs "github.com/cloudfoundry-incubator/runtime-schema/bbs"
	"github.com/cloudfoundry-incubator/runtime-schema/bbs/services_bbs"
	"github.com/cloudfoundry-incubator/runtime-schema/router"
	steno "github.com/cloudfoundry/gosteno"
	"github.com/cloudfoundry/gunk/localip"
	"github.com/cloudfoundry/gunk/timeprovider"
	"github.com/cloudfoundry/storeadapter/etcdstoreadapter"
	"github.com/cloudfoundry/storeadapter/workerpool"
	uuid "github.com/nu7hatch/gouuid"
)

var (
	presence *services_bbs.Presence
	config   *conf.Config
)

func init() {
	config = conf.New()
	flag.StringVar(&config.Address, "address", "", "Specifies the address to bind to")
	flag.IntVar(&config.Port, "port", 8080, "Specifies the port of the file server")
	flag.StringVar(&config.SyslogName, "syslogName", "", "Syslog name")
	flag.StringVar(&config.StaticDirectory, "staticDirectory", "", "Specifies the directory to serve local static files from")
	flag.StringVar(&config.LogLevel, "logLevel", "info", "Logging level (none, fatal, error, warn, info, debug, debug1, debug2, all)")
	flag.StringVar(&config.EtcdCluster, "etcdCluster", "http://127.0.0.1:4001", "comma-separated list of etcd addresses (http://ip:port)")
	flag.DurationVar(&config.HeartbeatInterval, "heartbeatInterval", 60*time.Second, "the interval between heartbeats for maintaining presence")
	flag.StringVar(&config.CCAddress, "ccAddress", "", "CloudController location")
	flag.StringVar(&config.CCUsername, "ccUsername", "", "CloudController basic auth username")
	flag.StringVar(&config.CCPassword, "ccPassword", "", "CloudController basic auth password")
	flag.DurationVar(&config.CCJobPollingInterval, "ccJobPollingInterval", 100*time.Millisecond, "the interval between job polling requests")
	flag.BoolVar(&config.SkipCertVerify, "skipCertVerify", false, "Skip SSL certificate verification")
}

func main() {
	flag.Parse()

	l, err := steno.GetLogLevel(config.LogLevel)
	if err != nil {
		log.Fatalf("Invalid loglevel: %s\n", config.LogLevel)
	}

	stenoConfig := steno.Config{
		Level: l,
		Sinks: []steno.Sink{steno.NewIOSink(os.Stdout)},
	}

	if config.SyslogName != "" {
		stenoConfig.Sinks = append(stenoConfig.Sinks, steno.NewSyslogSink(config.SyslogName))
	}

	steno.Init(&stenoConfig)
	logger := steno.NewLogger("file_server")

	errs := config.Validate()
	if len(errs) > 0 {
		for _, err := range errs {
			logger.Error(err.Error())
		}
		os.Exit(1)
	}

	etcdAdapter := etcdstoreadapter.NewETCDStoreAdapter(
		strings.Split(config.EtcdCluster, ","),
		workerpool.NewWorkerPool(10),
	)

	err = etcdAdapter.Connect()
	if err != nil {
		logger.Errorf("Error connecting to etcd: %s\n", err.Error())
		os.Exit(1)
	}

	if config.Address == "" {
		config.Address, err = localip.LocalIP()
		if err != nil {
			logger.Errorf("Error obtaining local ip address: %s\n", err.Error())
			os.Exit(1)
		}
	}

	fileServerURL := fmt.Sprintf("http://%s:%d/", config.Address, config.Port)
	fileServerId, err := uuid.NewV4()
	if err != nil {
		logger.Error("Could not create a UUID")
		os.Exit(1)
	}

	actions := handlers.New(config, logger)
	r, err := router.NewFileServerRoutes().Router(actions)

	if err != nil {
		logger.Errorf("Failed to build router: %s", err.Error())
		os.Exit(1)
	}

	bbs := Bbs.NewFileServerBBS(etcdAdapter, timeprovider.NewTimeProvider())

	presence, statusChannel, err := bbs.MaintainFileServerPresence(config.HeartbeatInterval, fileServerURL, fileServerId.String())
	if err != nil {
		logger.Errorf("Failed to create file server presence: %s", err.Error())
		os.Exit(1)
	}

	stopFileServer := make(chan bool)

	registerSignalHandler(stopFileServer, logger)

	go func() {
		for {
			select {
			case status, ok := <-statusChannel:
				if !ok {
					return
				}

				if !status {
					logger.Error("file-server.maintaining-presence.failed")
				}

			case <-stopFileServer:
				presence.Remove()
				os.Exit(0)
			}
		}
	}()

	logger.Infof("Serving files on %s", fileServerURL)
	logger.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", config.Port), r).Error())
}

func registerSignalHandler(stopChannel chan<- bool, logger *steno.Logger) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		select {
		case <-c:
			signal.Stop(c)
			stopChannel <- true
		}
	}()
}
