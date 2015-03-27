package upload_build_artifacts_test

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
	"github.com/cloudfoundry-incubator/file-server/handlers/upload_build_artifacts"
	"github.com/cloudfoundry-incubator/runtime-schema/models"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-golang/lager"
)

var _ = Describe("UploadBuildArtifacts", func() {
	Describe("ServeHTTP", func() {
		var incomingRequest *http.Request
		var responseWriter http.ResponseWriter
		var outgoingResponse *httptest.ResponseRecorder
		var uploader fake_ccclient.FakeUploader
		var logger lager.Logger

		BeforeEach(func() {
			outgoingResponse = httptest.NewRecorder()
			responseWriter = outgoingResponse
		})

		JustBeforeEach(func() {
			logger = lager.NewLogger("fake-logger")
			buildArtifactsUploadHandler := upload_build_artifacts.New(&uploader, logger)

			buildArtifactsUploadHandler.ServeHTTP(responseWriter, incomingRequest)
		})

		Context("When the request does not include a build artifacts upload URI", func() {
			BeforeEach(func() {
				var err error
				incomingRequest, err = http.NewRequest("POST", "http://example.com", bytes.NewBufferString(""))
				Ω(err).ShouldNot(HaveOccurred())

				uploader = fake_ccclient.FakeUploader{}
			})

			It("responds with an error code", func() {
				Ω(outgoingResponse.Code).Should(Equal(http.StatusBadRequest))
			})

			It("does not attempt to upload", func() {
				Ω(uploader.UploadCallCount()).Should(BeZero())
			})

			It("responds with the error message in the body", func() {
				body, _ := outgoingResponse.Body.ReadString('\n')
				Ω(body).Should(Equal(upload_build_artifacts.MissingCCBuildArtifactsUploadUriKeyError.Error()))
			})
		})

		Context("When it fails to make the upload request to the upload URI", func() {
			BeforeEach(func() {
				var err error
				incomingRequest, err = http.NewRequest(
					"POST",
					fmt.Sprintf("http://example.com?%s=upload-uri.com", models.CcBuildArtifactsUploadUriKey),
					bytes.NewBufferString(""),
				)
				Ω(err).ShouldNot(HaveOccurred())

				uploader = fake_ccclient.FakeUploader{}
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
					fmt.Sprintf("http://example.com?%s=upload-uri.com", models.CcBuildArtifactsUploadUriKey),
					bytes.NewBufferString(""),
				)
				Ω(err).ShouldNot(HaveOccurred())

				uploader = fake_ccclient.FakeUploader{}
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
			BeforeEach(func() {
				var err error
				incomingRequest, err = http.NewRequest(
					"POST",
					fmt.Sprintf("http://example.com?%s=upload-uri.com", models.CcBuildArtifactsUploadUriKey),
					bytes.NewBufferString(""),
				)
				Ω(err).ShouldNot(HaveOccurred())

				uploader = fake_ccclient.FakeUploader{}
				uploader.UploadReturns(&http.Response{StatusCode: http.StatusOK}, nil)
			})

			It("responds with a status OK", func() {
				Ω(outgoingResponse.Code).Should(Equal(http.StatusOK))
			})
		})

		Context("when the requester (client) goes away", func() {
			var fakeResponseWriter *test_helpers.FakeResponseWriter

			BeforeEach(func() {
				var err error
				incomingRequest, err = http.NewRequest(
					"POST",
					fmt.Sprintf("http://example.com?%s=upload-uri.com", models.CcBuildArtifactsUploadUriKey),
					bytes.NewBufferString(""),
				)
				Ω(err).ShouldNot(HaveOccurred())

				closedChan := make(chan bool)
				fakeResponseWriter = test_helpers.NewFakeResponseWriter(closedChan)
				responseWriter = fakeResponseWriter

				uploader = fake_ccclient.FakeUploader{}
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

		Context("when the request times out", func() {
			BeforeEach(func() {
				var err error
				incomingRequest, err = http.NewRequest(
					"POST",
					fmt.Sprintf("http://example.com?%s=upload-uri.com&timeout=1", models.CcBuildArtifactsUploadUriKey),
					bytes.NewBufferString(""),
				)
				Ω(err).ShouldNot(HaveOccurred())

				uploader = fake_ccclient.FakeUploader{}
				uploader.UploadStub = func(uploadURL *url.URL, filename string, r *http.Request, cancelChan <-chan struct{}) (*http.Response, error) {
					Eventually(cancelChan, 2*time.Second).Should(BeClosed())
					return nil, errors.New("cancelled")
				}
			})

			It("responds with an error code", func() {
				Ω(outgoingResponse.Code).Should(Equal(http.StatusInternalServerError))
			})
		})
	})
})
