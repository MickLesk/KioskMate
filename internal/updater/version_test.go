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
		{"v1.0.0-pre.2", "v1.0.0-pre", 1},
		{"v1.0.0-pre", "v1.0.0-pre.2", -1},
		{"v1.0.0-pre.10", "v1.0.0-pre.2", 1},
		{"v1.0.0-pre2", "v1.0.0-pre", 1},
		{"v1.0.0-rc.1", "v1.0.0-pre.3", 1},
		{"v1.0.0-pre.2", "v1.0.0", -1},
		{"v1.0.0-pre.2", "v1.0.1-pre", -1},
	}
	for _, tc := range cases {
		got := compareVersion(tc.left, tc.right)
		if (got > 0) != (tc.want > 0) || (got < 0) != (tc.want < 0) {
			t.Errorf("compareVersion(%q, %q) = %d, want sign %d", tc.left, tc.right, got, tc.want)
		}
	}
}

func TestNewestReleasePicksHighestVersionOfChannel(t *testing.T) {
	releases := []githubRelease{
		{TagName: "v1.0.0-pre.1", Prerelease: true},
		{TagName: "v1.0.0-pre.2", Prerelease: true},
		{TagName: "v0.9.2", Draft: true},
		{TagName: "v0.9.1"},
		{TagName: "v0.9.0"},
	}
	for _, tc := range []struct {
		prerelease bool
		want       string
	}{{false, "v0.9.1"}, {true, "v1.0.0-pre.2"}} {
		got, err := newestRelease(releases, tc.prerelease)
		if err != nil || got.TagName != tc.want {
			t.Fatalf("prerelease=%v: got %q, %v; want %q", tc.prerelease, got.TagName, err, tc.want)
		}
	}
	if _, err := newestRelease([]githubRelease{{TagName: "v1.0.0-pre", Prerelease: true}}, false); err == nil {
		t.Fatal("expected no stable release")
	}
}
