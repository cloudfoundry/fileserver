package download_build_artifacts

import (
	"fmt"
	steno "github.com/cloudfoundry/gosteno"
	"github.com/cloudfoundry/gunk/http_client"
	"github.com/cloudfoundry/gunk/urljoiner"
	"io"
	"io/ioutil"
	"net/http"
	"strconv"
)

func New(addr, username, password string, skipCertVerification bool, logger *steno.Logger) http.Handler {
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
	logger   *steno.Logger
}

func (h *buildArtifactDownloader) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// cloud controller buildpack cache upload url
	// TODO: this should be refactored into runtime-schema/router if
	// we continue to make cloud controller endpoints
	url := urljoiner.Join(h.addr, "staging", "buildpack_cache", r.FormValue(":app_guid"), "download")
	h.logger.Infod(map[string]interface{}{
		"url":            url,
		"content-length": r.ContentLength,
	}, "build_artifacts.download.start")

	ccRequest, err := http.NewRequest("GET", url, nil)
	if err != nil {
		h.handleError(w, r, err, http.StatusInternalServerError)
		return
	}
	ccRequest.SetBasicAuth(h.username, h.password)

	ccResponse, err := h.client.Do(ccRequest)
	if err != nil {
		h.handleError(w, r, err, http.StatusInternalServerError)
		return
	}

	if ccResponse.StatusCode > 203 {
		respBody, _ := ioutil.ReadAll(ccResponse.Body)
		err = fmt.Errorf("Got status: %d\n%s", ccResponse.StatusCode, string(respBody))
		h.handleError(w, r, err, ccResponse.StatusCode)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Length", strconv.FormatInt(ccResponse.ContentLength, 10))

	bytesWritten, err := io.Copy(w, ccResponse.Body)

	if err != nil {
		h.logger.Errord(map[string]interface{}{
			"error": fmt.Sprintf("Failed to copy response bytes: %s", err),
		}, "build_artifacts.download.failed")
		return
	}

	if ccResponse.ContentLength != bytesWritten {
		h.logger.Errord(map[string]interface{}{
			"error": fmt.Sprintf("Content-Length does not match response body length: \nContent-Length:%d\nBytes Transferred:%d", ccResponse.ContentLength, bytesWritten),
		}, "build_artifacts.download.failed")
		return
	}

	h.logger.Infod(map[string]interface{}{
		"url":            url,
		"content-length": ccResponse.ContentLength,
	}, "build_artifacts.download.success")

}

func (h *buildArtifactDownloader) handleError(w http.ResponseWriter, r *http.Request, err error, status int) {
	h.logger.Errord(map[string]interface{}{
		"error": err.Error(),
	}, "build_artifacts.download.failed")
	w.WriteHeader(status)
	w.Write([]byte(err.Error()))
}
