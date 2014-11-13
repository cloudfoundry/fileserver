package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"time"

	"github.com/cloudfoundry-incubator/cf-debug-server"
	"github.com/cloudfoundry-incubator/cf-lager"
	"github.com/cloudfoundry-incubator/file-server/handlers"
	"github.com/cloudfoundry-incubator/file-server/uploader"
	"github.com/cloudfoundry-incubator/runtime-schema/router"
	"github.com/cloudfoundry/dropsonde"
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

var dropsondeOrigin = flag.String(
	"dropsondeOrigin",
	"file_server",
	"Origin identifier for dropsonde-emitted metrics.",
)

var dropsondeDestination = flag.String(
	"dropsondeDestination",
	"localhost:3457",
	"Destination for dropsonde-emitted metrics.",
)

var ccUploadDialTimeout = flag.Duration(
	"ccUploadDialTimeout",
	10*time.Second,
	"CloudController upload dial timeout",
)

var ccUploadKeepAlive = flag.Duration(
	"ccUploadKeepAlive",
	30*time.Second,
	"CloudController keep-alive duration",
)

var ccUploadTLSHandshakeTimeout = flag.Duration(
	"ccUploadTLSHandshakeTimeout",
	10*time.Second,
	"CloudController TLS handshake timeout",
)

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	flag.Parse()

	logger := cf_lager.New("file-server")
	cf_debug_server.Run()

	initializeDropsonde(logger)

	group := grouper.NewOrdered(os.Interrupt, grouper.Members{
		{"file server", initializeServer(logger)},
	})

	monitor := ifrit.Invoke(sigmon.New(group))
	logger.Info("ready")

	err := <-monitor.Wait()
	if err != nil {
		logger.Fatal("exited-with-failure", err)
	}
}

func initializeDropsonde(logger lager.Logger) {
	err := dropsonde.Initialize(*dropsondeDestination, *dropsondeOrigin)
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
			Timeout:   *ccUploadDialTimeout,
			KeepAlive: *ccUploadKeepAlive,
		}).Dial,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: *skipCertVerify,
		},
		TLSHandshakeTimeout: *ccUploadTLSHandshakeTimeout,
	}

	uploader := uploader.New(ccUrl, transport)

	actions := handlers.New(*staticDirectory, *ccJobPollingInterval, uploader, logger)

	fileServerRouter, err := router.NewFileServerRoutes().Router(actions)

	if err != nil {
		logger.Error("router-building-failed", err)
		os.Exit(1)
	}

	address := fmt.Sprintf(":%d", *serverPort)
	return http_server.New(address, fileServerRouter)
}
