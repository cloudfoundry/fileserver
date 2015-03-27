package upload_droplet_test

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"time"

	"github.com/cloudfoundry-incubator/file-server/ccclient/fake_ccclient"
	"github.com/cloudfoundry-incubator/file-server/handlers/test_helpers"
	"github.com/cloudfoundry-incubator/file-server/handlers/upload_droplet"
	"github.com/cloudfoundry-incubator/runtime-schema/models"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-golang/lager"
)

var _ = Describe("UploadDroplet", func() {
	Describe("ServeHTTP", func() {
		var incomingRequest *http.Request
		var responseWriter http.ResponseWriter
		var outgoingResponse *httptest.ResponseRecorder
		var uploader fake_ccclient.FakeUploader
		var poller fake_ccclient.FakePoller
		var logger lager.Logger

		BeforeEach(func() {
			outgoingResponse = httptest.NewRecorder()
			responseWriter = outgoingResponse
			uploader = fake_ccclient.FakeUploader{}
			poller = fake_ccclient.FakePoller{}
		})

		JustBeforeEach(func() {
			logger = lager.NewLogger("fake-logger")
			dropletUploadHandler := upload_droplet.New(&uploader, &poller, logger)

			dropletUploadHandler.ServeHTTP(responseWriter, incomingRequest)
		})

		Context("When the request does not include a droplet upload URI", func() {
			BeforeEach(func() {
				var err error
				incomingRequest, err = http.NewRequest("POST", "http://example.com", bytes.NewBufferString(""))
				Ω(err).ShouldNot(HaveOccurred())
			})

			It("responds with an error code", func() {
				Ω(outgoingResponse.Code).Should(Equal(http.StatusBadRequest))
			})

			It("does not attempt to upload", func() {
				Ω(uploader.UploadCallCount()).Should(BeZero())
			})

			It("responds with the error message in the body", func() {
				body, _ := outgoingResponse.Body.ReadString('\n')
				Ω(body).Should(Equal(upload_droplet.MissingCCDropletUploadUriKeyError.Error()))
			})
		})

		Context("When the request includes a droplet upload URI", func() {
			BeforeEach(func() {
				var err error
				incomingRequest, err = http.NewRequest(
					"POST",
					fmt.Sprintf("http://example.com?%s=upload-uri.com", models.CcDropletUploadUriKey),
					bytes.NewBufferString(""),
				)
				Ω(err).ShouldNot(HaveOccurred())
			})

			It("responds adds the async=true query parameter to the upload URI for the upload request", func() {
				uploadUrl, _, _, _ := uploader.UploadArgsForCall(0)
				Ω(uploadUrl).Should(MatchRegexp("async=true"))
			})
		})

		Context("When it fails to make the upload request to the upload URI", func() {
			BeforeEach(func() {
				var err error
				incomingRequest, err = http.NewRequest(
					"POST",
					fmt.Sprintf("http://example.com?%s=upload-uri.com", models.CcDropletUploadUriKey),
					bytes.NewBufferString(""),
				)
				Ω(err).ShouldNot(HaveOccurred())

				uploader.UploadReturns(nil, errors.New("some-error"))
			})

			It("responds with an error code", func() {
				Ω(outgoingResponse.Code).Should(Equal(http.StatusInternalServerError))
			})

			It("responds with the error message in the body", func() {
				body, _ := outgoingResponse.Body.ReadString('\n')
				Ω(body).Should(Equal("some-error"))
			})
		})

		Context("When the request to the upload URI responds with a failed status", func() {
			BeforeEach(func() {
				var err error
				incomingRequest, err = http.NewRequest(
					"POST",
					fmt.Sprintf("http://example.com?%s=upload-uri.com", models.CcDropletUploadUriKey),
					bytes.NewBufferString(""),
				)
				Ω(err).ShouldNot(HaveOccurred())

				uploader.UploadReturns(&http.Response{StatusCode: 404}, errors.New("some-error"))
			})

			It("responds with an error code", func() {
				Ω(outgoingResponse.Code).Should(Equal(404))
			})

			It("responds with the error message in the body", func() {
				body, _ := outgoingResponse.Body.ReadString('\n')
				Ω(body).Should(Equal("some-error"))
			})
		})

		Context("When the upload succeeds", func() {
			var uploadResponse *http.Response

			BeforeEach(func() {
				var err error
				incomingRequest, err = http.NewRequest(
					"POST",
					fmt.Sprintf("http://example.com?%s=upload-uri.com", models.CcDropletUploadUriKey),
					bytes.NewBufferString(""),
				)
				Ω(err).ShouldNot(HaveOccurred())

				uploadResponse = &http.Response{StatusCode: http.StatusOK}

				uploader.UploadReturns(uploadResponse, nil)
			})

			It("Polls for success of the upload", func() {
				uploadURL, _, _, _ := uploader.UploadArgsForCall(0)
				pollArgsURL, pollArgsUploadResponse, _ := poller.PollArgsForCall(0)
				Ω(pollArgsURL).Should(Equal(uploadURL))
				Ω(pollArgsUploadResponse).Should(Equal(uploadResponse))
			})

			Context("When polling for success of the upload fails", func() {
				BeforeEach(func() {
					poller.PollReturns(errors.New("poll-error"))
				})

				It("responds with an error code", func() {
					Ω(outgoingResponse.Code).Should(Equal(http.StatusInternalServerError))
				})

				It("responds with the error message in the body", func() {
					body, _ := outgoingResponse.Body.ReadString('\n')
					Ω(body).Should(Equal("poll-error"))
				})
			})

			Context("When polling for success of the upload succeeds", func() {
				BeforeEach(func() {
					poller.PollReturns(nil)
				})

				It("responds with a status created", func() {
					Ω(outgoingResponse.Code).Should(Equal(http.StatusCreated))
				})
			})
		})

		Context("when the requester (client) goes away", func() {
			var fakeResponseWriter *test_helpers.FakeResponseWriter

			BeforeEach(func() {
				var err error
				incomingRequest, err = http.NewRequest(
					"POST",
					fmt.Sprintf("http://example.com?%s=upload-uri.com", models.CcDropletUploadUriKey),
					bytes.NewBufferString(""),
				)
				Ω(err).ShouldNot(HaveOccurred())
			})

			Context("and we are uploading", func() {
				BeforeEach(func() {
					closedChan := make(chan bool)
					fakeResponseWriter = test_helpers.NewFakeResponseWriter(closedChan)
					responseWriter = fakeResponseWriter

					uploader.UploadStub = func(uploadURL *url.URL, filename string, r *http.Request, cancelChan <-chan struct{}) (*http.Response, error) {
						closedChan <- true
						Eventually(cancelChan).Should(BeClosed())
						return nil, errors.New("cancelled")
					}
				})

				It("responds with an error code", func() {
					Ω(fakeResponseWriter.Code).Should(Equal(http.StatusInternalServerError))
				})
			})

			Context("and we are polling", func() {
				BeforeEach(func() {
					uploadResponse := &http.Response{StatusCode: http.StatusOK}
					uploader.UploadReturns(uploadResponse, nil)

					closedChan := make(chan bool)
					fakeResponseWriter = test_helpers.NewFakeResponseWriter(closedChan)
					responseWriter = fakeResponseWriter

					poller.PollStub = func(fallbackURL *url.URL, res *http.Response, cancelChan <-chan struct{}) error {
						closedChan <- true
						Eventually(cancelChan).Should(BeClosed())
						return errors.New("cancelled")
					}
				})

				It("responds with an error code", func() {
					Ω(fakeResponseWriter.Code).Should(Equal(http.StatusInternalServerError))
				})
			})
		})

		Context("when the request times out", func() {
			BeforeEach(func() {
				var err error
				incomingRequest, err = http.NewRequest(
					"POST",
					fmt.Sprintf("http://example.com?%s=upload-uri.com&timeout=1", models.CcDropletUploadUriKey),
					bytes.NewBufferString(""),
				)
				Ω(err).ShouldNot(HaveOccurred())
			})

			Context("and we are uploading", func() {
				BeforeEach(func() {
					uploader.UploadStub = func(uploadURL *url.URL, filename string, r *http.Request, cancelChan <-chan struct{}) (*http.Response, error) {
						Eventually(cancelChan, 2*time.Second).Should(BeClosed())
						return nil, errors.New("timeout")
					}
				})

				It("responds with an error code", func() {
					Ω(outgoingResponse.Code).Should(Equal(http.StatusInternalServerError))
				})
			})

			Context("and we are polling", func() {
				BeforeEach(func() {
					poller.PollStub = func(fallbackURL *url.URL, res *http.Response, cancelChan <-chan struct{}) error {
						Eventually(cancelChan, 2*time.Second).Should(BeClosed())
						return errors.New("timeout")
					}
				})

				It("responds with an error code", func() {
					Ω(outgoingResponse.Code).Should(Equal(http.StatusInternalServerError))
				})
			})
		})
	})
})
