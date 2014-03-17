package upload_droplet_test

import (
	"bytes"
	"fmt"
	"github.com/cloudfoundry-incubator/file-server/config"
	"github.com/cloudfoundry-incubator/file-server/handlers"
	"github.com/cloudfoundry-incubator/runtime-schema/router"
	"github.com/cloudfoundry/gosteno"
	ts "github.com/cloudfoundry/gunk/test_server"
	"github.com/cloudfoundry/gunk/urljoiner"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"time"
)

var _ = Describe("UploadDroplet", func() {
	var (
		fakeCloudController *ts.Server
		postStatusCode      int
		postResponseBody    string
		uploadedBytes       []byte
		uploadedFileName    string

		incomingRequest  *http.Request
		outgoingResponse *httptest.ResponseRecorder
	)

	PollingResponseBody := func(jobGuid, status string, fullUrl bool) string {
		url := urljoiner.Join("/v2/jobs", jobGuid)
		if fullUrl {
			url = urljoiner.Join(fakeCloudController.URL(), url)
		}
		return fmt.Sprintf(`
				{
					"metadata":{
						"guid": "%s",
						"url": "%s"
					},
					"entity": {
						"status": "%s"
					}
				}
			`, jobGuid, url, status)
	}

	VerifyPollingRequest := func(jobGuid, status string, timeClicker chan time.Time) http.HandlerFunc {
		return ts.CombineHandlers(
			ts.VerifyRequest("GET", urljoiner.Join("/v2/jobs/", jobGuid)),
			ts.Respond(http.StatusOK, PollingResponseBody(jobGuid, status, false)),
			func(w http.ResponseWriter, r *http.Request) {
				timeClicker <- time.Now()
			},
		)
	}

	BeforeEach(func() {
		uploadedBytes = nil
		uploadedFileName = ""

		fakeCloudController = ts.New()

		fakeCloudController.Append(ts.CombineHandlers(
			ts.VerifyRequest("POST", "/staging/droplets/app-guid/upload", "async=true"),
			ts.VerifyBasicAuth("bob", "password"),
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
		buffer := bytes.NewBufferString("the file I'm uploading")
		incomingRequest, err = http.NewRequest("POST", "http://file-server.com/droplet/app-guid", buffer)
		Ω(err).ShouldNot(HaveOccurred())
	})

	JustBeforeEach(func(done Done) {
		conf := config.New()
		conf.CCAddress = fakeCloudController.URL()
		conf.CCUsername = "bob"
		conf.CCPassword = "password"
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
		postStatusCode = 0
		postResponseBody = ""
	})

	Context("uploading the file, when all is well", func() {
		var timeClicker chan time.Time

		BeforeEach(func() {
			postStatusCode = http.StatusCreated
			postResponseBody = PollingResponseBody("my-job-guid", "queued", true)
			timeClicker = make(chan time.Time, 3)
			fakeCloudController.Append(
				VerifyPollingRequest("my-job-guid", "queued", timeClicker),
				VerifyPollingRequest("my-job-guid", "running", timeClicker),
				VerifyPollingRequest("my-job-guid", "finished", timeClicker),
			)
		})

		It("calls all the requests", func() {
			Ω(fakeCloudController.ReceivedRequests).Should(HaveLen(4))
		})

		It("responds with 201 CREATED", func() {
			Ω(outgoingResponse.Code).Should(Equal(http.StatusCreated))
		})

		It("uploads the correct file", func() {
			Ω(uploadedBytes).Should(Equal([]byte("the file I'm uploading")))
			Ω(uploadedFileName).Should(Equal("droplet.tgz"))
		})

		It("should wait between polls", func() {
			firstTime := <-timeClicker
			secondTime := <-timeClicker
			thirdTime := <-timeClicker

			Ω(secondTime.Sub(firstTime)).Should(BeNumerically(">", 5*time.Millisecond))
			Ω(thirdTime.Sub(secondTime)).Should(BeNumerically(">", 5*time.Millisecond))
		})
	})

	Context("uploading the file, when the job fails", func() {
		var timeClicker chan time.Time

		BeforeEach(func() {
			postStatusCode = http.StatusCreated
			postResponseBody = PollingResponseBody("my-job-guid", "queued", true)
			timeClicker = make(chan time.Time, 3)
			fakeCloudController.Append(
				VerifyPollingRequest("my-job-guid", "queued", timeClicker),
				VerifyPollingRequest("my-job-guid", "running", timeClicker),
				VerifyPollingRequest("my-job-guid", "failed", timeClicker),
				VerifyPollingRequest("my-job-guid", "finished", timeClicker),
			)
		})

		It("stops polling after the first fail", func() {
			Ω(fakeCloudController.ReceivedRequests).Should(HaveLen(4))
		})

		It("responds with 500", func() {
			Ω(outgoingResponse.Code).Should(Equal(http.StatusInternalServerError))
		})
	})

	Context("uploading the file, when the request is missing content length", func() {
		BeforeEach(func() {
			incomingRequest.ContentLength = -1
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

		It("should make the request", func() {
			Ω(fakeCloudController.ReceivedRequests).Should(HaveLen(1))
		})

		It("should pass along that status code", func() {
			Ω(outgoingResponse.Code).Should(Equal(403))

			data, err := ioutil.ReadAll(outgoingResponse.Body)
			Ω(err).ShouldNot(HaveOccurred())
			Ω(string(data)).Should(ContainSubstring("403"))
		})
	})
})
