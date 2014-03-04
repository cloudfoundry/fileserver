package upload_droplet

import (
	"encoding/json"
	"fmt"
	"github.com/cloudfoundry/gunk/urljoiner"
	"io"
	"mime/multipart"
	"net/http"
	"time"
)

const (
	JOB_QUEUED   = "queued"
	JOB_RUNNING  = "running"
	JOB_FAILED   = "failed"
	JOB_FINISHED = "finished"
)

func New(addr, username, password string, pollingInterval time.Duration) http.Handler {
	return &dropletUploader{
		addr:            addr,
		username:        username,
		password:        password,
		pollingInterval: pollingInterval,
	}
}

type dropletUploader struct {
	addr            string
	username        string
	password        string
	pollingInterval time.Duration
}

func (h *dropletUploader) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var closeChan <-chan bool

	closeNotifier, ok := w.(http.CloseNotifier)
	if ok {
		closeChan = closeNotifier.CloseNotify()
	}

	pipeReader, pipeWriter := io.Pipe()
	uploadReq, err := http.NewRequest("POST", h.uploadURL(r), pipeReader)
	if err != nil {
		handleError(w, r, err)
		return
	}

	uploadReq.SetBasicAuth(h.username, h.password)

	done := make(chan error, 1)

	writer := multipart.NewWriter(pipeWriter)
	uploadReq.Header.Set("Content-Type", writer.FormDataContentType())
	go func() {
		defer pipeWriter.Close()
		fileWriter, err := writer.CreateFormFile("upload[droplet]", "droplet.tgz")
		if err != nil {
			done <- err
			return
		}
		_, err = io.Copy(fileWriter, r.Body)
		if err != nil {
			done <- err
			return
		}
		done <- writer.Close()
	}()

	uploadResp, err := http.DefaultClient.Do(uploadReq)
	if err != nil {
		handleError(w, r, err)
		return
	}

	if uploadResp.StatusCode > 203 {
		handleError(w, r, fmt.Errorf("Got status: %d", uploadResp.StatusCode))
		return
	}

	err = <-done
	if err != nil {
		handleError(w, r, err)
		return
	}

	err = h.poll(uploadResp, closeChan)
	if err != nil {
		handleError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (h *dropletUploader) uploadURL(r *http.Request) string {
	appGuid := r.FormValue(":guid")
	return urljoiner.Join(h.addr, "staging", "droplets", appGuid, "upload?async=true")
}

func (h *dropletUploader) poll(res *http.Response, closeChan <-chan bool) error {
	ticker := time.NewTicker(h.pollingInterval)
	defer ticker.Stop()

	body, err := parsePollingRequest(res)
	if err != nil {
		return err
	}

	for {
		select {
		case <-ticker.C:
			switch body.Entity.Status {
			case JOB_QUEUED, JOB_RUNNING:
				res, err := http.Get(urljoiner.Join(h.addr, body.Metadata.Url))
				if err != nil {
					return err
				}
				body, err = parsePollingRequest(res)
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

func parsePollingRequest(res *http.Response) (pollingResponseBody, error) {
	body := pollingResponseBody{}
	err := json.NewDecoder(res.Body).Decode(&body)
	res.Body.Close()
	return body, err
}

func handleError(w http.ResponseWriter, r *http.Request, err error) {
	w.WriteHeader(http.StatusInternalServerError)
	w.Write([]byte(err.Error()))
}
