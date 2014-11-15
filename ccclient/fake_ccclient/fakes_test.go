package fake_ccclient_test

import (
	"github.com/cloudfoundry-incubator/file-server/ccclient"
	. "github.com/cloudfoundry-incubator/file-server/ccclient/fake_ccclient"

	. "github.com/onsi/ginkgo"
)

var _ = Describe("Fakes", func() {
	It("is an Uploader", func() {
		var _ ccclient.Uploader = &FakeUploader{}
	})

	It("is a Poller", func() {
		var _ ccclient.Poller = &FakePoller{}
	})
})
