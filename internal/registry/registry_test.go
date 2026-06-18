package registry

import "testing"

func TestImageTag(t *testing.T) {
	tests := []struct {
		image string
		tag   string
		ok    bool
	}{
		{"docker.io/fystacklabs/apex:1.0.54", "1.0.54", true},
		{"mongo:7.0", "7.0", true},
		{"postgres", "", false},
		{"localhost:5000/repo/image:1.2.3", "1.2.3", true},
	}
	for _, tt := range tests {
		tag, ok := ImageTag(tt.image)
		if tag != tt.tag || ok != tt.ok {
			t.Fatalf("ImageTag(%q) = %q, %v; want %q, %v", tt.image, tag, ok, tt.tag, tt.ok)
		}
	}
}
