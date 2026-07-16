package versions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadAndEnvFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stack.versions.yaml")
	if err := os.WriteFile(path, []byte(`services:
  apex:
    env: FYSTACK_APEX_IMAGE
    image: docker.io/fystacklabs/apex:1.0.54
    environments: [dev]
  mongo-prod:
    env: FYSTACK_MONGO_PROD_IMAGE
    image: mongo:7.0
    environments: [prod]
`), 0644); err != nil {
		t.Fatal(err)
	}
	defs, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	env := defs.EnvFile("dev")
	if !strings.Contains(env, "FYSTACK_APEX_IMAGE=docker.io/fystacklabs/apex:1.0.54") {
		t.Fatalf("missing apex env:\n%s", env)
	}
	if strings.Contains(env, "FYSTACK_MONGO_PROD_IMAGE") {
		t.Fatalf("prod env leaked into dev env:\n%s", env)
	}
}

func TestUpdateImagesPreservesOtherServices(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stack.versions.yaml")
	if err := os.WriteFile(path, []byte(`services:
  apex:
    env: FYSTACK_APEX_IMAGE
    image: docker.io/fystacklabs/apex:1.0.54
    environments: [dev]
  redis:
    env: FYSTACK_REDIS_IMAGE
    image: redis:latest
    environments: [dev]
`), 0644); err != nil {
		t.Fatal(err)
	}

	if err := UpdateImages(path, map[string]string{
		"apex": "docker.io/fystacklabs/apex:1.0.66",
	}); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if !strings.Contains(got, "image: docker.io/fystacklabs/apex:1.0.66") {
		t.Fatalf("missing updated apex image:\n%s", got)
	}
	if !strings.Contains(got, "image: redis:latest") {
		t.Fatalf("redis image should be unchanged:\n%s", got)
	}
}
