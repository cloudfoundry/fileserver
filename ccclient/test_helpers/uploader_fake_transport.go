package test_helpers

import (
	"errors"
	"net/http"
	"sync"
	"time"
)

type RespErrorPair struct {
	Resp *http.Response
	Err  error
}

type fakeRoundTripper struct {
	reqChan chan *http.Request
	respMap map[string]RespErrorPair
}

func (f *fakeRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	f.reqChan <- r
	pair := f.respMap[r.URL.Host]
	return pair.Resp, pair.Err
}

func NewFakeRoundTripper(reqChan chan *http.Request, responses map[string]RespErrorPair) *fakeRoundTripper {
	return &fakeRoundTripper{reqChan, responses}
}

type fakeTimedRoundTripper struct {
	duration time.Duration
	mu       sync.Mutex
	requests map[*http.Request]chan struct{}
	respMap  map[string]RespErrorPair
}

func (f *fakeTimedRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	barrier := make(chan struct{})
	f.mu.Lock()
	f.requests[r] = barrier

	response := f.respMap[r.URL.Host]
	f.mu.Unlock()

	if response.Err == nil {
		select {
		case <-time.After(f.duration):
		case <-barrier:
			response.Resp = nil
			response.Err = errors.New("cancelled")
		}
	}

	f.mu.Lock()
	delete(f.requests, r)
	f.mu.Unlock()

	return response.Resp, response.Err
}

func (f *fakeTimedRoundTripper) CancelRequest(req *http.Request) {
	f.mu.Lock()
	barrier := f.requests[req]
	f.mu.Unlock()

	if barrier != nil {
		close(barrier)
	}
}

func NewFakeTimedRoundTripper(duration time.Duration, respMap map[string]RespErrorPair) *fakeTimedRoundTripper {
	return &fakeTimedRoundTripper{
		duration: duration,
		requests: make(map[*http.Request]chan struct{}),
		respMap:  respMap,
	}
}
