package upload_build_artifacts_test

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/cloudfoundry-incubator/file-server/handlers/upload_build_artifacts"
	"github.com/cloudfoundry-incubator/file-server/uploader/fake_uploader"
	"github.com/cloudfoundry-incubator/runtime-schema/models"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-golang/lager"
)

var _ = Describe("UploadBuildArtifacts", func() {
	Describe("ServeHTTP", func() {
		var incomingRequest *http.Request
		var outgoingResponse *httptest.ResponseRecorder
		var uploader fake_uploader.FakeUploader
		var logger lager.Logger

		JustBeforeEach(func() {
			logger = lager.NewLogger("fake-logger")
			buildArtifactsUploadHandler := upload_build_artifacts.New(&uploader, logger)

			outgoingResponse = httptest.NewRecorder()
			buildArtifactsUploadHandler.ServeHTTP(outgoingResponse, incomingRequest)
		})

		Context("When the request does not include a build artifacts upload URI", func() {
			BeforeEach(func() {
				var err error
				incomingRequest, err = http.NewRequest("POST", "http://example.com", bytes.NewBufferString(""))
				Ω(err).ShouldNot(HaveOccurred())

				uploader = fake_uploader.FakeUploader{}
			})

			It("responds with an error code", func() {
				Ω(outgoingResponse.Code).Should(Equal(http.StatusInternalServerError))
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

				uploader = fake_uploader.FakeUploader{}
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
					fmt.Sprintf("http://example.com?%s=upload-uri.com", models.CcBuildArtifactsUploadUriKey),
					bytes.NewBufferString(""),
				)
				Ω(err).ShouldNot(HaveOccurred())

				uploader = fake_uploader.FakeUploader{}
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
			BeforeEach(func() {
				var err error
				incomingRequest, err = http.NewRequest(
					"POST",
					fmt.Sprintf("http://example.com?%s=upload-uri.com", models.CcBuildArtifactsUploadUriKey),
					bytes.NewBufferString(""),
				)
				Ω(err).ShouldNot(HaveOccurred())

				uploader = fake_uploader.FakeUploader{}
				uploader.UploadReturns(&http.Response{StatusCode: http.StatusOK}, nil, nil)
			})

			It("responds with a status OK", func() {
				Ω(outgoingResponse.Code).Should(Equal(http.StatusOK))
			})
		})
	})
})
