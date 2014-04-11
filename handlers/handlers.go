package handlers

import (
	"github.com/cloudfoundry-incubator/file-server/config"
	"github.com/cloudfoundry-incubator/file-server/handlers/download_build_artifacts"
	"github.com/cloudfoundry-incubator/file-server/handlers/static"
	"github.com/cloudfoundry-incubator/file-server/handlers/upload_build_artifacts"
	"github.com/cloudfoundry-incubator/file-server/handlers/upload_droplet"
	"github.com/cloudfoundry-incubator/runtime-schema/router"
	steno "github.com/cloudfoundry/gosteno"
)

func New(c *config.Config, logger *steno.Logger) router.Handlers {
	return router.Handlers{
		router.FS_STATIC:                   static.New(c.StaticDirectory),
		router.FS_UPLOAD_DROPLET:           upload_droplet.New(c.CCAddress, c.CCUsername, c.CCPassword, c.CCJobPollingInterval, c.SkipCertVerify, logger),
		router.FS_UPLOAD_BUILD_ARTIFACTS:   upload_build_artifacts.New(c.CCAddress, c.CCUsername, c.CCPassword, c.SkipCertVerify, logger),
		router.FS_DOWNLOAD_BUILD_ARTIFACTS: download_build_artifacts.New(c.CCAddress, c.CCUsername, c.CCPassword, c.SkipCertVerify, logger),
	}
}
