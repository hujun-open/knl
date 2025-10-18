package controller

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"kubenetlab.net/knl/api/v1beta1"
)

type Config struct {
	config     *v1beta1.KNLConfigSpec
	lock       *sync.RWMutex
	generation int64
}

func (cfg *Config) Get() v1beta1.KNLConfigSpec {
	cfg.lock.RLock()
	defer cfg.lock.RUnlock()
	return *cfg.config
}

// return true if changed
func (cfg *Config) Set(new *v1beta1.KNLConfigSpec, gen int64) bool {
	cfg.lock.Lock()
	defer cfg.lock.Unlock()
	if cfg.generation != gen {
		cfg.config = new
		cfg.generation = gen
		return true
	}
	return false
}

func (cfg *Config) GetGen() int64 {
	cfg.lock.RLock()
	defer cfg.lock.RUnlock()
	return cfg.generation
}

func newConfig() *Config {
	defCfg := v1beta1.DefKNLConfig()
	return &Config{
		config:     &defCfg,
		lock:       new(sync.RWMutex),
		generation: -1,
	}
}

func getWatchNamespace() (string, error) {
	if ns := os.Getenv("WATCH_NAMESPACE"); ns != "" {
		return ns, nil
	}
	b, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err != nil {
		return "", fmt.Errorf("WATCH_NAMESPACE not set and failed reading serviceaccount namespace: %w", err)
	}
	return strings.TrimSpace(string(b)), nil
}

// the global configuration variable to be used by Lab operator
var GCONF *Config
var MYNAMESPACE string

func init() {
	GCONF = newConfig()
	var err error
	MYNAMESPACE, err = getWatchNamespace()
	if err != nil {
		panic(err)
	}

}
