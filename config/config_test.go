package config_test

import (
	"errors"
	. "github.com/cloudfoundry-incubator/file_server/config"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Config", func() {
	Describe("Validate", func() {
		It("should return errors when required things are missing", func() {
			c := New()
			errs := c.Validate()
			Ω(errs).Should(ContainElement(errors.New("CCAddress is required")))
			Ω(errs).Should(ContainElement(errors.New("CCUsername is required")))
			Ω(errs).Should(ContainElement(errors.New("CCPassword is required")))
			Ω(errs).Should(ContainElement(errors.New("StaticDirectory is required")))
		})

		It("should return nothing if all is well", func() {
			c := New()

			c.CCAddress = "http://cc.com"
			c.CCUsername = "bob"
			c.CCPassword = "password"
			c.StaticDirectory = "/somewhere/static"

			errs := c.Validate()
			Ω(errs).Should(BeEmpty())
		})
	})
})
