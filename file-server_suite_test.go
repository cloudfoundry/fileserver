package main_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/vito/cmdtest"

	"testing"
)

var fileServerBinary string

func TestFileServer(t *testing.T) {
	RegisterFailHandler(Fail)

	var err error
	fileServerBinary, err = cmdtest.Build("github.com/cloudfoundry-incubator/file-server")
	if err != nil {
		panic(err.Error())
	}

	RunSpecs(t, "File Server Suite")
}
