package upload_droplet

import (
	"net/http"
	"time"

	"github.com/cloudfoundry-incubator/file-server/handlers/uploader"
	steno "github.com/cloudfoundry/gosteno"
	"github.com/cloudfoundry/gunk/urljoiner"
)

func New(addr, username, password string, pollingInterval time.Duration, skipCertVerification bool, logger *steno.Logger) http.Handler {
	return &dropletUploader{
		uploader:        uploader.New(addr, username, password, skipCertVerification),
		pollingInterval: pollingInterval,
		logger:          logger,
	}
}

type dropletUploader struct {
	pollingInterval time.Duration
	uploader        uploader.Uploader
	logger          *steno.Logger
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

	h.logger.Infod(map[string]interface{}{
		"url":            url,
		"content-length": r.ContentLength,
	}, "droplet.upload.start")

	uploadResp, err := h.uploader.Upload(url, "droplet.tgz", r)
	if err != nil {
		h.handleError(w, r, err, uploadResp)
		return
	}

	err = h.uploader.Poll(uploadResp, closeChan, h.pollingInterval)
	if err != nil {
		h.handleError(w, r, err, nil)
		return
	}

	w.WriteHeader(http.StatusCreated)
	h.logger.Infod(map[string]interface{}{
		"url":            url,
		"content-length": r.ContentLength,
	}, "droplet.upload.success")
}

func (h *dropletUploader) handleError(w http.ResponseWriter, r *http.Request, err error, resp *http.Response) {
	status := http.StatusInternalServerError
	if resp != nil {
		status = resp.StatusCode
	}

	h.logger.Errord(map[string]interface{}{
		"error": err.Error(),
	}, "droplet.upload.failed")
	w.WriteHeader(status)
	w.Write([]byte(err.Error()))
}
