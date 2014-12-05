package handlers

import (
	"net/http"

	"github.com/cloudfoundry-incubator/file-server/ccclient"
	"github.com/cloudfoundry-incubator/file-server/handlers/static"
	"github.com/cloudfoundry-incubator/file-server/handlers/upload_build_artifacts"
	"github.com/cloudfoundry-incubator/file-server/handlers/upload_droplet"
	"github.com/cloudfoundry-incubator/runtime-schema/routes"
	"github.com/pivotal-golang/lager"
	"github.com/tedsuo/rata"
)

func New(staticDirectory string, uploader ccclient.Uploader, poller ccclient.Poller, logger lager.Logger) (http.Handler, error) {
	staticRoute, err := routes.FileServerRoutes.CreatePathForRoute(routes.FS_STATIC, nil)
	if err != nil {
		return nil, err
	}

	return rata.NewRouter(routes.FileServerRoutes, rata.Handlers{
		routes.FS_STATIC:                 static.New(staticDirectory, staticRoute, logger),
		routes.FS_UPLOAD_DROPLET:         upload_droplet.New(uploader, poller, logger),
		routes.FS_UPLOAD_BUILD_ARTIFACTS: upload_build_artifacts.New(uploader, logger),
	})
}
