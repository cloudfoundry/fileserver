package main_test

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	Bbs "github.com/cloudfoundry-incubator/runtime-schema/bbs"
	"github.com/cloudfoundry-incubator/runtime-schema/router"
	"github.com/cloudfoundry/gunk/timeprovider"
	"github.com/cloudfoundry/gunk/urljoiner"
	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/config"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

var ccAddress = os.Getenv("CC_ADDRESS")
var ccUsername = os.Getenv("CC_USERNAME")
var ccPassword = os.Getenv("CC_PASSWORD")
var appGuid = os.Getenv("CC_APPGUID")

var _ = Describe("File server", func() {
	var (
		bbs             *Bbs.BBS
		port            int
		servedDirectory string
		session         *gexec.Session
		err             error
	)

	start := func(extras ...string) *gexec.Session {
		args := append(extras, "-staticDirectory", servedDirectory, "-port", strconv.Itoa(port), "-etcdCluster", etcdRunner.NodeURLS()[0], "-ccAddress", ccAddress, "-ccUsername", ccUsername, "-ccPassword", ccPassword, "-skipCertVerify")
		session, err = gexec.Start(exec.Command(fileServerBinary, args...), GinkgoWriter, GinkgoWriter)
		Ω(err).ShouldNot(HaveOccurred())
		time.Sleep(10 * time.Millisecond)
		return session
	}

	BeforeEach(func() {
		if ccAddress == "" {
			ccAddress = "http://example.com"
			ccUsername = "username"
			ccPassword = "password"
		}

		bbs = Bbs.NewBBS(etcdRunner.Adapter(), timeprovider.NewTimeProvider())
		servedDirectory, err = ioutil.TempDir("", "file_server-test")
		Ω(err).ShouldNot(HaveOccurred())
		port = 8182 + config.GinkgoConfig.ParallelNode
	})

	AfterEach(func() {
		session.Kill().Wait()
		os.RemoveAll(servedDirectory)
	})

	Context("when file server exits", func() {
		It("should remove its presence", func() {
			session = start("-address", "localhost", "-heartbeatInterval", "10s")

			_, err = bbs.GetAvailableFileServer()
			Ω(err).ShouldNot(HaveOccurred())

			session.Interrupt()
			Eventually(session).Should(gexec.Exit(0))

			_, err = bbs.GetAvailableFileServer()
			Ω(err).Should(HaveOccurred())
		})
	})

	Context("when it fails to maintain presence", func() {
		BeforeEach(func() {
			session = start("-address", "localhost", "-heartbeatInterval", "1s")
		})

		It("should retry", func() {
			_, err := bbs.GetAvailableFileServer()
			Ω(err).ShouldNot(HaveOccurred())

			etcdRunner.Stop()
			Eventually(func() error {
				_, err := bbs.GetAvailableFileServer()
				return err
			}).Should(HaveOccurred())

			Consistently(session, 1).ShouldNot(gexec.Exit())

			etcdRunner.Start()
			Eventually(func() error {
				_, err := bbs.GetAvailableFileServer()
				return err
			}, 3).ShouldNot(HaveOccurred())
		})
	})

	Context("when started without any arguments", func() {
		It("should fail", func() {
			session, err = gexec.Start(exec.Command(fileServerBinary), GinkgoWriter, GinkgoWriter)
			Ω(err).ShouldNot(HaveOccurred())
			Eventually(session).Should(gexec.Exit(2))
		})
	})

	Context("when started correctly", func() {
		BeforeEach(func() {
			session = start("-address", "localhost")
			ioutil.WriteFile(filepath.Join(servedDirectory, "test"), []byte("hello"), os.ModePerm)
		})

		It("should maintain presence in ETCD", func() {
			fileServerURL, err := bbs.GetAvailableFileServer()
			Ω(err).ShouldNot(HaveOccurred())

			Ω(fileServerURL).Should(Equal(fmt.Sprintf("http://localhost:%d/", port)))
		})

		It("should return that file on GET request", func() {
			resp, err := http.Get(fmt.Sprintf("http://localhost:%d/v1/static/test", port))
			Ω(err).ShouldNot(HaveOccurred())
			defer resp.Body.Close()

			Ω(resp.StatusCode).Should(Equal(http.StatusOK))

			body, err := ioutil.ReadAll(resp.Body)
			Ω(err).ShouldNot(HaveOccurred())
			Ω(string(body)).Should(Equal("hello"))
		})
	})

	Context("when an address is not specified", func() {
		It("publishes its url properly", func() {
			session = start()

			fileServerURL, err := bbs.GetAvailableFileServer()
			Ω(err).ShouldNot(HaveOccurred())

			serverURL, err := url.Parse(fileServerURL)
			Ω(err).ShouldNot(HaveOccurred())

			host, _, err := net.SplitHostPort(serverURL.Host)
			Ω(err).ShouldNot(HaveOccurred())

			Ω(host).ShouldNot(Equal(""))
		})
	})

	if appGuid != "" {
		Describe("uploading a file", func() {
			var tempFile string
			BeforeEach(func() {
				f, err := ioutil.TempFile("", "upload.tmp")
				Ω(err).ShouldNot(HaveOccurred())
				tempFile = f.Name()
				f.Close()
			})

			It("should upload the file...", func() {
				session = start("-address", "localhost")

				content := strings.Repeat("a big file", 10*1024)
				err := ioutil.WriteFile(tempFile, []byte(content), 0777)
				Ω(err).ShouldNot(HaveOccurred())

				route, ok := router.NewFileServerRoutes().RouteForHandler(router.FS_UPLOAD_DROPLET)
				Ω(ok).Should(BeTrue())

				path, err := route.PathWithParams(map[string]string{"guid": appGuid})
				Ω(err).ShouldNot(HaveOccurred())
				url := urljoiner.Join(fmt.Sprintf("http://localhost:%d", port), path)

				file, err := os.Open(tempFile)
				Ω(err).ShouldNot(HaveOccurred())

				fileStat, err := file.Stat()
				Ω(err).ShouldNot(HaveOccurred())

				postRequest, err := http.NewRequest("POST", url, file)
				Ω(err).ShouldNot(HaveOccurred())
				postRequest.ContentLength = fileStat.Size()
				postRequest.Header.Set("Content-Type", "application/octet-stream")

				resp, err := http.DefaultClient.Do(postRequest)
				Ω(err).ShouldNot(HaveOccurred())
				Ω(resp.StatusCode).Should(Equal(http.StatusCreated))
			})
		})
	}
})
