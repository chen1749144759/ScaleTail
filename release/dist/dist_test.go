package dist

import (
	"testing"

	"scaletail.com/version/mkversion"
)

func TestUseGoCrossForDist(t *testing.T) {
	t.Setenv("SCALETAIL_DIST_USE_SYSTEM_GO", "")
	if !useGoCrossForDist() {
		t.Fatal("gocross should be enabled by default")
	}

	t.Setenv("SCALETAIL_DIST_USE_SYSTEM_GO", "1")
	if useGoCrossForDist() {
		t.Fatal("system Go override did not disable gocross")
	}
}

func TestSystemGoVersionLDFlags(t *testing.T) {
	got := systemGoVersionLDFlags(mkversion.VersionInfo{
		Short:   "0.0.10",
		Long:    "0.0.10-tabcdef123",
		GitHash: "abcdef1234567890",
	})
	want := "-X scaletail.com/version.longStamp=0.0.10-tabcdef123 " +
		"-X scaletail.com/version.shortStamp=0.0.10 " +
		"-X scaletail.com/version.gitCommitStamp=abcdef1234567890"
	if got != want {
		t.Fatalf("system Go ldflags = %q, want %q", got, want)
	}
}
