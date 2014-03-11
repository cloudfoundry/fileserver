package upload_droplet

import (
	"bytes"
	"encoding/json"
	"fmt"
	steno "github.com/cloudfoundry/gosteno"
	"github.com/cloudfoundry/gunk/http_client"
	"github.com/cloudfoundry/gunk/urljoiner"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/url"
	"time"
)

func New(addr, username, password string, pollingInterval time.Duration, skipCertVerification bool, logger *steno.Logger) http.Handler {
	return &dropletUploader{
		addr:            addr,
		username:        username,
		password:        password,
		pollingInterval: pollingInterval,
		client:          http_client.New(skipCertVerification),
		logger:          logger,
	}
}

type dropletUploader struct {
	addr            string
	username        string
	password        string
	pollingInterval time.Duration
	client          *http.Client
	logger          *steno.Logger
}

func (h *dropletUploader) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	if r.ContentLength <= 0 {
		h.handleError(w, r, fmt.Errorf("Mssing Content Length"), http.StatusLengthRequired)
		return
	}

	var closeChan <-chan bool

	closeNotifier, ok := w.(http.CloseNotifier)
	if ok {
		closeChan = closeNotifier.CloseNotify()
	}

	// cloud controller droplet upload url
	// TODO: this should be refactored into runtime-schema/router if
	// we continue to make cloud controller endpoints
	url := urljoiner.Join(h.addr, "staging", "droplets", r.FormValue(":guid"), "upload?async=true")

	h.logger.Infod(map[string]interface{}{
		"url":            url,
		"content-length": r.ContentLength,
	}, "droplet.upload.start")

	uploadReq, err := streamingMultipartRequest(url, r.ContentLength, r.Body, "upload[droplet]", "droplet.tgz")
	if err != nil {
		h.handleError(w, r, err, http.StatusInternalServerError)
		return
	}
	uploadReq.SetBasicAuth(h.username, h.password)

	uploadResp, err := h.client.Do(uploadReq)
	if err != nil {
		h.handleError(w, r, err, http.StatusInternalServerError)
		return
	}

	if uploadResp.StatusCode > 203 {
		respBody, _ := ioutil.ReadAll(uploadResp.Body)
		h.handleError(w, r, fmt.Errorf("Got status: %d\n%s", uploadResp.StatusCode, string(respBody)), uploadResp.StatusCode)
		return
	}

	err = h.poll(uploadResp, closeChan)
	if err != nil {
		h.handleError(w, r, err, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	h.logger.Infod(map[string]interface{}{
		"url":            url,
		"content-length": uploadReq.ContentLength,
	}, "droplet.upload.success")
}

func (h *dropletUploader) handleError(w http.ResponseWriter, r *http.Request, err error, status int) {
	h.logger.Errord(map[string]interface{}{
		"error": err.Error(),
	}, "droplet.upload.failed")
	w.WriteHeader(status)
	w.Write([]byte(err.Error()))
}

// CLOUD CONTROLLER POLLING HELPERS
// The following code is not specific to uploading droplets.
// As soon as we talk to cloud controller in more than one place,
// this code should be extracted into a library

const (
	JOB_QUEUED   = "queued"
	JOB_RUNNING  = "running"
	JOB_FAILED   = "failed"
	JOB_FINISHED = "finished"
)

func (h *dropletUploader) poll(res *http.Response, closeChan <-chan bool) error {
	ticker := time.NewTicker(h.pollingInterval)
	defer ticker.Stop()

	body, err := h.parsePollingResponse(res)
	if err != nil {
		return err
	}

	for {
		select {
		case <-ticker.C:
			switch body.Entity.Status {
			case JOB_QUEUED, JOB_RUNNING:
				res, err := h.client.Get(body.Metadata.Url)
				if err != nil {
					return err
				}
				body, err = h.parsePollingResponse(res)
				if err != nil {
					return err
				}
			case JOB_FINISHED:
				return nil
			case JOB_FAILED:
				return fmt.Errorf("upload job failed")
			default:
				return fmt.Errorf("unknown job status: %s", body.Entity.Status)
			}
		case <-closeChan:
			return fmt.Errorf("upstream request was cancelled")
		}
	}
}

type pollingResponseBody struct {
	Metadata struct {
		Url string
	}
	Entity struct {
		Status string
	}
}

func (h *dropletUploader) parsePollingResponse(res *http.Response) (pollingResponseBody, error) {
	body := pollingResponseBody{}
	err := json.NewDecoder(res.Body).Decode(&body)
	res.Body.Close()
	if err != nil {
		return body, err
	}
	u, err := url.Parse(body.Metadata.Url)
	if err != nil {
		return body, err
	}
	if u.Host == "" {
		body.Metadata.Url = urljoiner.Join(h.addr, body.Metadata.Url)
	}
	return body, nil
}

// FILE UPLOAD HELPERS

func computeMultipartLength(formField string, fileName string) (int64, string, error) {
	multipartBuffer := &bytes.Buffer{}
	multipartWriter := multipart.NewWriter(multipartBuffer)
	_, err := multipartWriter.CreateFormFile(formField, fileName)
	multipartWriter.Close()

	return int64(multipartBuffer.Len()), multipartWriter.Boundary(), err
}

func streamingMultipartRequest(url string, contentLength int64, body io.Reader, formField string, fileName string) (*http.Request, error) {
	pipeReader, pipeWriter := io.Pipe()

	multipartLength, multipartBoundary, err := computeMultipartLength(formField, fileName)
	if err != nil {
		return nil, err
	}

	multipartWriter := multipart.NewWriter(pipeWriter)
	multipartWriter.SetBoundary(multipartBoundary)
	go func() {
		var err error
		defer func() {
			pipeWriter.CloseWithError(err)
		}()

		filePartWriter, err := multipartWriter.CreateFormFile(formField, fileName)
		if err != nil {
			return
		}

		_, err = io.Copy(filePartWriter, body)
		if err != nil {
			return
		}

		err = multipartWriter.Close()
	}()

	uploadReq, err := http.NewRequest("POST", url, pipeReader)
	if err != nil {
		return nil, err
	}
	uploadReq.Header.Set("Content-Type", multipartWriter.FormDataContentType())
	uploadReq.ContentLength = contentLength + multipartLength

	return uploadReq, nil
}
