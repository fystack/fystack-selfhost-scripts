package semver

import "testing"

func TestLatest(t *testing.T) {
	got := Latest([]string{"latest", "1.0.9", "1.0.10", "bad", "v2.0.0"})
	if got != "v2.0.0" {
		t.Fatalf("want v2.0.0, got %q", got)
	}
}

func TestCompare(t *testing.T) {
	if Compare("1.0.10", "1.0.9") <= 0 {
		t.Fatal("expected 1.0.10 > 1.0.9")
	}
	if Compare("0.3.5", "0.3.5") != 0 {
		t.Fatal("expected equal versions")
	}
}
