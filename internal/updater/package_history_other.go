//go:build !linux

package updater

func inferPackageHistory(string) (HistoryEntry, bool) {
	return HistoryEntry{}, false
}
