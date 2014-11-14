package test_helpers

import "net/http"

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
