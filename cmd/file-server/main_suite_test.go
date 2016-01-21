package main_test

import (
	"fmt"
	"os"

	"github.com/cloudfoundry-incubator/consuladapter/consulrunner"
	"github.com/cloudfoundry-incubator/inigo/fake_cc"
	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/config"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"

	"testing"
)

var fileServerBinary string
var fakeCC *fake_cc.FakeCC
var fakeCCProcess ifrit.Process
var consulRunner *consulrunner.ClusterRunner

func TestFileServer(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "File Server Suite")
}

var _ = SynchronizedBeforeSuite(func() []byte {
	fileServerPath, err := gexec.Build("github.com/cloudfoundry-incubator/file-server/cmd/file-server")
	Expect(err).NotTo(HaveOccurred())
	return []byte(fileServerPath)
}, func(fileServerPath []byte) {
	fakeCCAddress := fmt.Sprintf("127.0.0.1:%d", 6767+GinkgoParallelNode())
	fakeCC = fake_cc.New(fakeCCAddress)

	fileServerBinary = string(fileServerPath)

	consulRunner = consulrunner.NewClusterRunner(
		9001+config.GinkgoConfig.ParallelNode*consulrunner.PortOffsetLength,
		1,
		"http",
	)

	consulRunner.Start()
	consulRunner.WaitUntilReady()
})

var _ = SynchronizedAfterSuite(func() {
	consulRunner.Stop()
}, func() {
	gexec.CleanupBuildArtifacts()
})

var _ = BeforeEach(func() {
	consulRunner.Reset()
	fakeCCProcess = ifrit.Envoke(fakeCC)
})

var _ = AfterEach(func() {
	fakeCCProcess.Signal(os.Kill)
	Eventually(fakeCCProcess.Wait()).Should(Receive(BeNil()))
})
