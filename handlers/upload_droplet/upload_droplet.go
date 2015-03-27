package upload_droplet

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/cloudfoundry-incubator/file-server/ccclient"
	"github.com/cloudfoundry-incubator/runtime-schema/models"
	"github.com/pivotal-golang/lager"
)

func New(uploader ccclient.Uploader, poller ccclient.Poller, logger lager.Logger) http.Handler {
	return &dropletUploader{
		uploader: uploader,
		poller:   poller,
		logger:   logger,
	}
}

type dropletUploader struct {
	uploader ccclient.Uploader
	poller   ccclient.Poller
	logger   lager.Logger
}

var MissingCCDropletUploadUriKeyError = errors.New(fmt.Sprintf("missing %s parameter", models.CcDropletUploadUriKey))

func (h *dropletUploader) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestLogger := h.logger.Session("droplet.upload")

	uploadUriParameter := r.URL.Query().Get(models.CcDropletUploadUriKey)
	if uploadUriParameter == "" {
		requestLogger.Error("failed", MissingCCDropletUploadUriKeyError)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(MissingCCDropletUploadUriKeyError.Error()))
		return
	}

	uploadUrl, err := url.Parse(uploadUriParameter)
	if err != nil {
		requestLogger.Error("failed: Invalid upload uri", err)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}

	timeout := 5 * time.Minute
	timeoutParameter := r.URL.Query().Get(models.CcTimeoutKey)
	if timeoutParameter != "" {
		t, err := strconv.Atoi(timeoutParameter)
		if err != nil {
			requestLogger.Error("failed: Invalid timeout", err)
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(err.Error()))
			return
		}
		timeout = time.Duration(t) * time.Second
	}

	query := uploadUrl.Query()
	query.Set("async", "true")
	uploadUrl.RawQuery = query.Encode()

	requestLogger.Info("start", lager.Data{
		"upload-url":     uploadUrl,
		"content-length": r.ContentLength,
	})

	cancelChan := make(chan struct{})
	var writerClosed <-chan bool
	closeNotifier, ok := w.(http.CloseNotifier)
	if ok {
		writerClosed = closeNotifier.CloseNotify()
	}

	done := make(chan struct{})
	go func() {
		timer := time.NewTimer(timeout)
		select {
		case <-writerClosed:
			close(cancelChan)
		case <-timer.C:
			close(cancelChan)
		case <-done:
		}
		timer.Stop()
	}()
	defer close(done)

	uploadStart := time.Now()
	uploadResponse, err := h.uploader.Upload(uploadUrl, "droplet.tgz", r, cancelChan)
	if err != nil {
		requestLogger.Error("failed", err)
		if uploadResponse == nil {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(uploadResponse.StatusCode)
		}
		w.Write([]byte(err.Error()))
		return
	}
	uploadEnd := time.Now()

	err = h.poller.Poll(uploadUrl, uploadResponse, cancelChan)
	if err != nil {
		requestLogger.Error("failed", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	pollEnd := time.Now()

	w.WriteHeader(http.StatusCreated)
	requestLogger.Info("success", lager.Data{
		"upload-url":     uploadUrl,
		"content-length": r.ContentLength,
		"upload-time":    uploadEnd.Sub(uploadStart).String(),
		"poll-time":      pollEnd.Sub(uploadEnd).String(),
	})
}
