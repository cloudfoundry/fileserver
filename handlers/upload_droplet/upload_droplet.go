package upload_droplet

import (
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/cloudfoundry-incubator/file-server/handlers/uploader"
	"github.com/cloudfoundry-incubator/runtime-schema/models"
	"github.com/pivotal-golang/lager"
)

func New(addr *url.URL, pollingInterval time.Duration, skipCertVerification bool, logger lager.Logger) http.Handler {
	return &dropletUploader{
		uploader:        uploader.New(addr, skipCertVerification),
		pollingInterval: pollingInterval,
		logger:          logger,
	}
}

type dropletUploader struct {
	pollingInterval time.Duration
	uploader        uploader.Uploader
	logger          lager.Logger
}

func (h *dropletUploader) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestLogger := h.logger.Session("droplet.upload")
	// cloud controller droplet upload url
	// TODO: this should be refactored into runtime-schema/router if
	// we continue to make cloud controller endpoints
	uploadUri := r.URL.Query().Get(models.CcDropletUploadUriKey)
	if uploadUri == "" {
		err := errors.New("missing " + models.CcDropletUploadUriKey + " parameter")
		handleError(w, r, err, nil, requestLogger)
		return
	}

	u, err := url.Parse(uploadUri)
	//u := urljoiner.Join("staging", "droplets", r.FormValue(":guid"), "upload?async=true")
	if err != nil {
		handleError(w, r, err, nil, requestLogger)
		return
	}

	if u.RawQuery == "" {
		u.RawQuery = "async=true"
	} else {
		v := u.Query()
		v.Set("async", "true")
		u.RawQuery = v.Encode()
	}

	requestLogger.Info("start", lager.Data{
		"url":            u,
		"content-length": r.ContentLength,
	})

	uploadStart := time.Now()
	uploadResp, pollUrl, err := h.uploader.Upload(u, "droplet.tgz", r)
	if err != nil {
		handleError(w, r, err, uploadResp, requestLogger)
		return
	}
	uploadEnd := time.Now()

	var closeChan <-chan bool
	closeNotifier, ok := w.(http.CloseNotifier)
	if ok {
		closeChan = closeNotifier.CloseNotify()
	}

	err = h.uploader.Poll(pollUrl, uploadResp, closeChan, h.pollingInterval)
	if err != nil {
		handleError(w, r, err, nil, requestLogger)
		return
	}
	pollEnd := time.Now()

	w.WriteHeader(http.StatusCreated)
	requestLogger.Info("success", lager.Data{
		"url":            u,
		"content-length": r.ContentLength,
		"upload-time":    uploadEnd.Sub(uploadStart).String(),
		"poll-time":      pollEnd.Sub(uploadEnd).String(),
	})
}

func handleError(w http.ResponseWriter, r *http.Request, err error, resp *http.Response, logger lager.Logger) {
	status := http.StatusInternalServerError
	if resp != nil {
		status = resp.StatusCode
	}

	logger.Error("failed", err)
	w.WriteHeader(status)
	w.Write([]byte(err.Error()))
}
