package upload_build_artifacts

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/cloudfoundry-incubator/file-server/handlers/uploader"
	"github.com/cloudfoundry-incubator/runtime-schema/models"
	"github.com/pivotal-golang/lager"
)

func New(addr *url.URL, skipCertVerification bool, logger lager.Logger) http.Handler {
	return &buildArtifactUploader{
		uploader: uploader.New(addr, skipCertVerification),
		logger:   logger,
	}
}

type buildArtifactUploader struct {
	uploader uploader.Uploader
	logger   lager.Logger
}

func (h *buildArtifactUploader) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestLogger := h.logger.Session("build-artifacts.upload")

	// cloud controller buildpack cache upload url
	// TODO: this should be refactored into runtime-schema/router if
	// we continue to make cloud controller endpoints
	uploadUri := r.URL.Query().Get(models.CcBuildArtifactsUploadUriKey)
	if uploadUri == "" {
		err := errors.New("missing " + models.CcBuildArtifactsUploadUriKey + " parameter")
		handleError(w, r, err, nil, requestLogger)
		return
	}

	url, err := url.Parse(uploadUri)
	// url.Path = urljoiner.Join("staging", "buildpack_cache", r.FormValue(":app_guid"), "upload")
	if err != nil {
		handleError(w, r, err, nil, requestLogger)
		return
	}

	requestLogger.Info("start", lager.Data{
		"url":            url,
		"content-length": r.ContentLength,
	})

	uploadResp, _, err := h.uploader.Upload(url, "buildpack_cache.tgz", r)
	if err != nil {
		handleError(w, r, err, uploadResp, requestLogger)
		return
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
