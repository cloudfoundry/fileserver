package handlers

import (
	"net/url"
	"time"

	"github.com/cloudfoundry-incubator/file-server/handlers/static"
	"github.com/cloudfoundry-incubator/file-server/handlers/upload_build_artifacts"
	"github.com/cloudfoundry-incubator/file-server/handlers/upload_droplet"
	"github.com/cloudfoundry-incubator/runtime-schema/router"
	"github.com/pivotal-golang/lager"
)

type Config struct {
	StaticDirectory string

	CCAddress            string
	CCUsername           string
	CCPassword           string
	CCJobPollingInterval time.Duration

	SkipCertVerify bool
}

func New(c Config, logger lager.Logger) router.Handlers {
	staticRoute, _ := router.NewFileServerRoutes().RouteForHandler(router.FS_STATIC)

	u, err := url.Parse(c.CCAddress)
	if err != nil {
		logger.Fatal("cc-address-parse-failure", err)
	}

	u.User = url.UserPassword(c.CCUsername, c.CCPassword)
	return router.Handlers{
		router.FS_STATIC:                 static.New(c.StaticDirectory, staticRoute.Path, logger),
		router.FS_UPLOAD_DROPLET:         upload_droplet.New(u, c.CCJobPollingInterval, c.SkipCertVerify, logger),
		router.FS_UPLOAD_BUILD_ARTIFACTS: upload_build_artifacts.New(u, c.SkipCertVerify, logger),
	}
}
