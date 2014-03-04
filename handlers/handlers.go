package handlers

import (
	"github.com/cloudfoundry-incubator/file-server/config"
	"github.com/cloudfoundry-incubator/file-server/handlers/static"
	"github.com/cloudfoundry-incubator/file-server/handlers/upload_droplet"
	"github.com/cloudfoundry-incubator/runtime-schema/router"
)

func New(c *config.Config) router.Handlers {
	return router.Handlers{
		"static":         static.New(c.StaticDirectory),
		"upload_droplet": upload_droplet.New(c.CCAddress, c.CCUsername, c.CCPassword, c.CCJobPollingInterval),
	}
}
