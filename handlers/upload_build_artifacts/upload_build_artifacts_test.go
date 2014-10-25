package upload_build_artifacts_test

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"

	"github.com/cloudfoundry-incubator/file-server/handlers"
	"github.com/cloudfoundry-incubator/runtime-schema/models"
	"github.com/cloudfoundry-incubator/runtime-schema/router"
	ts "github.com/cloudfoundry/gunk/test_server"
	"github.com/pivotal-golang/lager"

	. "github.com/cloudfoundry-incubator/file-server/handlers/test_helpers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("UploadBuildArtifacts", func() {
	var (
		fakeCloudController *ts.Server
		primaryUrl          *url.URL
		config              handlers.Config

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

		fakeCloudController = ts.New()

		config = handlers.Config{
			CCAddress:  fakeCloudController.URL(),
			CCUsername: "bob",
			CCPassword: "password",
		}

		fakeCloudController.Append(ts.CombineHandlers(
			ts.VerifyRequest("POST", "/staging/buildpack_cache/app-guid/upload"),
			ts.VerifyBasicAuth("bob", "password"),
			ts.RespondPtr(&postStatusCode, &postResponseBody),
			func(w http.ResponseWriter, r *http.Request) {
				uploadedHeaders = r.Header
				file, fileHeader, err := r.FormFile("upload[droplet]")
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
	})

	JustBeforeEach(func() {
		logger := lager.NewLogger("fakelogger")
		r, err := router.NewFileServerRoutes().Router(handlers.New(config, logger))
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
				config.CCAddress = "127.0.0.1:0"
			})

			It("makes the request to CC", func() {
				Ω(fakeCloudController.ReceivedRequestsCount()).Should(Equal(1))

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
				Ω(fakeCloudController.ReceivedRequestsCount()).Should(Equal(1))

				By("responds with 200 CREATED", func() {
					Ω(outgoingResponse.Code).Should(Equal(http.StatusOK))
				})
			})
		})
	})

	ItFailsWhenTheContentLengthIsMissing(&incomingRequest, &outgoingResponse, &fakeCloudController)
	ItHandlesCCFailures(&postStatusCode, &outgoingResponse, &fakeCloudController)

	Context("when both urls fail", func() {
		BeforeEach(func() {
			primaryUrl.Host = "127.0.0.1:0"
			config.CCAddress = "127.0.0.1:0"
		})

		It("reports a 500", func() {
			Ω(fakeCloudController.ReceivedRequestsCount()).Should(Equal(0))

			By("responds with 201", func() {
				Ω(outgoingResponse.Code).Should(Equal(http.StatusInternalServerError))
			})
		})
	})
})
