package uploader

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"time"
)

type Uploader interface {
	Upload(u *url.URL, filename string, r *http.Request) (*http.Response, *url.URL, error)
	Poll(u *url.URL, res *http.Response, closeChan <-chan bool, interval time.Duration) error
}

type httpUploader struct {
	baseUrl *url.URL
	client  *http.Client
}

func New(baseUrl *url.URL, transport http.RoundTripper) Uploader {
	return &httpUploader{
		baseUrl: baseUrl,
		client: &http.Client{
			Transport: transport,
		},
	}
}

const contentMD5Header = "Content-MD5"

func (u *httpUploader) Upload(primaryUrl *url.URL, filename string, r *http.Request) (*http.Response, *url.URL, error) {
	if r.ContentLength <= 0 {
		return &http.Response{StatusCode: http.StatusLengthRequired}, nil, fmt.Errorf("Missing Content Length")
	}
	defer r.Body.Close()

	uploadReq, err := newMultipartRequestFromReader(r.ContentLength, r.Body, filename)
	if err != nil {
		return nil, nil, err
	}

	uploadReq.Header.Set(contentMD5Header, r.Header.Get(contentMD5Header))

	// try the fast path
	uploadReq.URL = primaryUrl
	if primaryUrl.User != nil {
		if password, set := primaryUrl.User.Password(); set {
			uploadReq.SetBasicAuth(primaryUrl.User.Username(), password)
		}
	}

	rsp, err := u.do(uploadReq)
	if err == nil {
		return rsp, uploadReq.URL, err
	}

	// not a connect (dial) error
	var nestedErr error = err
	if urlErr, ok := err.(*url.Error); ok {
		nestedErr = urlErr.Err
	}
	if netErr, ok := nestedErr.(*net.OpError); !ok || netErr.Op != "dial" {
		return rsp, nil, err
	}

	// try the slow path
	uploadReq, err = newMultipartRequestFromReader(r.ContentLength, r.Body, filename)
	if err != nil {
		return nil, nil, err
	}

	uploadReq.Header.Set(contentMD5Header, r.Header.Get(contentMD5Header))

	uploadReq.URL.Scheme = u.baseUrl.Scheme
	uploadReq.URL.Host = u.baseUrl.Host
	if u.baseUrl.User != nil {
		if password, set := u.baseUrl.User.Password(); set {
			uploadReq.URL.User = u.baseUrl.User
			uploadReq.SetBasicAuth(u.baseUrl.User.Username(), password)
		}
	}
	uploadReq.URL.Path = primaryUrl.Path
	uploadReq.URL.RawQuery = primaryUrl.RawQuery

	rsp, err = u.do(uploadReq)
	return rsp, uploadReq.URL, err
}

func (u *httpUploader) do(req *http.Request) (*http.Response, error) {
	rsp, err := u.client.Do(req)
	req.Body.Close()
	if err != nil {
		return nil, err
	}

	switch rsp.StatusCode {
	case http.StatusOK, http.StatusCreated:
		return rsp, nil
	}

	respBody, _ := ioutil.ReadAll(rsp.Body)
	rsp.Body.Close()
	return rsp, fmt.Errorf("status code: %d\n%s", rsp.StatusCode, string(respBody))
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

func (u *httpUploader) Poll(fallbackURL *url.URL, res *http.Response, closeChan <-chan bool, interval time.Duration) error {
	body, err := u.parsePollingResponse(res)
	if err != nil {
		return err
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		switch body.Entity.Status {
		case JOB_QUEUED, JOB_RUNNING:
		case JOB_FINISHED:
			return nil
		case JOB_FAILED:
			return fmt.Errorf("upload job failed")
		default:
			return fmt.Errorf("unknown job status: %s", body.Entity.Status)
		}

		select {
		case <-ticker.C:
			pollingUrl, err := url.Parse(body.Metadata.Url)
			if err != nil {
				return err
			}

			if pollingUrl.Host == "" {
				pollingUrl.Scheme = fallbackURL.Scheme
				pollingUrl.Host = fallbackURL.Host
			}
			res, err := u.client.Get(pollingUrl.String())
			if err != nil {
				return err
			}

			body, err = u.parsePollingResponse(res)
			if err != nil {
				return err
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
	return body, err
}

type pollingResponseBody struct {
	Metadata struct {
		Url string
	}
	Entity struct {
		Status string
	}
}
