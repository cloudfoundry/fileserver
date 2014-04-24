package uploader

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"

	"github.com/cloudfoundry-incubator/file-server/multipart"
	"github.com/cloudfoundry/gunk/http_client"
	"github.com/cloudfoundry/gunk/urljoiner"
)

type Uploader interface {
	Upload(url, filename string, r *http.Request) (*http.Response, error)
	Poll(res *http.Response, closeChan <-chan bool, interval time.Duration) error
}

type httpUploader struct {
	baseUrl  string
	username string
	password string
	client   *http.Client
}

func New(baseUrl, username, password string, skipCertVerification bool) Uploader {
	return &httpUploader{
		baseUrl:  baseUrl,
		username: username,
		password: password,
		client:   http_client.New(skipCertVerification),
	}
}

func (u *httpUploader) Upload(url, filename string, r *http.Request) (*http.Response, error) {
	defer r.Body.Close()
	if r.ContentLength <= 0 {
		return &http.Response{StatusCode: http.StatusLengthRequired}, fmt.Errorf("Missing Content Length")
	}

	uploadReq, err := multipart.NewRequestFromReader(urljoiner.Join(u.baseUrl, url), r.ContentLength, r.Body, "upload[droplet]", filename)
	if err != nil {
		return nil, err
	}
	uploadReq.SetBasicAuth(u.username, u.password)

	uploadResp, err := u.client.Do(uploadReq)
	if err != nil {
		return nil, err
	}

	if uploadResp.StatusCode > 203 {
		respBody, _ := ioutil.ReadAll(uploadResp.Body)
		return uploadResp, fmt.Errorf("Got status: %d\n%s", uploadResp.StatusCode, string(respBody))
	}

	return uploadResp, nil
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

func (u *httpUploader) Poll(res *http.Response, closeChan <-chan bool, interval time.Duration) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	body, err := u.parsePollingResponse(res)
	if err != nil {
		return err
	}

	for {
		select {
		case <-ticker.C:
			switch body.Entity.Status {
			case JOB_QUEUED, JOB_RUNNING:
				res, err := u.client.Get(body.Metadata.Url)
				if err != nil {
					return err
				}
				body, err = u.parsePollingResponse(res)
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

func (u *httpUploader) parsePollingResponse(res *http.Response) (pollingResponseBody, error) {
	body := pollingResponseBody{}
	err := json.NewDecoder(res.Body).Decode(&body)
	res.Body.Close()
	if err != nil {
		return body, err
	}
	pollingUrl, err := url.Parse(body.Metadata.Url)
	if err != nil {
		return body, err
	}
	if pollingUrl.Host == "" {
		body.Metadata.Url = urljoiner.Join(u.baseUrl, body.Metadata.Url)
	}
	return body, nil
}

type pollingResponseBody struct {
	Metadata struct {
		Url string
	}
	Entity struct {
		Status string
	}
}
