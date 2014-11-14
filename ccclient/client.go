package ccclient

import (
	"net/http"
	"net/url"
)

type Uploader interface {
	Upload(uploadURL *url.URL, filename string, r *http.Request, cancelChan chan struct{}) (*http.Response, *url.URL, error)
}

type Poller interface {
	Poll(fallbackURL *url.URL, res *http.Response, closeChan <-chan bool) error
}
