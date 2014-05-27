package main_test

import (
	"github.com/cloudfoundry-incubator/inigo/fake_cc"
	"github.com/cloudfoundry/storeadapter/storerunner/etcdstorerunner"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"

	"testing"
)

var fileServerBinary string
var etcdRunner *etcdstorerunner.ETCDClusterRunner
var fakeCC *fake_cc.FakeCC

func TestFileServer(t *testing.T) {
	RegisterFailHandler(Fail)

	BeforeSuite(func() {
		var err error
		fileServerBinary, err = gexec.Build("github.com/cloudfoundry-incubator/file-server")
		Ω(err).ShouldNot(HaveOccurred())

		etcdRunner = etcdstorerunner.NewETCDClusterRunner(5001+GinkgoParallelNode(), 1)
	})

	AfterSuite(func() {
		etcdRunner.Stop()
	})

	RunSpecs(t, "File Server Suite")
}

var _ = BeforeEach(func() {
	etcdRunner.Start()
	fakeCC = fake_cc.New()
	fakeCC.Start()
})

var _ = AfterEach(func() {
	etcdRunner.Stop()
	fakeCC.Stop()
})
