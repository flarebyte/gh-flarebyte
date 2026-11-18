// purpose: Prepare the selected native artifact output directory.
// responsibilities: Apply output overrides consistently before writing artifacts.
package cli

import "os"

func prepareBuildOutputDir(configured, override string) (string, error) {
	if override != "" {
		configured = override
	}
	if err := os.MkdirAll(configured, 0o755); err != nil {
		return "", err
	}
	return configured, nil
}
