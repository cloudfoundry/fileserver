package maintain

import (
	"os"
	"time"

	Bbs "github.com/cloudfoundry-incubator/runtime-schema/bbs"
	"github.com/pivotal-golang/lager"
)

type Maintainer struct {
	url               string
	id                string
	bbs               Bbs.FileServerBBS
	logger            lager.Logger
	heartbeatInterval time.Duration
}

func New(url, id string, bbs Bbs.FileServerBBS, logger lager.Logger, heartbeatInterval time.Duration) *Maintainer {
	maintainerLogger := logger.Session("maintain-presense")
	return &Maintainer{
		url:               url,
		id:                id,
		bbs:               bbs,
		logger:            maintainerLogger,
		heartbeatInterval: heartbeatInterval,
	}
}

func (m *Maintainer) Run(sigChan <-chan os.Signal, ready chan<- struct{}) error {
	presence, status, err := m.bbs.MaintainFileServerPresence(m.heartbeatInterval, m.url, m.id)
	if err != nil {
		m.logger.Error("begin.failed", err)
	}

	for {
		select {
		case <-sigChan:
			presence.Remove()
			return nil

		case _, ok := <-status:
			if ok {
				if ready != nil {
					close(ready)
					ready = nil
				}
			} else {
				return nil
			}
		}
	}
}
