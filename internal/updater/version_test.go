package updater

import "testing"

func TestCompareVersionOrdersPrereleases(t *testing.T) {
	cases := []struct {
		left, right string
		want        int
	}{
		{"v1.0.0-pre", "v0.9.1", 1},
		{"v1.0.0-pre", "v1.0.0", -1},
		{"1.0.0", "1.0.0-pre", 1},
		{"v1.0.0", "1.0.0", 0},
		{"0.9.1", "0.10.0", -1},
	}
	for _, tc := range cases {
		got := compareVersion(tc.left, tc.right)
		if (got > 0) != (tc.want > 0) || (got < 0) != (tc.want < 0) {
			t.Errorf("compareVersion(%q, %q) = %d, want sign %d", tc.left, tc.right, got, tc.want)
		}
	}
}
