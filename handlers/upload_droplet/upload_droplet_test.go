package upload_droplet_test

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"

	"github.com/cloudfoundry-incubator/file-server/ccclient/fake_ccclient"
	"github.com/cloudfoundry-incubator/file-server/handlers/upload_droplet"
	"github.com/cloudfoundry-incubator/runtime-schema/models"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-golang/lager"
)

var _ = Describe("UploadDroplet", func() {
	Describe("ServeHTTP", func() {
		var incomingRequest *http.Request
		var outgoingResponse *httptest.ResponseRecorder
		var uploader fake_ccclient.FakeUploader
		var poller fake_ccclient.FakePoller
		var logger lager.Logger

		JustBeforeEach(func() {
			logger = lager.NewLogger("fake-logger")
			dropletUploadHandler := upload_droplet.New(&uploader, &poller, logger)

			outgoingResponse = httptest.NewRecorder()
			dropletUploadHandler.ServeHTTP(outgoingResponse, incomingRequest)
		})

		Context("When the request does not include a droplet upload URI", func() {
			BeforeEach(func() {
				var err error
				incomingRequest, err = http.NewRequest("POST", "http://example.com", bytes.NewBufferString(""))
				Ω(err).ShouldNot(HaveOccurred())

				uploader = fake_ccclient.FakeUploader{}
				poller = fake_ccclient.FakePoller{}
			})

			It("responds with an error code", func() {
				Ω(outgoingResponse.Code).Should(Equal(http.StatusInternalServerError))
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

				uploader = fake_ccclient.FakeUploader{}
				poller = fake_ccclient.FakePoller{}
			})

			It("responds adds the async=true query parameter to the upload URI for the upload request", func() {
				uploadUrl, _, _ := uploader.UploadArgsForCall(0)
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

				uploader = fake_ccclient.FakeUploader{}
				poller = fake_ccclient.FakePoller{}
				uploader.UploadReturns(nil, nil, errors.New("some-error"))
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

				uploader = fake_ccclient.FakeUploader{}
				poller = fake_ccclient.FakePoller{}
				uploader.UploadReturns(&http.Response{StatusCode: 404}, nil, errors.New("some-error"))
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
			var pollUrl *url.URL
			var uploadResponse *http.Response

			BeforeEach(func() {
				var err error
				incomingRequest, err = http.NewRequest(
					"POST",
					fmt.Sprintf("http://example.com?%s=upload-uri.com", models.CcDropletUploadUriKey),
					bytes.NewBufferString(""),
				)
				Ω(err).ShouldNot(HaveOccurred())

				var urlParseErr error
				pollUrl, urlParseErr = url.Parse("http://poll-url.com")
				Ω(urlParseErr).ShouldNot(HaveOccurred())
				uploadResponse = &http.Response{StatusCode: http.StatusOK}
				uploader = fake_ccclient.FakeUploader{}
				poller = fake_ccclient.FakePoller{}
				uploader.UploadReturns(uploadResponse, pollUrl, nil)
			})

			It("Polls for success of the upload", func() {
				pollArgsURL, pollArgsUploadResponse, _ := poller.PollArgsForCall(0)
				Ω(pollArgsURL).Should(Equal(pollUrl))
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
	})
})
