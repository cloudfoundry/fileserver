package config_test

import (
	"io/ioutil"
	"os"

	"code.cloudfoundry.org/debugserver"
	loggingclient "code.cloudfoundry.org/diego-logging-client"
	"code.cloudfoundry.org/fileserver/cmd/file-server/config"
	"code.cloudfoundry.org/lager/lagerflags"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Config", func() {
	var configPath, configData string

	BeforeEach(func() {
		configData = `{
			"server_address": "192.168.1.1:8080",
			"static_directory": "/tmp/static",
			"consul_cluster": "consul.example.com",
			"debug_address": "127.0.0.1:17017",
			"enable_consul_service_registration": true,
			"log_level": "debug"
		}`
	})

	JustBeforeEach(func() {
		configFile, err := ioutil.TempFile("", "file-server-config")
		Expect(err).NotTo(HaveOccurred())

		configPath = configFile.Name()

		n, err := configFile.WriteString(configData)
		Expect(err).NotTo(HaveOccurred())
		Expect(n).To(Equal(len(configData)))
	})

	AfterEach(func() {
		err := os.RemoveAll(configPath)
		Expect(err).NotTo(HaveOccurred())
	})

	It("correctly parses the config file", func() {
		fileserverConfig, err := config.NewFileServerConfig(configPath)
		Expect(err).NotTo(HaveOccurred())

		expectedConfig := config.FileServerConfig{
			ServerAddress:   "192.168.1.1:8080",
			StaticDirectory: "/tmp/static",
			ConsulCluster:   "consul.example.com",
			DebugServerConfig: debugserver.DebugServerConfig{
				DebugAddress: "127.0.0.1:17017",
			},
			EnableConsulServiceRegistration: true,
			LagerConfig: lagerflags.LagerConfig{
				LogLevel: "debug",
			},
			LoggregatorConfig: loggingclient.Config{
				SourceID: "file_server",
			},
		}

		Expect(fileserverConfig).To(Equal(expectedConfig))
	})

	Context("when the file does not exist", func() {
		It("returns an error", func() {
			_, err := config.NewFileServerConfig("foobar")
			Expect(err).To(HaveOccurred())
		})
	})

	Context("when the file does not contain valid json", func() {
		BeforeEach(func() {
			configData = "{{"
		})

		It("returns an error", func() {
			_, err := config.NewFileServerConfig(configPath)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("DefaultConfig", func() {
		BeforeEach(func() {
			configData = `{}`
		})

		It("has default values", func() {
			fileserverConfig, err := config.NewFileServerConfig(configPath)
			Expect(err).NotTo(HaveOccurred())

			config := config.FileServerConfig{
				ServerAddress: "0.0.0.0:8080",
				LagerConfig: lagerflags.LagerConfig{
					LogLevel: "info",
				},
				LoggregatorConfig: loggingclient.Config{
					SourceID: "file_server",
				},
			}

			Expect(fileserverConfig).To(Equal(config))
		})
	})
})
