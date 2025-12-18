package v1beta1

import (
	"fmt"
	"os"
	"strings"
	"sync"
)

// +kubebuilder:object:generate=false
// +kubebuilder:object:root=false
// +hidefromdoc
type Config struct {
	config     *KNLConfigSpec
	lock       *sync.RWMutex
	generation int64
}

func (cfg *Config) Get() KNLConfigSpec {
	cfg.lock.RLock()
	defer cfg.lock.RUnlock()
	return *cfg.config
}

// return true if changed
func (cfg *Config) Set(new *KNLConfigSpec, gen int64) bool {
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
	defCfg := DefKNLConfig()
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

func isRunningInsideManagerPod() bool {
	for _, env := range os.Environ() {
		if strings.HasPrefix(env, "KNL_") {
			return true
		}
	}
	return false
}

func init() {
	GCONF = newConfig()
	var err error
	os.Environ()
	if isRunningInsideManagerPod() {
		MYNAMESPACE, err = getWatchNamespace()
		if err != nil {
			panic(err)
		}
	}

}
