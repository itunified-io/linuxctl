package presets

import "github.com/itunified-io/linuxctl/pkg/config"

// init wires the bundle expander into pkg/config at import time. Any code
// path that imports pkg/presets (managers, CLI) gets automatic bundle
// expansion during config.LoadLinux / LoadEnv.
func init() {
	config.RegisterBundleExpander(func(name string) (map[string]string, error) {
		return BundleExpand(name, nil)
	})
}
