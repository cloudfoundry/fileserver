package upload_build_artifacts_test

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"net/http/httptest"

	"github.com/cloudfoundry-incubator/file-server/handlers"
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
		postStatusCode      int
		postResponseBody    string
		uploadedBytes       []byte
		uploadedFileName    string
		uploadedHeaders     http.Header

		incomingRequest  *http.Request
		outgoingResponse *httptest.ResponseRecorder
	)

	BeforeEach(func() {
		postStatusCode = 0

		uploadedHeaders = nil
		uploadedBytes = nil
		uploadedFileName = ""

		fakeCloudController = ts.New()

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
		buffer := bytes.NewBufferString("the file I'm uploading")
		incomingRequest, err = http.NewRequest("POST", "http://file-server.com/v1/build_artifacts/app-guid", buffer)
		incomingRequest.Header.Set("Content-MD5", "the-md5")

		Ω(err).ShouldNot(HaveOccurred())
	})

	JustBeforeEach(func(done Done) {
		conf := handlers.Config{
			CCAddress:  fakeCloudController.URL(),
			CCUsername: "bob",
			CCPassword: "password",
		}

		logger := lager.NewLogger("fakelogger")
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
			Ω(uploadedBytes).Should(Equal([]byte("the file I'm uploading")))
			Ω(uploadedFileName).Should(Equal("buildpack_cache.tgz"))
		})

		It("forwards the content-md5 header", func() {
			Ω(uploadedHeaders.Get("Content-MD5")).Should(Equal("the-md5"))
		})
	})

	ItFailsWhenTheContentLengthIsMissing(&incomingRequest, &outgoingResponse, &fakeCloudController)
	ItHandlesCCFailures(&postStatusCode, &outgoingResponse, &fakeCloudController)
})
