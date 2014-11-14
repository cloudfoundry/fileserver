package ccclient

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

const (
	JOB_QUEUED   = "queued"
	JOB_RUNNING  = "running"
	JOB_FAILED   = "failed"
	JOB_FINISHED = "finished"
)

type poller struct {
	client       *http.Client
	pollInterval time.Duration
}

func NewPoller(transport http.RoundTripper, pollInterval time.Duration) Poller {
	return &poller{
		client: &http.Client{
			Transport: transport,
		},
		pollInterval: pollInterval,
	}
}

func (p *poller) Poll(fallbackURL *url.URL, res *http.Response, closeChan <-chan bool) error {
	body, err := p.parsePollingResponse(res)
	if err != nil {
		return err
	}

	ticker := time.NewTicker(p.pollInterval)
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
			res, err := p.client.Get(pollingUrl.String())
			if err != nil {
				return err
			}

			body, err = p.parsePollingResponse(res)
			if err != nil {
				return err
			}
		case <-closeChan:
			return fmt.Errorf("upstream request was cancelled")
		}
	}
}

func (p *poller) parsePollingResponse(res *http.Response) (pollingResponseBody, error) {
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
