package ccclient

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/pivotal-golang/lager"
)

const (
	JOB_QUEUED   = "queued"
	JOB_RUNNING  = "running"
	JOB_FAILED   = "failed"
	JOB_FINISHED = "finished"
)

type poller struct {
	logger lager.Logger

	client       *http.Client
	pollInterval time.Duration
}

func NewPoller(logger lager.Logger, httpClient *http.Client, pollInterval time.Duration) Poller {
	return &poller{
		client:       httpClient,
		pollInterval: pollInterval,
		logger:       logger.Session("poller"),
	}
}

func (p *poller) Poll(fallbackURL *url.URL, res *http.Response, cancelChan <-chan struct{}) error {
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

			req, err := http.NewRequest("GET", pollingUrl.String(), nil)
			if err != nil {
				return err
			}

			completion := make(chan struct{})
			go func() {
				select {
				case <-cancelChan:
					if canceller, ok := p.client.Transport.(requestCanceller); ok {
						canceller.CancelRequest(req)
					} else {
						p.logger.Error("Invalid transport, does not support CancelRequest", nil, lager.Data{"transport": p.client.Transport})
					}
				case <-completion:
				}
			}()

			res, err := p.client.Do(req)
			close(completion)
			if err != nil {
				return err
			}

			body, err = p.parsePollingResponse(res)
			if err != nil {
				return err
			}
		case <-cancelChan:
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
