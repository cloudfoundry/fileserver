package upload_build_artifacts

import (
	"net/http"

	"github.com/cloudfoundry-incubator/file-server/handlers/uploader"
	"github.com/cloudfoundry/gunk/urljoiner"
	"github.com/pivotal-golang/lager"
)

func New(addr, username, password string, skipCertVerification bool, logger lager.Logger) http.Handler {
	return &buildArtifactUploader{
		uploader: uploader.New(addr, username, password, skipCertVerification),
		logger:   logger,
	}
}

type buildArtifactUploader struct {
	uploader uploader.Uploader
	logger   lager.Logger
}

func (h *buildArtifactUploader) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// cloud controller buildpack cache upload url
	// TODO: this should be refactored into runtime-schema/router if
	// we continue to make cloud controller endpoints
	url := urljoiner.Join("staging", "buildpack_cache", r.FormValue(":app_guid"), "upload")

	requestLogger := h.logger.Session("build-artifacts.upload")

	requestLogger.Info("start", lager.Data{
		"url":            url,
		"content-length": r.ContentLength,
	})

	uploadResp, err := h.uploader.Upload(url, "buildpack_cache.tgz", r)
	if err != nil {
		handleError(w, r, err, uploadResp, requestLogger)
	}

	w.WriteHeader(http.StatusOK)
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
