package uploader_test

import (
	"bytes"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"

	"github.com/cloudfoundry-incubator/file-server/uploader"
	"github.com/cloudfoundry-incubator/file-server/uploader/test_helpers"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Uploader", func() {
	var (
		u         uploader.Uploader
		baseURL   *url.URL
		transport http.RoundTripper
	)

	Describe("Upload", func() {
		var (
			response  *http.Response
			usedURL   *url.URL
			uploadErr error

			primaryURL      *url.URL
			filename        string
			incomingRequest *http.Request
		)

		JustBeforeEach(func() {
			u = uploader.New(baseURL, transport)
			response, usedURL, uploadErr = u.Upload(primaryURL, filename, incomingRequest)
		})

		Context("Validating the content length of the request", func() {
			BeforeEach(func() {
				baseURL = &url.URL{}
				transport = http.DefaultTransport

				primaryURL, _ = url.Parse("http://example.com")
				filename = "filename"
				incomingRequest = &http.Request{}
			})

			It("fails early if the content length is 0", func() {
				Ω(response.StatusCode).Should(Equal(http.StatusLengthRequired))

				Ω(uploadErr).Should(HaveOccurred())
			})
		})

		Context("When it can create a valid multipart request to the primary URL", func() {
			var uploadRequestChan chan *http.Request

			BeforeEach(func() {
				baseURL = &url.URL{}
				uploadRequestChan = make(chan *http.Request, 2)
				transport = test_helpers.NewFakeRoundTripper(
					uploadRequestChan,
					map[string]test_helpers.RespErrorPair{
						"example.com": {responseWithCode(http.StatusOK), nil},
					},
				)

				primaryURL, _ = url.Parse("http://example.com")
				filename = "filename"
				incomingRequest = createValidRequest()
			})

			It("Makes an upload request using that multipart request", func() {
				var uploadRequest *http.Request
				Eventually(uploadRequestChan).Should(Receive(&uploadRequest))
				Ω(uploadRequest.Header.Get("Content-Type")).Should(ContainSubstring("multipart/form-data; boundary="))
			})

			It("Forwards Content-MD5 header onto the upload request", func() {
				var uploadRequest *http.Request
				Eventually(uploadRequestChan).Should(Receive(&uploadRequest))
				Ω(uploadRequest.Header.Get("Content-MD5")).Should(Equal("the-md5"))
			})

			Context("When the primary URL has basic auth credentials", func() {
				BeforeEach(func() {
					primaryURL.User = url.UserPassword("bob", "cobb")
				})

				It("Forwards the basic auth credentials", func() {
					var uploadRequest *http.Request
					Eventually(uploadRequestChan).Should(Receive(&uploadRequest))
					Ω(uploadRequest.URL.User).Should(Equal(url.UserPassword("bob", "cobb")))
				})
			})

			Context("When upload to the primary URL succeeds", func() {
				It("Returns the response, the upload URL it used (for subsequent polling), and no error", func() {
					Ω(response).Should(Equal(responseWithCode(http.StatusOK))) // assumes (*http.Client).do doesn't modify the response from the roundtripper
					Ω(usedURL).Should(Equal(primaryURL))
					Ω(uploadErr).ShouldNot(HaveOccurred())
				})
			})

			Context("When request to the primary URL fails due to a network error other than a dial error", func() {
				BeforeEach(func() {
					transport = test_helpers.NewFakeRoundTripper(
						uploadRequestChan,
						map[string]test_helpers.RespErrorPair{
							"example.com": {nil, &net.OpError{Op: "not-dial"}},
						},
					)
				})

				It("Returns the network error", func() {
					Ω(uploadErr).Should(HaveOccurred())

					urlErr, ok := uploadErr.(*url.Error)
					Ω(ok).Should(BeTrue())

					Ω(urlErr.Err).Should(Equal(&net.OpError{Op: "not-dial"}))
				})
			})

			Context("When request to the primary URL fails due to a bad response", func() {
				BeforeEach(func() {
					transport = test_helpers.NewFakeRoundTripper(
						uploadRequestChan,
						map[string]test_helpers.RespErrorPair{
							"example.com": {responseWithCode(http.StatusUnauthorized), nil},
						},
					)
				})

				It("Returns the response", func() {
					Ω(response).Should(Equal(responseWithCode(http.StatusUnauthorized))) // assumes (*http.Client).do doesn't modify the response from the roundtripper
				})
			})

			Context("When upload to the primary URL fails due to just a dial error", func() {
				BeforeEach(func() {
					baseURL, _ = url.Parse("http://all-your-base.com")

					transport = test_helpers.NewFakeRoundTripper(
						uploadRequestChan,
						map[string]test_helpers.RespErrorPair{
							"example.com":       {nil, &net.OpError{Op: "dial"}},
							"all-your-base.com": {responseWithCode(http.StatusOK), nil},
						},
					)
				})

				JustBeforeEach(func() {
					var primaryUploadRequest *http.Request
					Eventually(uploadRequestChan).Should(Receive(&primaryUploadRequest))
					Ω(primaryUploadRequest.URL.Host).Should(Equal("example.com"))
				})

				It("Makes the upload request to the base URL", func() {
					var uploadRequest *http.Request
					Eventually(uploadRequestChan).Should(Receive(&uploadRequest))
					Ω(uploadRequest.URL).Should(Equal(baseURL))
				})

				Context("When it can create a valid multipart request to the base URL", func() {
					It("Makes an upload request using that multipart request", func() {
						var uploadRequest *http.Request
						Eventually(uploadRequestChan).Should(Receive(&uploadRequest))
						Ω(uploadRequest.Header.Get("Content-Type")).Should(ContainSubstring("multipart/form-data; boundary="))
					})

					It("Forwards Content-MD5 header onto the upload request", func() {
						var uploadRequest *http.Request
						Eventually(uploadRequestChan).Should(Receive(&uploadRequest))
						Ω(uploadRequest.Header.Get("Content-MD5")).Should(Equal("the-md5"))
					})

					Context("When the basic URL has base auth credentials", func() {
						BeforeEach(func() {
							baseURL.User = url.UserPassword("mary", "jane")
						})

						It("Forwards the basic auth credentials", func() {
							var uploadRequest *http.Request
							Eventually(uploadRequestChan).Should(Receive(&uploadRequest))
							Ω(uploadRequest.URL.User).Should(Equal(url.UserPassword("mary", "jane")))
						})
					})

					Context("When upload to the base URL succeeds", func() {
						It("Returns the response, the upload URL it used (for subsequent polling), and no error", func() {
							Ω(response).Should(Equal(responseWithCode(http.StatusOK))) // assumes (*http.Client).do doesn't modify the response from the roundtripper
							Ω(usedURL).Should(Equal(baseURL))
							Ω(uploadErr).ShouldNot(HaveOccurred())
						})
					})

					Context("When upload to the base URL fails", func() {
						BeforeEach(func() {
							transport = test_helpers.NewFakeRoundTripper(
								uploadRequestChan,
								map[string]test_helpers.RespErrorPair{
									"example.com":       {nil, &net.OpError{Op: "dial"}},
									"all-your-base.com": {nil, &net.OpError{Op: "not-dial"}},
								},
							)
						})

						It("Returns the response, and an error", func() {
							Ω(uploadErr).Should(HaveOccurred())
						})
					})
				})
			})
		})
	})
})

func createValidRequest() *http.Request {
	buffer := bytes.NewBufferString("file-upload-contents")
	request, err := http.NewRequest("POST", "", buffer)
	Ω(err).ShouldNot(HaveOccurred())

	request.Header.Set("Content-MD5", "the-md5")
	request.Body = ioutil.NopCloser(bytes.NewBufferString(""))

	return request
}

func responseWithCode(code int) *http.Response {
	return &http.Response{StatusCode: code, Body: ioutil.NopCloser(bytes.NewBufferString(""))}
}
