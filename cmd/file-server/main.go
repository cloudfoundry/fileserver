package main

import (
	"crypto/tls"
	"flag"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"time"

	"github.com/cloudfoundry-incubator/cf-debug-server"
	"github.com/cloudfoundry-incubator/cf-lager"
	"github.com/cloudfoundry-incubator/file-server/ccclient"
	"github.com/cloudfoundry-incubator/file-server/handlers"
	"github.com/cloudfoundry/dropsonde"
	"github.com/pivotal-golang/lager"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/grouper"
	"github.com/tedsuo/ifrit/http_server"
	"github.com/tedsuo/ifrit/sigmon"
)

var serverAddress = flag.String(
	"address",
	"0.0.0.0:8080",
	"Specifies the address to bind to",
)

var staticDirectory = flag.String(
	"staticDirectory",
	"",
	"Specifies the directory to serve local static files from",
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

var skipCertVerify = flag.Bool(
	"skipCertVerify",
	false,
	"Skip SSL certificate verification",
)

var ccJobPollingInterval = flag.Duration(
	"ccJobPollingInterval",
	1*time.Second,
	"the interval between job polling requests",
)

const (
	ccUploadDialTimeout         = 10 * time.Second
	ccUploadKeepAlive           = 30 * time.Second
	ccUploadTLSHandshakeTimeout = 10 * time.Second
	dropsondeDestination        = "localhost:3457"
	dropsondeOrigin             = "file_server"
)

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	cf_debug_server.AddFlags(flag.CommandLine)
	cf_lager.AddFlags(flag.CommandLine)
	flag.Parse()

	logger := cf_lager.New("file-server")

	initializeDropsonde(logger)

	members := grouper.Members{
		{"file server", initializeServer(logger)},
	}

	if dbgAddr := cf_debug_server.DebugAddress(flag.CommandLine); dbgAddr != "" {
		members = append(grouper.Members{
			{"debug-server", cf_debug_server.Runner(dbgAddr)},
		}, members...)
	}

	group := grouper.NewOrdered(os.Interrupt, members)

	monitor := ifrit.Invoke(sigmon.New(group))
	logger.Info("ready")

	err := <-monitor.Wait()
	if err != nil {
		logger.Error("exited-with-failure", err)
		os.Exit(1)
	}

	logger.Info("exited")
}

func initializeDropsonde(logger lager.Logger) {
	err := dropsonde.Initialize(dropsondeDestination, dropsondeOrigin)
	if err != nil {
		logger.Error("failed to initialize dropsonde: %v", err)
	}
}

func initializeServer(logger lager.Logger) ifrit.Runner {
	if *staticDirectory == "" {
		logger.Fatal("static-directory-missing", nil)
	}
	if *ccAddress == "" {
		logger.Fatal("cc-address-missing", nil)
	}
	ccUrl, err := url.Parse(*ccAddress)
	if err != nil {
		logger.Fatal("cc-address-parse-failure", err)
	}
	if *ccUsername == "" {
		logger.Fatal("cc-username-missing", nil)
	}
	if *ccPassword == "" {
		logger.Fatal("cc-password-missing", nil)
	}

	ccUrl.User = url.UserPassword(*ccUsername, *ccPassword)

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		Dial: (&net.Dialer{
			Timeout:   ccUploadDialTimeout,
			KeepAlive: ccUploadKeepAlive,
		}).Dial,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: *skipCertVerify,
		},
		TLSHandshakeTimeout: ccUploadTLSHandshakeTimeout,
	}

	uploader := ccclient.NewUploader(logger, ccUrl, transport)
	poller := ccclient.NewPoller(logger, transport, *ccJobPollingInterval)

	fileServerHandler, err := handlers.New(*staticDirectory, uploader, poller, logger)
	if err != nil {
		logger.Error("router-building-failed", err)
		os.Exit(1)
	}

	return http_server.New(*serverAddress, fileServerHandler)
}
