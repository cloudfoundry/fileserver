package handlers

import (
	"time"

	"github.com/cloudfoundry-incubator/file-server/handlers/download_build_artifacts"
	"github.com/cloudfoundry-incubator/file-server/handlers/static"
	"github.com/cloudfoundry-incubator/file-server/handlers/upload_build_artifacts"
	"github.com/cloudfoundry-incubator/file-server/handlers/upload_droplet"
	"github.com/cloudfoundry-incubator/runtime-schema/router"
	steno "github.com/cloudfoundry/gosteno"
)

type Config struct {
	StaticDirectory   string
	HeartbeatInterval time.Duration

	CCAddress            string
	CCUsername           string
	CCPassword           string
	CCJobPollingInterval time.Duration

	SkipCertVerify bool
}

func New(c Config, logger *steno.Logger) router.Handlers {
	staticRoute, _ := router.NewFileServerRoutes().RouteForHandler(router.FS_STATIC)

	return router.Handlers{
		router.FS_STATIC:                   static.New(c.StaticDirectory, staticRoute.Path),
		router.FS_UPLOAD_DROPLET:           upload_droplet.New(c.CCAddress, c.CCUsername, c.CCPassword, c.CCJobPollingInterval, c.SkipCertVerify, logger),
		router.FS_UPLOAD_BUILD_ARTIFACTS:   upload_build_artifacts.New(c.CCAddress, c.CCUsername, c.CCPassword, c.SkipCertVerify, logger),
		router.FS_DOWNLOAD_BUILD_ARTIFACTS: download_build_artifacts.New(c.CCAddress, c.CCUsername, c.CCPassword, c.SkipCertVerify, logger),
	}
}
