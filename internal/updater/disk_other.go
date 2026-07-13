//go:build !linux

package updater

import "fmt"

func freeDiskBytes(string) (int64, error) {
	return 0, fmt.Errorf("disk preflight is only supported on Linux")
}
