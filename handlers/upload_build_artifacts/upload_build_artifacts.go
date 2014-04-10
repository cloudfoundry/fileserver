package upload_build_artifacts

import (
	"fmt"
	"github.com/cloudfoundry-incubator/file-server/multipart"
	steno "github.com/cloudfoundry/gosteno"
	"github.com/cloudfoundry/gunk/http_client"
	"github.com/cloudfoundry/gunk/urljoiner"
	"io/ioutil"
	"net/http"
)

func New(addr, username, password string, skipCertVerification bool, logger *steno.Logger) http.Handler {
	return &buildArtifactUploader{
		addr:     addr,
		username: username,
		password: password,
		client:   http_client.New(skipCertVerification),
		logger:   logger,
	}
}

type buildArtifactUploader struct {
	addr     string
	username string
	password string
	client   *http.Client
	logger   *steno.Logger
}

func (h *buildArtifactUploader) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	if r.ContentLength <= 0 {
		h.handleError(w, r, fmt.Errorf("Mssing Content Length"), http.StatusLengthRequired)
		return
	}

	// cloud controller buildpack cache upload url
	// TODO: this should be refactored into runtime-schema/router if
	// we continue to make cloud controller endpoints
	url := urljoiner.Join(h.addr, "staging", "buildpack_cache", r.FormValue(":app_guid"), "upload")

	h.logger.Infod(map[string]interface{}{
		"url":            url,
		"content-length": r.ContentLength,
	}, "build_artifacts.upload.start")

	uploadReq, err := multipart.NewRequestFromReader(url, r.ContentLength, r.Body, "upload[droplet]", "buildpack_cache.tgz")
	if err != nil {
		h.handleError(w, r, err, http.StatusInternalServerError)
		return
	}
	uploadReq.SetBasicAuth(h.username, h.password)

	uploadResp, err := h.client.Do(uploadReq)
	if err != nil {
		h.handleError(w, r, err, http.StatusInternalServerError)
		return
	}

	if uploadResp.StatusCode > 203 {
		respBody, _ := ioutil.ReadAll(uploadResp.Body)
		h.handleError(w, r, fmt.Errorf("Got status: %d\n%s", uploadResp.StatusCode, string(respBody)), uploadResp.StatusCode)
		return
	}

	w.WriteHeader(http.StatusOK)
	h.logger.Infod(map[string]interface{}{
		"url":            url,
		"content-length": uploadReq.ContentLength,
	}, "build_artifacts.upload.success")
}

func (h *buildArtifactUploader) handleError(w http.ResponseWriter, r *http.Request, err error, status int) {
	h.logger.Errord(map[string]interface{}{
		"error": err.Error(),
	}, "build_artifacts.upload.failed")
	w.WriteHeader(status)
	w.Write([]byte(err.Error()))
}
