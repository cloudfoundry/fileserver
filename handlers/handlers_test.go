package handlers_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"time"

	"github.com/cloudfoundry-incubator/file-server/ccclient"
	"github.com/cloudfoundry-incubator/file-server/handlers"
	"github.com/cloudfoundry-incubator/runtime-schema/models"
	"github.com/cloudfoundry/gunk/urljoiner"
	"github.com/pivotal-golang/lager/lagertest"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
)

var _ = Describe("Handlers", func() {
	var (
		logger *lagertest.TestLogger

		fakeCloudController *ghttp.Server
		primaryUrl          *url.URL

		incomingRequest  *http.Request
		outgoingResponse *httptest.ResponseRecorder

		handler http.Handler

		postStatusCode   int
		postResponseBody string
		uploadedBytes    []byte
		uploadedFileName string
		uploadedHeaders  http.Header
	)

	BeforeEach(func() {
		var err error

		logger = lagertest.NewTestLogger("test")

		buffer := bytes.NewBufferString("the file I'm uploading")
		incomingRequest, err = http.NewRequest("POST", "", buffer)
		Ω(err).ShouldNot(HaveOccurred())
		incomingRequest.Header.Set("Content-MD5", "the-md5")

		fakeCloudController = ghttp.NewServer()

		ccUrl, err := url.Parse(fakeCloudController.URL())
		Ω(err).ShouldNot(HaveOccurred())
		ccUrl.User = url.UserPassword("bob", "password")

		uploader := ccclient.NewUploader(logger, ccUrl, http.DefaultClient)
		poller := ccclient.NewPoller(logger, http.DefaultClient, 100*time.Millisecond)

		handler, err = handlers.New("", uploader, poller, logger)
		Ω(err).ShouldNot(HaveOccurred())

		postStatusCode = http.StatusCreated
		uploadedBytes = nil
		uploadedFileName = ""
		uploadedHeaders = nil
	})

	AfterEach(func() {
		fakeCloudController.Close()
	})

	Describe("UploadDroplet", func() {
		var (
			timeClicker chan time.Time
			startTime   time.Time
			endTime     time.Time
		)

		BeforeEach(func() {
			var err error

			timeClicker = make(chan time.Time, 4)
			fakeCloudController.AppendHandlers(ghttp.CombineHandlers(
				ghttp.VerifyRequest("POST", "/staging/droplet/app-guid/upload"),
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

			primaryUrl, err = url.Parse(fakeCloudController.URL())
			Ω(err).ShouldNot(HaveOccurred())

			primaryUrl.User = url.UserPassword("bob", "password")
			primaryUrl.Path = "/staging/droplet/app-guid/upload"
			primaryUrl.RawQuery = url.Values{"async": []string{"true"}}.Encode()
		})

		JustBeforeEach(func() {
			u, err := url.Parse("http://file-server.com/v1/droplet/app-guid")
			Ω(err).ShouldNot(HaveOccurred())

			v := url.Values{models.CcDropletUploadUriKey: []string{primaryUrl.String()}}
			u.RawQuery = v.Encode()
			incomingRequest.URL = u

			outgoingResponse = httptest.NewRecorder()

			startTime = time.Now()
			handler.ServeHTTP(outgoingResponse, incomingRequest)
			endTime = time.Now()
		})

		Context("uploading the file, when all is well", func() {
			BeforeEach(func() {
				postStatusCode = http.StatusCreated
				postResponseBody = pollingResponseBody("my-job-guid", "queued", fakeCloudController.URL())
				fakeCloudController.AppendHandlers(
					verifyPollingRequest("my-job-guid", "queued", timeClicker),
					verifyPollingRequest("my-job-guid", "running", timeClicker),
					verifyPollingRequest("my-job-guid", "finished", timeClicker),
				)
			})

			It("responds with 201 CREATED", func() {
				Ω(outgoingResponse.Code).Should(Equal(http.StatusCreated))
			})

			It("forwards the content-md5 header", func() {
				Ω(uploadedHeaders.Get("Content-MD5")).Should(Equal("the-md5"))
			})

			It("uploads the correct file", func() {
				Ω(uploadedBytes).Should(Equal([]byte("the file I'm uploading")))
				Ω(uploadedFileName).Should(Equal("droplet.tgz"))
			})

			It("should wait between polls", func() {
				var firstTime, secondTime, thirdTime time.Time
				Eventually(timeClicker).Should(Receive(&firstTime))
				Eventually(timeClicker).Should(Receive(&secondTime))
				Eventually(timeClicker).Should(Receive(&thirdTime))

				Ω(secondTime.Sub(firstTime)).Should(BeNumerically(">", 75*time.Millisecond))
				Ω(thirdTime.Sub(secondTime)).Should(BeNumerically(">", 75*time.Millisecond))
			})
		})

		Context("uploading the file, when the job fails", func() {
			BeforeEach(func() {
				postStatusCode = http.StatusCreated
				postResponseBody = pollingResponseBody("my-job-guid", "queued", fakeCloudController.URL())
				fakeCloudController.AppendHandlers(
					verifyPollingRequest("my-job-guid", "queued", timeClicker),
					verifyPollingRequest("my-job-guid", "failed", timeClicker),
				)
			})

			It("stops polling after the first fail", func() {
				Ω(fakeCloudController.ReceivedRequests()).Should(HaveLen(3))

				Ω(outgoingResponse.Code).Should(Equal(http.StatusInternalServerError))
			})
		})

		Context("uploading the file, when the inbound upload request is missing content length", func() {
			BeforeEach(func() {
				incomingRequest.ContentLength = -1
			})

			It("does not make the request to CC", func() {
				Ω(fakeCloudController.ReceivedRequests()).Should(HaveLen(0))
			})

			It("responds with 411", func() {
				Ω(outgoingResponse.Code).Should(Equal(http.StatusLengthRequired))
			})
		})

		Context("when CC returns a non-succesful status code", func() {
			BeforeEach(func() {
				postStatusCode = http.StatusForbidden
			})

			It("makes the request to CC", func() {
				Ω(fakeCloudController.ReceivedRequests()).Should(HaveLen(1))
			})

			It("responds with the status code from the CC request", func() {
				Ω(outgoingResponse.Code).Should(Equal(http.StatusForbidden))

				data, err := ioutil.ReadAll(outgoingResponse.Body)
				Ω(err).ShouldNot(HaveOccurred())
				Ω(string(data)).Should(ContainSubstring(strconv.Itoa(http.StatusForbidden)))
			})
		})
	})

	Describe("Uploading Build Artifacts", func() {
		BeforeEach(func() {
			var err error

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

			primaryUrl, err = url.Parse(fakeCloudController.URL())
			Ω(err).ShouldNot(HaveOccurred())

			primaryUrl.User = url.UserPassword("bob", "password")
			primaryUrl.Path = "/staging/buildpack_cache/app-guid/upload"
		})

		JustBeforeEach(func() {
			u, err := url.Parse("http://file-server.com/v1/build_artifacts/app-guid")
			Ω(err).ShouldNot(HaveOccurred())
			v := url.Values{models.CcBuildArtifactsUploadUriKey: []string{primaryUrl.String()}}
			u.RawQuery = v.Encode()
			incomingRequest.URL = u

			outgoingResponse = httptest.NewRecorder()

			handler.ServeHTTP(outgoingResponse, incomingRequest)
		})

		Context("uploading the file, when all is well", func() {
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

		Context("uploading the file, when the inbound upload request is missing content length", func() {
			BeforeEach(func() {
				incomingRequest.ContentLength = -1
			})

			It("does not make the request to CC", func() {
				Ω(fakeCloudController.ReceivedRequests()).Should(HaveLen(0))
			})

			It("responds with 411", func() {
				Ω(outgoingResponse.Code).Should(Equal(http.StatusLengthRequired))
			})
		})

		Context("when CC returns a non-succesful status code", func() {
			BeforeEach(func() {
				postStatusCode = http.StatusForbidden
			})

			It("makes the request to CC", func() {
				Ω(fakeCloudController.ReceivedRequests()).Should(HaveLen(1))
			})

			It("responds with the status code from the CC request", func() {
				Ω(outgoingResponse.Code).Should(Equal(http.StatusForbidden))

				data, err := ioutil.ReadAll(outgoingResponse.Body)
				Ω(err).ShouldNot(HaveOccurred())
				Ω(string(data)).Should(ContainSubstring(strconv.Itoa(http.StatusForbidden)))
			})
		})
	})
})

func pollingResponseBody(jobGuid, status string, baseUrl string) string {
	url := urljoiner.Join("/v2/jobs", jobGuid)
	if baseUrl != "" {
		url = urljoiner.Join(baseUrl, url)
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

func verifyPollingRequest(jobGuid, status string, timeClicker chan time.Time) http.HandlerFunc {
	return ghttp.CombineHandlers(
		ghttp.VerifyRequest("GET", urljoiner.Join("/v2/jobs/", jobGuid)),
		ghttp.RespondWith(http.StatusOK, pollingResponseBody(jobGuid, status, "")),
		func(w http.ResponseWriter, r *http.Request) {
			timeClicker <- time.Now()
		},
	)
}
