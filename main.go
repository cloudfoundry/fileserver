package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/cloudfoundry-incubator/file-server/handlers"
	"github.com/cloudfoundry-incubator/file-server/maintain"
	Bbs "github.com/cloudfoundry-incubator/runtime-schema/bbs"
	"github.com/cloudfoundry-incubator/runtime-schema/bbs/services_bbs"
	Router "github.com/cloudfoundry-incubator/runtime-schema/router"
	steno "github.com/cloudfoundry/gosteno"
	"github.com/cloudfoundry/gunk/localip"
	"github.com/cloudfoundry/gunk/timeprovider"
	"github.com/cloudfoundry/storeadapter/etcdstoreadapter"
	"github.com/cloudfoundry/storeadapter/workerpool"
	uuid "github.com/nu7hatch/gouuid"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/grouper"
	"github.com/tedsuo/ifrit/http_server"
	"github.com/tedsuo/ifrit/sigmon"
)

var presence *services_bbs.Presence

var serverAddress = flag.String(
	"address",
	"",
	"Specifies the address to bind to",
)

var logLevel = flag.String(
	"logLevel",
	"info",
	"Logging level (none, fatal, error, warn, info, debug, debug1, debug2, all)",
)

var etcdCluster = flag.String(
	"etcdCluster",
	"http://127.0.0.1:4001",
	"comma-separated list of etcd addresses (http://ip:port)",
)

var staticDirectory = flag.String(
	"staticDirectory",
	"",
	"Specifies the directory to serve local static files from",
)

var syslogName = flag.String(
	"syslogName",
	"",
	"Syslog name",
)

var serverPort = flag.Int(
	"port",
	8080,
	"Specifies the port of the file server",
)

var ccPassword = flag.String(
	"ccPassword",
	"",
	"CloudController basic auth password",
)

var ccUsername = flag.String(
	"ccUsername",
	"",
	"CloudController basic auth username",
)

var ccAddress = flag.String(
	"ccAddress",
	"",
	"CloudController location",
)

var heartbeatInterval = flag.Duration(
	"heartbeatInterval",
	60*time.Second,
	"the interval between heartbeats for maintaining presence",
)

var skipCertVerify = flag.Bool(
	"skipCertVerify",
	false,
	"Skip SSL certificate verification",
)

var ccJobPollingInterval = flag.Duration(
	"ccJobPollingInterval",
	100*time.Millisecond,
	"the interval between job polling requests",
)

func main() {
	flag.Parse()

	logger := initializeLogger()
	bbs := initializeFileServerBBS(logger)

	group := grouper.EnvokeGroup(grouper.RunGroup{
		"maintainer":  initializeMaintainer(logger, bbs),
		"file server": initializeServer(logger),
	})
	monitor := ifrit.Envoke(sigmon.New(group))

	logger.Info("file-server.ready")

	monitorEnded := monitor.Wait()
	workerEnded := group.Exits()

	for {
		select {
		case member := <-workerEnded:
			logger.Infof("%s exited", member.Name)
			monitor.Signal(syscall.SIGTERM)

		case err := <-monitorEnded:
			if err != nil {
				logger.Fatal(err.Error())
			}
			os.Exit(0)
		}
	}
}

func initializeLogger() *steno.Logger {
	l, err := steno.GetLogLevel(*logLevel)
	if err != nil {
		log.Fatalf("Invalid loglevel: %s\n", *logLevel)
	}

	stenoConfig := steno.Config{
		Level: l,
		Sinks: []steno.Sink{steno.NewIOSink(os.Stdout)},
	}

	if *syslogName != "" {
		stenoConfig.Sinks = append(stenoConfig.Sinks, steno.NewSyslogSink(*syslogName))
	}

	steno.Init(&stenoConfig)
	return steno.NewLogger("file_server")
}

func initializeMaintainer(logger *steno.Logger, bbs Bbs.FileServerBBS) *maintain.Maintainer {
	if *serverAddress == "" {
		var err error
		*serverAddress, err = localip.LocalIP()
		if err != nil {
			logger.Errorf("Error obtaining local ip address: %s\n", err.Error())
			os.Exit(1)
		}
	}

	url := fmt.Sprintf("http://%s:%d/", *serverAddress, *serverPort)
	logger.Infof("Serving files on %s", url)

	id, err := uuid.NewV4()
	if err != nil {
		logger.Error("Could not create a UUID")
		os.Exit(1)
	}

	return maintain.New(url, id.String(), bbs, logger, *heartbeatInterval)
}

func initializeServer(logger *steno.Logger) ifrit.Runner {
	if *staticDirectory == "" {
		logger.Fatal("staticDirectory is required")
	}
	if *ccAddress == "" {
		logger.Fatal("ccAddress is required")
	}
	if *ccUsername == "" {
		logger.Fatal("ccUsername is required")
	}
	if *ccPassword == "" {
		logger.Fatal("ccPassword is required")
	}

	actions := handlers.New(handlers.Config{
		CCJobPollingInterval: *ccJobPollingInterval,
		CCAddress:            *ccAddress,
		CCPassword:           *ccPassword,
		CCUsername:           *ccUsername,
		SkipCertVerify:       *skipCertVerify,
		StaticDirectory:      *staticDirectory,
	}, logger)

	router, err := Router.NewFileServerRoutes().Router(actions)

	if err != nil {
		logger.Errorf("Failed to build router: %s", err.Error())
		os.Exit(1)
	}

	address := fmt.Sprintf(":%d", *serverPort)
	return http_server.New(address, router)
}

func initializeFileServerBBS(logger *steno.Logger) Bbs.FileServerBBS {
	etcdAdapter := etcdstoreadapter.NewETCDStoreAdapter(
		strings.Split(*etcdCluster, ","),
		workerpool.NewWorkerPool(10),
	)

	err := etcdAdapter.Connect()
	if err != nil {
		logger.Errorf("Error connecting to etcd: %s\n", err.Error())
		os.Exit(1)
	}

	return Bbs.NewFileServerBBS(etcdAdapter, timeprovider.NewTimeProvider(), logger)
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
