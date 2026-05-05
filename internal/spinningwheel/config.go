package spinningwheel

import (
	"time"

	"github.com/genie-cvc/verdandi/internal/verdandi"
)

type Config struct {
	Name               string        `json:"name"`
	Enabled            bool          `json:"enabled"`
	DataDir            string        `json:"dataDir"`
	Addr               string        `json:"addr"`
	StreamPollInterval time.Duration `json:"streamPollInterval"`
}

func DefaultConfig() Config {
	return Config{
		Name:               "Spinning Wheel",
		Enabled:            true,
		DataDir:            verdandi.DefaultDataDir(),
		Addr:               "127.0.0.1:8787",
		StreamPollInterval: 750 * time.Millisecond,
	}
}

func (c Config) WithDataDir(dataDir string) Config {
	if dataDir != "" {
		c.DataDir = dataDir
	}
	return c
}

func (c Config) WithAddr(addr string) Config {
	if addr != "" {
		c.Addr = addr
	}
	return c
}

func (c Config) WithEnabled(enabled bool) Config {
	c.Enabled = enabled
	return c
}
