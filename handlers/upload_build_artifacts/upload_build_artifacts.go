package upload_build_artifacts

import (
	"net/http"

	"github.com/cloudfoundry-incubator/file-server/handlers/uploader"
	steno "github.com/cloudfoundry/gosteno"
	"github.com/cloudfoundry/gunk/urljoiner"
)

func New(addr, username, password string, skipCertVerification bool, logger *steno.Logger) http.Handler {
	return &buildArtifactUploader{
		uploader: uploader.New(addr, username, password, skipCertVerification),
		logger:   logger,
	}
}

type buildArtifactUploader struct {
	uploader uploader.Uploader
	logger   *steno.Logger
}

func (h *buildArtifactUploader) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// cloud controller buildpack cache upload url
	// TODO: this should be refactored into runtime-schema/router if
	// we continue to make cloud controller endpoints
	url := urljoiner.Join("staging", "buildpack_cache", r.FormValue(":app_guid"), "upload")

	h.logger.Infod(map[string]interface{}{
		"url":            url,
		"content-length": r.ContentLength,
	}, "build_artifacts.upload.start")

	uploadResp, err := h.uploader.Upload(url, "buildpack_cache.tgz", r)
	if err != nil {
		h.handleError(w, r, err, uploadResp)
	}

	w.WriteHeader(http.StatusOK)
	h.logger.Infod(map[string]interface{}{
		"url":            url,
		"content-length": r.ContentLength,
	}, "build_artifacts.upload.success")
}

func (h *buildArtifactUploader) handleError(w http.ResponseWriter, r *http.Request, err error, resp *http.Response) {
	status := http.StatusInternalServerError
	if resp != nil {
		status = resp.StatusCode
	}

	h.logger.Errord(map[string]interface{}{
		"error": err.Error(),
	}, "build_artifacts.upload.failed")
	w.WriteHeader(status)
	w.Write([]byte(err.Error()))
}
