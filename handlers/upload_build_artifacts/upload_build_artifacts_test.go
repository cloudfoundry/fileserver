package upload_build_artifacts_test

import (
	"bytes"
	"github.com/cloudfoundry-incubator/file-server/config"
	"github.com/cloudfoundry-incubator/file-server/handlers"
	"github.com/cloudfoundry-incubator/runtime-schema/router"
	"github.com/cloudfoundry/gosteno"
	ts "github.com/cloudfoundry/gunk/test_server"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("UploadBuildArtifacts", func() {
	var (
		fakeCloudController *ts.Server
		postStatusCode      int
		postResponseBody    string
		uploadedBytes       []byte
		uploadedFileName    string

		incomingRequest  *http.Request
		outgoingResponse *httptest.ResponseRecorder

		ccUploadMethod = "POST"
		ccUploadPath   = "/staging/buildpack_cache/app-guid/upload"
		ccUsername     = "bob"
		ccPassword     = "password"

		uploadBody   = []byte("the file I'm uploading")
		uploadMethod = "PUT"
		uploadUrl    = "http://file-server.com/build_artifacts/app-guid"
	)

	BeforeEach(func() {
		postStatusCode = 0
		postResponseBody = ""

		uploadedBytes = nil
		uploadedFileName = ""

		fakeCloudController = ts.New()

		fakeCloudController.Append(ts.CombineHandlers(
			ts.VerifyRequest(ccUploadMethod, ccUploadPath),
			ts.VerifyBasicAuth(ccUsername, ccPassword),
			ts.RespondPtr(&postStatusCode, &postResponseBody),
			func(w http.ResponseWriter, r *http.Request) {
				file, fileHeader, err := r.FormFile("upload[droplet]")
				Ω(err).ShouldNot(HaveOccurred())
				uploadedBytes, err = ioutil.ReadAll(file)
				Ω(err).ShouldNot(HaveOccurred())
				uploadedFileName = fileHeader.Filename
				Ω(r.ContentLength).Should(BeNumerically(">", len(uploadedBytes)))
			},
		))

		var err error
		buffer := bytes.NewBuffer(uploadBody)
		incomingRequest, err = http.NewRequest(uploadMethod, uploadUrl, buffer)
		Ω(err).ShouldNot(HaveOccurred())
	})

	JustBeforeEach(func(done Done) {
		conf := config.New()
		conf.CCAddress = fakeCloudController.URL()
		conf.CCUsername = ccUsername
		conf.CCPassword = ccPassword
		conf.CCJobPollingInterval = 10 * time.Millisecond

		logger := gosteno.NewLogger("")
		r, err := router.NewFileServerRoutes().Router(handlers.New(conf, logger))
		Ω(err).ShouldNot(HaveOccurred())

		outgoingResponse = httptest.NewRecorder()

		r.ServeHTTP(outgoingResponse, incomingRequest)

		close(done)
	})

	AfterEach(func() {
		fakeCloudController.Close()
	})

	Context("uploading the file, when all is well", func() {

		It("makes the request to CC", func() {
			Ω(fakeCloudController.ReceivedRequestsCount()).Should(Equal(1))
		})

		It("responds with 200 OK", func() {
			Ω(outgoingResponse.Code).Should(Equal(http.StatusOK))
		})

		It("uploads the correct file", func() {
			Ω(uploadedBytes).Should(Equal(uploadBody))
			Ω(uploadedFileName).Should(Equal("buildpack_cache.tgz"))
		})
	})

	Context("uploading the file, when the request is missing content length", func() {
		BeforeEach(func() {
			incomingRequest.ContentLength = -1
		})

		It("does not make the request to CC", func() {
			Ω(fakeCloudController.ReceivedRequestsCount()).Should(Equal(0))
		})

		It("responds with 411", func() {
			Ω(outgoingResponse.Code).Should(Equal(http.StatusLengthRequired))
		})
	})

	Context("when CC returns a non-succesful status code", func() {
		BeforeEach(func() {
			postStatusCode = 403
			postResponseBody = ""
		})

		It("makes the request to CC", func() {
			Ω(fakeCloudController.ReceivedRequestsCount()).Should(Equal(1))
		})

		It("responds with the status code from the CC request", func() {
			Ω(outgoingResponse.Code).Should(Equal(postStatusCode))

			data, err := ioutil.ReadAll(outgoingResponse.Body)
			Ω(err).ShouldNot(HaveOccurred())
			Ω(string(data)).Should(ContainSubstring(strconv.Itoa(postStatusCode)))
		})
	})
})
