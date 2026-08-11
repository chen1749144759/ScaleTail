package dist

import "testing"

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
