package upload_build_artifacts

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/cloudfoundry-incubator/file-server/ccclient"
	"github.com/cloudfoundry-incubator/runtime-schema/models"
	"github.com/pivotal-golang/lager"
)

func New(uploader ccclient.Uploader, logger lager.Logger) http.Handler {
	return &buildArtifactUploader{
		uploader: uploader,
		logger:   logger,
	}
}

type buildArtifactUploader struct {
	uploader ccclient.Uploader
	logger   lager.Logger
}

var MissingCCBuildArtifactsUploadUriKeyError = errors.New(fmt.Sprintf("missing %s parameter", models.CcBuildArtifactsUploadUriKey))

func (h *buildArtifactUploader) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestLogger := h.logger.Session("build-artifacts.upload")

	uploadUriParameter := r.URL.Query().Get(models.CcBuildArtifactsUploadUriKey)
	if uploadUriParameter == "" {
		requestLogger.Error("failed", MissingCCBuildArtifactsUploadUriKeyError)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(MissingCCBuildArtifactsUploadUriKeyError.Error()))
		return
	}

	uploadUrl, err := url.Parse(uploadUriParameter)
	if err != nil {
		requestLogger.Error("failed", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	requestLogger.Info("start", lager.Data{
		"upload-url":     uploadUrl,
		"content-length": r.ContentLength,
	})

	uploadResponse, _, err := h.uploader.Upload(uploadUrl, "buildpack_cache.tgz", r)
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

	w.WriteHeader(http.StatusOK)
	requestLogger.Info("success", lager.Data{
		"upload-url":     uploadUrl,
		"content-length": r.ContentLength,
	})
}
