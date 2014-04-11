package download_build_artifacts_test

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"

	"github.com/cloudfoundry-incubator/file-server/config"
	"github.com/cloudfoundry-incubator/file-server/handlers"
	"github.com/cloudfoundry-incubator/runtime-schema/router"
	"github.com/cloudfoundry/gosteno"
	ts "github.com/cloudfoundry/gunk/test_server"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("DownloadBuildArtifacts", func() {
	var (
		fakeCloudController *ts.Server
		ccMethod            = "GET"
		ccPath              = "/staging/buildpack_cache/app-guid/download"
		ccUsername          = "bob"
		ccPassword          = "password"
		ccStatusCode        int
		ccResponseBody      string

		testFile = []byte("the file I'm uploading")

		request       *http.Request
		requestMethod = "GET"
		requestUrl    = "http://file-server.com/build_artifacts/app-guid"

		response *httptest.ResponseRecorder

		logger *gosteno.Logger
	)

	BeforeEach(func() {
		ccStatusCode = 0
		ccResponseBody = string(testFile)

		fakeCloudController = ts.New()

		fakeCloudController.Append(ts.CombineHandlers(
			ts.VerifyRequest(ccMethod, ccPath),
			ts.VerifyBasicAuth(ccUsername, ccPassword),
			ts.RespondPtr(&ccStatusCode, &ccResponseBody),
		))

		var err error
		request, err = http.NewRequest(requestMethod, requestUrl, nil)
		Ω(err).ShouldNot(HaveOccurred())
	})

	JustBeforeEach(func(done Done) {
		conf := config.New()
		conf.CCAddress = fakeCloudController.URL()
		conf.CCUsername = ccUsername
		conf.CCPassword = ccPassword

		gosteno.EnterTestMode()
		logger = gosteno.NewLogger("test")
		r, err := router.NewFileServerRoutes().Router(handlers.New(conf, logger))
		Ω(err).ShouldNot(HaveOccurred())

		response = httptest.NewRecorder()

		r.ServeHTTP(response, request)

		close(done)
	})

	AfterEach(func() {
		fakeCloudController.Close()
	})

	Context("downloads the file, when all is well", func() {

		It("makes the request to CC", func() {
			Ω(fakeCloudController.ReceivedRequestsCount()).Should(Equal(1))
		})

		It("responds with 200 OK", func() {
			Ω(response.Code).Should(Equal(http.StatusOK))
		})

		It("downloads the correct file", func() {
			body, err := ioutil.ReadAll(response.Body)
			Ω(err).ShouldNot(HaveOccurred())
			Ω(body).Should(Equal(testFile))
		})

		It("logs the request as success", func() {
			records := gosteno.GetMeTheGlobalTestSink().Records()
			var logs string
			for _, record := range records {
				logs += record.Message + "\n"
			}
			Ω(logs).ShouldNot(ContainSubstring("build_artifacts.download.failed"))
			Ω(logs).Should(ContainSubstring("build_artifacts.download.success"))
		})
	})

	Context("when CC returns a non-succesful status code", func() {
		BeforeEach(func() {
			ccStatusCode = 403
		})

		It("makes the request to CC", func() {
			Ω(fakeCloudController.ReceivedRequestsCount()).Should(Equal(1))
		})

		It("responds with the status code from the CC request", func() {
			Ω(response.Code).Should(Equal(ccStatusCode))
		})

		It("logs the request as failed", func() {
			records := gosteno.GetMeTheGlobalTestSink().Records()
			var logs string
			for _, record := range records {
				logs += record.Message + "\n"
			}
			Ω(logs).Should(ContainSubstring("build_artifacts.download.failed"))
			Ω(logs).ShouldNot(ContainSubstring("build_artifacts.download.success"))
		})
	})
})
