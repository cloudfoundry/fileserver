package upload_droplet_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"time"

	"github.com/cloudfoundry-incubator/file-server/handlers"
	"github.com/cloudfoundry-incubator/runtime-schema/models"
	"github.com/cloudfoundry-incubator/runtime-schema/router"
	ts "github.com/cloudfoundry/gunk/test_server"
	"github.com/cloudfoundry/gunk/urljoiner"
	"github.com/pivotal-golang/lager"

	. "github.com/cloudfoundry-incubator/file-server/handlers/test_helpers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("UploadDroplet", func() {
	var (
		fakeCloudController *ts.Server
		primaryUrl          *url.URL
		config              handlers.Config

		ccUrl            string
		postStatusCode   int
		postResponseBody string
		queryMatch       string
		uploadedBytes    []byte
		uploadedFileName string
		uploadedHeaders  http.Header
		timeClicker      chan time.Time
		startTime        time.Time
		endTime          time.Time

		incomingRequest  *http.Request
		outgoingResponse *httptest.ResponseRecorder
	)

	BeforeEach(func() {
		timeClicker = make(chan time.Time, 4)
		uploadedBytes = nil
		uploadedFileName = ""
		uploadedHeaders = nil

		fakeCloudController = ts.New()
		ccUrl = fakeCloudController.URL()

		config = handlers.Config{
			CCAddress:            fakeCloudController.URL(),
			CCUsername:           "bob",
			CCPassword:           "password",
			CCJobPollingInterval: 100 * time.Millisecond,
		}

		queryMatch = "async=true"

		fakeCloudController.Append(ts.CombineHandlers(
			ts.VerifyRequest("POST", "/staging/droplet/app-guid/upload"),
			ts.VerifyBasicAuth("bob", "password"),
			ts.RespondPtr(&postStatusCode, &postResponseBody),
			func(w http.ResponseWriter, r *http.Request) {
				Ω(r.URL.RawQuery).Should(Equal(queryMatch))
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
		primaryUrl, err = url.Parse(ccUrl)
		Ω(err).ShouldNot(HaveOccurred())
		primaryUrl.User = url.UserPassword("bob", "password")
		primaryUrl.Path = "/staging/droplet/app-guid/upload"
		v := url.Values{"async": []string{"true"}}
		primaryUrl.RawQuery = v.Encode()

		buffer := bytes.NewBufferString("the file I'm uploading")
		incomingRequest, err = http.NewRequest("POST", "", buffer)
		incomingRequest.Header.Set("Content-MD5", "the-md5")
	})

	JustBeforeEach(func() {
		logger := lager.NewLogger("fakelogger")
		r, err := router.NewFileServerRoutes().Router(handlers.New(config, logger))
		Ω(err).ShouldNot(HaveOccurred())

		u, err := url.Parse("http://file-server.com/v1/droplet/app-guid")
		Ω(err).ShouldNot(HaveOccurred())
		v := url.Values{models.CcDropletUploadUriKey: []string{primaryUrl.String()}}
		u.RawQuery = v.Encode()
		incomingRequest.URL = u

		outgoingResponse = httptest.NewRecorder()

		startTime = time.Now()
		r.ServeHTTP(outgoingResponse, incomingRequest)
		endTime = time.Now()
	})

	AfterEach(func() {
		fakeCloudController.Close()
		postStatusCode = 0
		postResponseBody = ""
	})

	Context("uploading the file, when there is no polling", func() {
		BeforeEach(func() {
			postStatusCode = http.StatusCreated
			postResponseBody = PollingResponseBody("my-job-guid", "finished", ccUrl)
		})

		It("should not wait for the polling interval", func() {
			Ω(endTime.Sub(startTime)).Should(BeNumerically("<", 75*time.Millisecond))
		})
	})

	Context("uploading the file, when all is well", func() {

		BeforeEach(func() {
			postStatusCode = http.StatusCreated
			postResponseBody = PollingResponseBody("my-job-guid", "queued", ccUrl)
			fakeCloudController.Append(
				VerifyPollingRequest("my-job-guid", "queued", timeClicker),
				VerifyPollingRequest("my-job-guid", "running", timeClicker),
				VerifyPollingRequest("my-job-guid", "finished", timeClicker),
			)
		})

		Context("when the primary url works", func() {
			BeforeEach(func() {
				config.CCAddress = "127.0.0.1:0"
			})

			ItSucceeds := func() {
				It("calls all the requests", func() {
					Ω(fakeCloudController.ReceivedRequestsCount()).Should(Equal(4))

					By("responds with 201 CREATED", func() {
						Ω(outgoingResponse.Code).Should(Equal(http.StatusCreated))
					})

					By("forwards the content-md5 header", func() {
						Ω(uploadedHeaders.Get("Content-MD5")).Should(Equal("the-md5"))
					})

					By("uploads the correct file", func() {
						Ω(uploadedBytes).Should(Equal([]byte("the file I'm uploading")))
						Ω(uploadedFileName).Should(Equal("droplet.tgz"))
					})
				})
			}

			ItSucceeds()

			It("should wait between polls", func() {
				var firstTime, secondTime, thirdTime time.Time
				Eventually(timeClicker).Should(Receive(&firstTime))
				Eventually(timeClicker).Should(Receive(&secondTime))
				Eventually(timeClicker).Should(Receive(&thirdTime))

				Ω(secondTime.Sub(firstTime)).Should(BeNumerically(">", 75*time.Millisecond))
				Ω(thirdTime.Sub(secondTime)).Should(BeNumerically(">", 75*time.Millisecond))
			})

			Context("when async=true is not included", func() {
				Context("no query parameters", func() {
					BeforeEach(func() {
						config.CCAddress = "127.0.0.1:0"
						primaryUrl.RawQuery = ""
					})

					ItSucceeds()
				})

				Context("other query parameters", func() {
					BeforeEach(func() {
						config.CCAddress = "127.0.0.1:0"
						primaryUrl.RawQuery = "a=b"
						queryMatch = "a=b&async=true"
					})

					ItSucceeds()
				})
			})

		})

		Context("when the primary url fails", func() {
			BeforeEach(func() {
				primaryUrl.Host = "127.0.0.1:0"
			})

			It("falls over to the secondary url", func() {
				Ω(fakeCloudController.ReceivedRequestsCount()).Should(Equal(4))

				By("responds with 201 CREATED", func() {
					Ω(outgoingResponse.Code).Should(Equal(http.StatusCreated))
				})
			})
		})
	})

	Context("uploading the file, when the job fails", func() {
		BeforeEach(func() {
			postStatusCode = http.StatusCreated
			postResponseBody = PollingResponseBody("my-job-guid", "queued", ccUrl)
			fakeCloudController.Append(
				VerifyPollingRequest("my-job-guid", "queued", timeClicker),
				VerifyPollingRequest("my-job-guid", "running", timeClicker),
				VerifyPollingRequest("my-job-guid", "finished", timeClicker),
			)
		})

		It("stops polling after the first fail", func() {
			Ω(fakeCloudController.ReceivedRequestsCount()).Should(Equal(4))

			By("responds with 201", func() {
				Ω(outgoingResponse.Code).Should(Equal(http.StatusCreated))
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

func PollingResponseBody(jobGuid, status string, baseUrl string) string {
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

func VerifyPollingRequest(jobGuid, status string, timeClicker chan time.Time) http.HandlerFunc {
	return ts.CombineHandlers(
		ts.VerifyRequest("GET", urljoiner.Join("/v2/jobs/", jobGuid)),
		ts.Respond(http.StatusOK, PollingResponseBody(jobGuid, status, "")),
		func(w http.ResponseWriter, r *http.Request) {
			timeClicker <- time.Now()
		},
	)
}
