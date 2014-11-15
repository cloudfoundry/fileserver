package ccclient

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"

	"github.com/cloudfoundry-incubator/cf-lager"
	"github.com/pivotal-golang/lager"
)

type uploader struct {
	baseUrl *url.URL
	client  *http.Client
	logger  lager.Logger
}

func NewUploader(baseUrl *url.URL, transport http.RoundTripper) Uploader {
	return &uploader{
		baseUrl: baseUrl,
		client: &http.Client{
			Transport: transport,
		},
		logger: cf_lager.New("Uploader"),
	}
}

const contentMD5Header = "Content-MD5"

func (u *uploader) Upload(primaryUrl *url.URL, filename string, r *http.Request, cancelChan <-chan struct{}) (*http.Response, *url.URL, error) {
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

	rsp, err := u.do(uploadReq, cancelChan)
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

	rsp, err = u.do(uploadReq, cancelChan)
	return rsp, uploadReq.URL, err
}

func (u *uploader) do(req *http.Request, cancelChan <-chan struct{}) (*http.Response, error) {
	completion := make(chan struct{})
	defer close(completion)

	go func() {
		select {
		case <-cancelChan:
			if canceller, ok := u.client.Transport.(requestCanceller); ok {
				canceller.CancelRequest(req)
			} else {
				u.logger.Error("Invalid transport, does not support CancelRequest", nil, lager.Data{"transport": u.client.Transport})
			}
		case <-completion:
		}
	}()

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
