package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/cloudfoundry-incubator/cf-debug-server"
	"github.com/cloudfoundry-incubator/cf-lager"
	"github.com/cloudfoundry-incubator/file-server/handlers"
	Bbs "github.com/cloudfoundry-incubator/runtime-schema/bbs"
	Router "github.com/cloudfoundry-incubator/runtime-schema/router"
	_ "github.com/cloudfoundry/dropsonde/autowire"
	"github.com/cloudfoundry/gunk/localip"
	"github.com/cloudfoundry/gunk/timeprovider"
	"github.com/cloudfoundry/storeadapter/etcdstoreadapter"
	"github.com/cloudfoundry/storeadapter/workerpool"
	uuid "github.com/nu7hatch/gouuid"
	"github.com/pivotal-golang/lager"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/grouper"
	"github.com/tedsuo/ifrit/http_server"
	"github.com/tedsuo/ifrit/sigmon"
)

var serverAddress = flag.String(
	"address",
	"",
	"Specifies the address to bind to",
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

	logger := cf_lager.New("file-server")
	cf_debug_server.Run()

	group := grouper.NewOrdered(os.Interrupt, grouper.Members{
		{"file server", initializeServer(logger)},
		{"maintainer", initializeHeartbeater(logger)},
	})

	monitor := ifrit.Envoke(sigmon.New(group))
	logger.Info("ready")

	err := <-monitor.Wait()
	if err != nil {
		logger.Fatal("exited-with-failure", err)
	}
}

func initializeHeartbeater(logger lager.Logger) ifrit.Runner {
	etcdAdapter := etcdstoreadapter.NewETCDStoreAdapter(
		strings.Split(*etcdCluster, ","),
		workerpool.NewWorkerPool(10),
	)

	err := etcdAdapter.Connect()
	if err != nil {
		logger.Fatal("failed-to-connect-to-etcd", err)
	}

	bbs := Bbs.NewFileServerBBS(etcdAdapter, timeprovider.NewTimeProvider(), logger)

	if *serverAddress == "" {
		var err error
		*serverAddress, err = localip.LocalIP()
		if err != nil {
			logger.Fatal("obtaining-local-ip-failed", err)
		}
	}

	url := fmt.Sprintf("http://%s:%d/", *serverAddress, *serverPort)
	logger.Info("serving-files-location", lager.Data{"url": url})

	id, err := uuid.NewV4()
	if err != nil {
		logger.Fatal("create-uuid-failed", err)
	}

	return bbs.NewFileServerHeartbeat(url, id.String(), *heartbeatInterval)
}

func initializeServer(logger lager.Logger) ifrit.Runner {
	if *staticDirectory == "" {
		logger.Fatal("static-directory-missing", nil)
	}
	if *ccAddress == "" {
		logger.Fatal("cc-address-missing", nil)
	}
	if *ccUsername == "" {
		logger.Fatal("cc-username-missing", nil)
	}
	if *ccPassword == "" {
		logger.Fatal("cc-password-missing", nil)
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
		logger.Error("router-building-failed", err)
		os.Exit(1)
	}

	address := fmt.Sprintf(":%d", *serverPort)
	return http_server.New(address, router)
}
