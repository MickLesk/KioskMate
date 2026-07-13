//go:build linux

package updater

import "os"

func inferPackageHistory(current string) (HistoryEntry, bool) {
	data, err := os.ReadFile("/var/log/dpkg.log")
	if err != nil {
		return HistoryEntry{}, false
	}
	const maxLogBytes = 2 << 20
	if len(data) > maxLogBytes {
		data = data[len(data)-maxLogBytes:]
	}
	return parseDPKGHistory(data, current)
}
