package main_test

import (
	"fmt"
	Bbs "github.com/cloudfoundry-incubator/runtime-schema/bbs"
	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/config"
	. "github.com/onsi/gomega"
	"github.com/vito/cmdtest"
	. "github.com/vito/cmdtest/matchers"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

var _ = Describe("File-Server", func() {
	var (
		bbs             *Bbs.BBS
		port            int
		servedDirectory string
		session         *cmdtest.Session
		err             error
	)

	BeforeEach(func() {
		bbs = Bbs.New(etcdRunner.Adapter())
		servedDirectory, err = ioutil.TempDir("", "file-server-test")
		Ω(err).ShouldNot(HaveOccurred())
		port = 8182 + config.GinkgoConfig.ParallelNode
	})

	AfterEach(func() {
		session.Cmd.Process.Kill()
		os.RemoveAll(servedDirectory)
	})

	Context("when file server exits", func() {
		It("should remove its presence", func() {
			session, err = cmdtest.Start(exec.Command(fileServerBinary, "-address", "localhost", "-directory", servedDirectory, "-port", strconv.Itoa(port), "-etcdMachines", etcdRunner.NodeURLS()[0], "-heartbeatInterval", "10"))
			time.Sleep(100 * time.Millisecond)

			_, err = bbs.GetAvailableFileServer()
			Ω(err).ShouldNot(HaveOccurred())

			session.Cmd.Process.Signal(os.Interrupt)
			Ω(session).Should(ExitWith(0))

			_, err = bbs.GetAvailableFileServer()
			Ω(err).Should(HaveOccurred())
		})
	})

	Context("when it fails to maintain presence", func() {
		BeforeEach(func() {
			session, err = cmdtest.Start(exec.Command(fileServerBinary, "-address", "localhost", "-directory", servedDirectory, "-port", strconv.Itoa(port), "-etcdMachines", etcdRunner.NodeURLS()[0], "-heartbeatInterval", "1"))
			_, err := session.Wait(10 * time.Millisecond)
			Ω(err).Should(HaveOccurred(), "Error: fileserver did not start")
		})

		It("should return an error", func() {
			_, err := bbs.GetAvailableFileServer()
			Ω(err).ShouldNot(HaveOccurred())
			etcdRunner.Stop()
			time.Sleep(1500 * time.Millisecond)
			Ω(session).Should(ExitWith(1))
		})
	})

	Context("when started without any arguments", func() {
		It("should fail", func() {
			session, err = cmdtest.Start(exec.Command(fileServerBinary))
			Ω(err).ShouldNot(HaveOccurred())
			Ω(session).Should(ExitWith(1))
		})
	})

	Context("when started correctly", func() {
		BeforeEach(func() {
			session, err = cmdtest.Start(exec.Command(fileServerBinary, "-address", "localhost", "-directory", servedDirectory, "-port", strconv.Itoa(port), "-etcdMachines", etcdRunner.NodeURLS()[0]))
			_, err := session.Wait(10 * time.Millisecond)
			Ω(err).Should(HaveOccurred(), "Error: fileserver did not start")

			ioutil.WriteFile(filepath.Join(servedDirectory, "test"), []byte("hello"), os.ModePerm)
		})

		It("should maintain presence in ETCD", func() {
			fileServerURL, err := bbs.GetAvailableFileServer()
			Ω(err).ShouldNot(HaveOccurred())

			Ω(fileServerURL).Should(Equal(fmt.Sprintf("http://localhost:%d/", port)))
		})

		It("should return that file on GET request", func() {
			resp, err := http.Get(fmt.Sprintf("http://localhost:%d/test", port))
			Ω(err).ShouldNot(HaveOccurred())
			defer resp.Body.Close()

			Ω(resp.StatusCode).Should(Equal(http.StatusOK))

			body, err := ioutil.ReadAll(resp.Body)
			Ω(err).ShouldNot(HaveOccurred())
			Ω(string(body)).Should(Equal("hello"))
		})
	})
})
