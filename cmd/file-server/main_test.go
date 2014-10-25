package main_test

import (
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	Bbs "github.com/cloudfoundry-incubator/runtime-schema/bbs"
	"github.com/cloudfoundry-incubator/runtime-schema/models"
	"github.com/cloudfoundry-incubator/runtime-schema/router"
	"github.com/cloudfoundry/gunk/timeprovider"
	"github.com/cloudfoundry/gunk/urljoiner"
	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/config"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/pivotal-golang/lager/lagertest"
)

type ByteEmitter struct {
	written int
	length  int
}

func NewEmitter(length int) *ByteEmitter {
	return &ByteEmitter{
		length:  length,
		written: 0,
	}
}

func (emitter *ByteEmitter) Read(p []byte) (n int, err error) {
	if emitter.written >= emitter.length {
		return 0, io.EOF
	}
	time.Sleep(time.Millisecond)
	p[0] = 0xF1
	emitter.written++
	return 1, nil
}

var _ = Describe("File server", func() {
	var (
		bbs             *Bbs.BBS
		port            int
		address         string
		servedDirectory string
		session         *gexec.Session
		err             error
		appGuid         = "app-guid"
	)

	start := func(extras ...string) *gexec.Session {
		args := append(
			extras,
			"-staticDirectory", servedDirectory,
			"-port", strconv.Itoa(port),
			"-etcdCluster", etcdRunner.NodeURLS()[0],
			"-ccAddress", fakeCC.Address(),
			"-ccUsername", fakeCC.Username(),
			"-ccPassword", fakeCC.Password(),
			"-skipCertVerify",
		)

		session, err = gexec.Start(exec.Command(fileServerBinary, args...), GinkgoWriter, GinkgoWriter)
		Ω(err).ShouldNot(HaveOccurred())

		Eventually(session).Should(gbytes.Say("file-server.ready"))

		return session
	}

	dropletUploadRequest := func(appGuid string, body io.Reader, contentLength int) *http.Request {
		ccUrl, err := url.Parse(fakeCC.Address())
		Ω(err).ShouldNot(HaveOccurred())
		ccUrl.User = url.UserPassword(fakeCC.Username(), fakeCC.Password())
		ccUrl.Path = urljoiner.Join("staging", "droplets", appGuid, "upload")
		v := url.Values{"async": []string{"true"}}
		ccUrl.RawQuery = v.Encode()

		route, ok := router.NewFileServerRoutes().RouteForHandler(router.FS_UPLOAD_DROPLET)
		Ω(ok).Should(BeTrue())

		path, err := route.PathWithParams(map[string]string{"guid": appGuid})
		Ω(err).ShouldNot(HaveOccurred())

		u, err := url.Parse(urljoiner.Join(address, path))
		Ω(err).ShouldNot(HaveOccurred())
		v = url.Values{models.CcDropletUploadUriKey: []string{ccUrl.String()}}
		u.RawQuery = v.Encode()

		postRequest, err := http.NewRequest("POST", u.String(), body)
		Ω(err).ShouldNot(HaveOccurred())
		postRequest.ContentLength = int64(contentLength)
		postRequest.Header.Set("Content-Type", "application/octet-stream")

		return postRequest
	}

	BeforeEach(func() {
		bbs = Bbs.NewBBS(etcdRunner.Adapter(), timeprovider.NewTimeProvider(), lagertest.NewTestLogger("test"))
		servedDirectory, err = ioutil.TempDir("", "file_server-test")
		Ω(err).ShouldNot(HaveOccurred())
		port = 8182 + config.GinkgoConfig.ParallelNode
		address = fmt.Sprintf("http://localhost:%d", port)
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

	Describe("uploading a file", func() {
		var contentLength = 100

		BeforeEach(func() {
			session = start("-address", "localhost")
		})

		It("should upload the file...", func() {
			emitter := NewEmitter(contentLength)
			postRequest := dropletUploadRequest(appGuid, emitter, contentLength)
			resp, err := http.DefaultClient.Do(postRequest)
			Ω(err).ShouldNot(HaveOccurred())
			defer resp.Body.Close()

			Ω(resp.StatusCode).Should(Equal(http.StatusCreated))
			Ω(len(fakeCC.UploadedDroplets[appGuid])).Should(Equal(contentLength))
		})
	})
})
