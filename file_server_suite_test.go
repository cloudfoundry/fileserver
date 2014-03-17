package main_test

import (
	"github.com/cloudfoundry/storeadapter/storerunner/etcdstorerunner"
	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/config"
	. "github.com/onsi/gomega"
	"github.com/vito/cmdtest"
	"os"
	"os/signal"

	"testing"
)

var fileServerBinary string
var etcdRunner *etcdstorerunner.ETCDClusterRunner

func TestFileServer(t *testing.T) {
	registerSignalHandler()
	RegisterFailHandler(Fail)

	var err error
	fileServerBinary, err = cmdtest.Build("github.com/cloudfoundry-incubator/file-server")
	if err != nil {
		panic(err.Error())
	}

	etcdRunner = etcdstorerunner.NewETCDClusterRunner(5001+config.GinkgoConfig.ParallelNode, 1)

	RunSpecs(t, "File Server Suite")
}

var _ = BeforeEach(func() {
	etcdRunner.Start()
})

var _ = AfterEach(func() {
	etcdRunner.Stop()
})

func registerSignalHandler() {
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, os.Kill)

		select {
		case <-c:
			etcdRunner.Stop()
			os.Exit(0)
		}
	}()
}
