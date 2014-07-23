package download_build_artifacts

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strconv"

	"github.com/cloudfoundry/gunk/http_client"
	"github.com/cloudfoundry/gunk/urljoiner"
	"github.com/pivotal-golang/lager"
)

func New(addr, username, password string, skipCertVerification bool, logger lager.Logger) http.Handler {
	return &buildArtifactDownloader{
		addr:     addr,
		username: username,
		password: password,
		client:   http_client.New(skipCertVerification),
		logger:   logger,
	}
}

type buildArtifactDownloader struct {
	addr     string
	username string
	password string
	client   *http.Client
	logger   lager.Logger
}

func (h *buildArtifactDownloader) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// cloud controller buildpack cache upload url
	// TODO: this should be refactored into runtime-schema/router if
	// we continue to make cloud controller endpoints
	requestLogger := h.logger.Session("build-artifacts.download")
	url := urljoiner.Join(h.addr, "staging", "buildpack_cache", r.FormValue(":app_guid"), "download")
	requestLogger.Info("start", lager.Data{"url": url})

	ccRequest, err := http.NewRequest("GET", url, nil)
	if err != nil {
		handleError(w, r, err, http.StatusInternalServerError, requestLogger)
		return
	}
	ccRequest.SetBasicAuth(h.username, h.password)

	ccResponse, err := h.client.Do(ccRequest)
	if err != nil {
		handleError(w, r, err, http.StatusInternalServerError, requestLogger)
		return
	}

	if ccResponse.StatusCode > 203 {
		respBody, _ := ioutil.ReadAll(ccResponse.Body)
		requestLogger.Error("cc-status-code-failed", nil, lager.Data{
			"status-code": ccResponse.StatusCode,
			"body":        string(respBody),
		})
		w.WriteHeader(ccResponse.StatusCode)
		w.Write([]byte(fmt.Sprintf("Got status: %d\n%s", ccResponse.StatusCode, string(respBody))))

		return
	}

	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Length", strconv.FormatInt(ccResponse.ContentLength, 10))

	bytesWritten, err := io.Copy(w, ccResponse.Body)

	if err != nil {
		requestLogger.Error("copying-bytes-failed", err)
		return
	}

	if ccResponse.ContentLength != bytesWritten {
		requestLogger.Error("content-length-and-body-match-failed", nil, lager.Data{
			"content-length":    ccResponse.ContentLength,
			"bytes-transferred": bytesWritten,
		})
		return
	}

	requestLogger.Info("success", lager.Data{
		"url":            url,
		"content-length": ccResponse.ContentLength,
	})
}

func handleError(w http.ResponseWriter, r *http.Request, err error, status int, logger lager.Logger) {
	logger.Error("failed", err)
	w.WriteHeader(status)
	w.Write([]byte(err.Error()))
}
