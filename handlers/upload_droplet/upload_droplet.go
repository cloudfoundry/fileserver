package upload_droplet

import (
	"net/http"
	"time"

	"github.com/cloudfoundry-incubator/file-server/handlers/uploader"
	"github.com/cloudfoundry/gunk/urljoiner"
	"github.com/pivotal-golang/lager"
)

func New(addr, username, password string, pollingInterval time.Duration, skipCertVerification bool, logger lager.Logger) http.Handler {
	return &dropletUploader{
		uploader:        uploader.New(addr, username, password, skipCertVerification),
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
	var closeChan <-chan bool
	closeNotifier, ok := w.(http.CloseNotifier)
	if ok {
		closeChan = closeNotifier.CloseNotify()
	}

	// cloud controller droplet upload url
	// TODO: this should be refactored into runtime-schema/router if
	// we continue to make cloud controller endpoints
	url := urljoiner.Join("staging", "droplets", r.FormValue(":guid"), "upload?async=true")

	requestLogger := h.logger.Session("droplet.upload")

	requestLogger.Info("start", lager.Data{
		"url":            url,
		"content-length": r.ContentLength,
	})

	uploadResp, err := h.uploader.Upload(url, "droplet.tgz", r)
	if err != nil {
		handleError(w, r, err, uploadResp, requestLogger)
		return
	}

	err = h.uploader.Poll(uploadResp, closeChan, h.pollingInterval)
	if err != nil {
		handleError(w, r, err, nil, requestLogger)
		return
	}

	w.WriteHeader(http.StatusCreated)
	requestLogger.Info("success", lager.Data{
		"url":            url,
		"content-length": r.ContentLength,
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
