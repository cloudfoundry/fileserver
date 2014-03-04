package handlers

import (
	"github.com/cloudfoundry-incubator/file-server/config"
	"github.com/cloudfoundry-incubator/file-server/handlers/static"
	"github.com/cloudfoundry-incubator/file-server/handlers/upload_droplet"
	"net/http"
)

func Exports(c *config.Config) map[string]http.Handler {
	return map[string]http.Handler{
		"static":         static.New(c.StaticDirectory),
		"upload_droplet": upload_droplet.New(c.CCAddress, c.CCUsername, c.CCPassword, c.CCJobPollingInterval),
	}
}
