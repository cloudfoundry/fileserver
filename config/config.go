package config

import (
	"errors"
	"time"
)

type Config struct {
	Address           string
	Port              int
	StaticDirectory   string
	LogLevel          string
	EtcdMachines      string
	HeartbeatInterval time.Duration

	CCAddress            string
	CCUsername           string
	CCPassword           string
	CCJobPollingInterval time.Duration
}

func New() *Config {
	return &Config{}
}

func (c *Config) Validate() []error {
	errs := make([]error, 0)
	if c.StaticDirectory == "" {
		errs = append(errs, errors.New("StaticDirectory is required"))
	}
	if c.CCAddress == "" {
		errs = append(errs, errors.New("CCAddress is required"))
	}
	if c.CCUsername == "" {
		errs = append(errs, errors.New("CCUsername is required"))
	}
	if c.CCPassword == "" {
		errs = append(errs, errors.New("CCPassword is required"))
	}
	return errs
}
