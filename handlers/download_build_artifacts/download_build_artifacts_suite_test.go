package download_build_artifacts_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestDownload_build_artifacts(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Download_build_artifacts Suite")
}
