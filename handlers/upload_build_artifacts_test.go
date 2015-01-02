package handlers_test

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"

	"github.com/cloudfoundry-incubator/file-server/ccclient"
	"github.com/cloudfoundry-incubator/file-server/handlers"
	"github.com/cloudfoundry-incubator/file-server/handlers/test_helpers"
	"github.com/cloudfoundry-incubator/runtime-schema/models"
	"github.com/pivotal-golang/lager"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
)

var _ = Describe("UploadBuildArtifacts", func() {
	var (
		ccAddress           string
		fakeCloudController *ghttp.Server
		primaryUrl          *url.URL

		postStatusCode   int
		postResponseBody string
		uploadedBytes    []byte
		uploadedFileName string
		uploadedHeaders  http.Header

		incomingRequest  *http.Request
		outgoingResponse *httptest.ResponseRecorder
	)

	BeforeEach(func() {
		postStatusCode = 200

		uploadedHeaders = nil
		uploadedBytes = nil
		uploadedFileName = ""

		fakeCloudController = ghttp.NewServer()

		fakeCloudController.AppendHandlers(ghttp.CombineHandlers(
			ghttp.VerifyRequest("POST", "/staging/buildpack_cache/app-guid/upload"),
			ghttp.VerifyBasicAuth("bob", "password"),
			ghttp.RespondWithPtr(&postStatusCode, &postResponseBody),
			func(w http.ResponseWriter, r *http.Request) {
				uploadedHeaders = r.Header
				file, fileHeader, err := r.FormFile(ccclient.FormField)
				Ω(err).ShouldNot(HaveOccurred())
				uploadedBytes, err = ioutil.ReadAll(file)
				Ω(err).ShouldNot(HaveOccurred())
				uploadedFileName = fileHeader.Filename
				Ω(r.ContentLength).Should(BeNumerically(">", len(uploadedBytes)))
			},
		))

		var err error
		primaryUrl, err = url.Parse(fakeCloudController.URL())
		Ω(err).ShouldNot(HaveOccurred())
		primaryUrl.User = url.UserPassword("bob", "password")
		primaryUrl.Path = "/staging/buildpack_cache/app-guid/upload"

		buffer := bytes.NewBufferString("the file I'm uploading")
		incomingRequest, err = http.NewRequest("POST", "", buffer)
		incomingRequest.Header.Set("Content-MD5", "the-md5")

		ccAddress = fakeCloudController.URL()
	})

	JustBeforeEach(func() {
		logger := lager.NewLogger("fakelogger")

		ccUrl, err := url.Parse(ccAddress)
		Ω(err).ShouldNot(HaveOccurred())
		ccUrl.User = url.UserPassword("bob", "password")
		uploader := ccclient.NewUploader(ccUrl, http.DefaultTransport)
		poller := ccclient.NewPoller(http.DefaultTransport, 0)

		r, err := handlers.New("", uploader, poller, logger)
		Ω(err).ShouldNot(HaveOccurred())

		u, err := url.Parse("http://file-server.com/v1/build_artifacts/app-guid")
		Ω(err).ShouldNot(HaveOccurred())
		v := url.Values{models.CcBuildArtifactsUploadUriKey: []string{primaryUrl.String()}}
		u.RawQuery = v.Encode()
		incomingRequest.URL = u

		outgoingResponse = httptest.NewRecorder()

		r.ServeHTTP(outgoingResponse, incomingRequest)
	})

	AfterEach(func() {
		fakeCloudController.Close()
	})

	Context("uploading the file, when all is well", func() {
		Context("when the primary url works", func() {
			BeforeEach(func() {
				ccAddress = "127.0.0.1:0"
			})

			It("makes the request to CC", func() {
				Ω(fakeCloudController.ReceivedRequests()).Should(HaveLen(1))

				By("responds with 200 OK", func() {
					Ω(outgoingResponse.Code).Should(Equal(http.StatusOK))
				})

				By("uploads the correct file", func() {
					Ω(uploadedBytes).Should(Equal([]byte("the file I'm uploading")))
					Ω(uploadedFileName).Should(Equal("buildpack_cache.tgz"))
				})

				By("forwards the content-md5 header", func() {
					Ω(uploadedHeaders.Get("Content-MD5")).Should(Equal("the-md5"))
				})
			})
		})

		Context("when the primary url fails", func() {
			BeforeEach(func() {
				primaryUrl.Host = "127.0.0.1:0"
			})

			It("falls over to the secondary url", func() {
				Ω(fakeCloudController.ReceivedRequests()).Should(HaveLen(1))

				By("responds with 200 CREATED", func() {
					Ω(outgoingResponse.Code).Should(Equal(http.StatusOK))
				})
			})
		})
	})

	test_helpers.ItFailsWhenTheContentLengthIsMissing(&incomingRequest, &outgoingResponse, &fakeCloudController)
	test_helpers.ItHandlesCCFailures(&postStatusCode, &outgoingResponse, &fakeCloudController)

	Context("when both urls fail", func() {
		BeforeEach(func() {
			primaryUrl.Host = "127.0.0.1:0"
			ccAddress = "127.0.0.1:0"
		})

		It("reports a 500", func() {
			Ω(fakeCloudController.ReceivedRequests()).Should(HaveLen(0))

			By("responds with 201", func() {
				Ω(outgoingResponse.Code).Should(Equal(http.StatusInternalServerError))
			})
		})
	})
})
