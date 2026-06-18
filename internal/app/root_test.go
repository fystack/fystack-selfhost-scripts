package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type fakeRunner struct {
	output []byte
	calls  []runnerCall
}

type runnerCall struct {
	dir  string
	name string
	args []string
}

func (f *fakeRunner) Run(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, runnerCall{dir: dir, name: name, args: append([]string(nil), args...)})
	return f.output, nil
}

func (f *fakeRunner) LookPath(name string) error {
	return nil
}

type fakeTags map[string][]string

func (f fakeTags) Tags(ctx context.Context, image string) ([]string, error) {
	return f[image], nil
}

func TestStatusWritesEnvAndRunsCompose(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "stack.versions.yaml"), `services:
  apex:
    env: FYSTACK_APEX_IMAGE
    image: docker.io/fystacklabs/apex:1.0.54
    environments: [dev]
`)
	mustWrite(t, filepath.Join(root, "dev", "docker-compose.yaml"), "services: {}\n")

	var out bytes.Buffer
	runner := &fakeRunner{}
	cmd := newRootCommand(dependencies{
		workDir:        root,
		out:            &out,
		errOut:         &out,
		runner:         runner,
		tagLister:      fakeTags{},
		versionFile:    "stack.versions.yaml",
		composeEnvFile: ".fystack.compose.env",
	})
	cmd.SetArgs([]string{"--env", "dev", "status"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	envFile, err := os.ReadFile(filepath.Join(root, ".fystack.compose.env"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(envFile), "FYSTACK_APEX_IMAGE=docker.io/fystacklabs/apex:1.0.54") {
		t.Fatalf("env file did not contain apex image:\n%s", envFile)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected one runner call, got %d", len(runner.calls))
	}
	wantArgs := []string{"compose", "--env-file", filepath.Join(root, ".fystack.compose.env"), "-f", "docker-compose.yaml", "ps"}
	if !reflect.DeepEqual(runner.calls[0].args, wantArgs) {
		t.Fatalf("args mismatch\nwant: %#v\n got: %#v", wantArgs, runner.calls[0].args)
	}
	if runner.calls[0].dir != filepath.Join(root, "dev") {
		t.Fatalf("unexpected dir %q", runner.calls[0].dir)
	}
}

func TestRestartNamedServicesRunsComposeRestart(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "stack.versions.yaml"), `services:
  apex:
    env: FYSTACK_APEX_IMAGE
    image: docker.io/fystacklabs/apex:1.0.54
    environments: [dev]
`)
	mustWrite(t, filepath.Join(root, "dev", "docker-compose.yaml"), `services:
  apex: {}
  rescanner: {}
`)

	var out bytes.Buffer
	runner := &fakeRunner{}
	cmd := newRootCommand(dependencies{
		workDir:        root,
		out:            &out,
		errOut:         &out,
		runner:         runner,
		tagLister:      fakeTags{},
		versionFile:    "stack.versions.yaml",
		composeEnvFile: ".fystack.compose.env",
	})
	cmd.SetArgs([]string{"--env", "dev", "restart", "apex", "rescanner"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected one runner call, got %#v", runner.calls)
	}
	wantArgs := []string{"compose", "--env-file", filepath.Join(root, ".fystack.compose.env"), "-f", "docker-compose.yaml", "restart", "apex", "rescanner"}
	if !reflect.DeepEqual(runner.calls[0].args, wantArgs) {
		t.Fatalf("args mismatch\nwant: %#v\n got: %#v", wantArgs, runner.calls[0].args)
	}
}

func TestRestartInteractiveFallbackSelectsServices(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "stack.versions.yaml"), `services:
  apex:
    env: FYSTACK_APEX_IMAGE
    image: docker.io/fystacklabs/apex:1.0.54
    environments: [dev]
`)
	mustWrite(t, filepath.Join(root, "dev", "docker-compose.yaml"), `services:
  rescanner: {}
  apex: {}
  mongo: {}
`)

	var out bytes.Buffer
	runner := &fakeRunner{}
	cmd := newRootCommand(dependencies{
		workDir:        root,
		in:             strings.NewReader("1,3\n"),
		out:            &out,
		errOut:         &out,
		runner:         runner,
		tagLister:      fakeTags{},
		versionFile:    "stack.versions.yaml",
		composeEnvFile: ".fystack.compose.env",
	})
	cmd.SetArgs([]string{"--env", "dev", "restart"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected one runner call, got %#v", runner.calls)
	}
	wantArgs := []string{"compose", "--env-file", filepath.Join(root, ".fystack.compose.env"), "-f", "docker-compose.yaml", "restart", "apex", "rescanner"}
	if !reflect.DeepEqual(runner.calls[0].args, wantArgs) {
		t.Fatalf("args mismatch\nwant: %#v\n got: %#v", wantArgs, runner.calls[0].args)
	}
}

func TestCheckUpdatesReportsSemverUpdatesAndSkipsLatest(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "stack.versions.yaml"), `services:
  apex:
    env: FYSTACK_APEX_IMAGE
    image: docker.io/fystacklabs/apex:1.0.54
    environments: [dev]
  nats-dev:
    env: FYSTACK_NATS_IMAGE
    image: nats:latest
    environments: [dev]
  mongo-dev:
    env: FYSTACK_MONGO_IMAGE
    image: mongo:7.0
    environments: [dev]
`)
	mustWrite(t, filepath.Join(root, "dev", "docker-compose.yaml"), "services: {}\n")

	var out bytes.Buffer
	cmd := newRootCommand(dependencies{
		workDir: root,
		out:     &out,
		errOut:  &out,
		runner:  &fakeRunner{},
		tagLister: fakeTags{
			"docker.io/fystacklabs/apex:1.0.54": {"1.0.53", "1.0.55", "latest", "bad"},
			"mongo:7.0":                         {"7.0", "8.3.4"},
		},
		versionFile:    "stack.versions.yaml",
		composeEnvFile: ".fystack.compose.env",
	})
	cmd.SetArgs([]string{"--env", "dev", "check-updates"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "apex: update available 1.0.54 -> 1.0.55") {
		t.Fatalf("missing apex update:\n%s", got)
	}
	if strings.Contains(got, "nats") || strings.Contains(got, "mongo-dev") {
		t.Fatalf("infrastructure services should not be reported:\n%s", got)
	}
}

func TestUpdateAllWritesLatestImagePinsWithoutDeploy(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "stack.versions.yaml"), `services:
  apex:
    env: FYSTACK_APEX_IMAGE
    image: docker.io/fystacklabs/apex:1.0.54
    environments: [dev]
  redis:
    env: FYSTACK_REDIS_IMAGE
    image: redis:latest
    environments: [dev]
  mongo-dev:
    env: FYSTACK_MONGO_IMAGE
    image: mongo:7.0
    environments: [dev]
`)
	mustWrite(t, filepath.Join(root, "dev", "docker-compose.yaml"), "services: {}\n")

	var out bytes.Buffer
	runner := &fakeRunner{}
	cmd := newRootCommand(dependencies{
		workDir: root,
		in:      strings.NewReader("no\n"),
		out:     &out,
		errOut:  &out,
		runner:  runner,
		tagLister: fakeTags{
			"docker.io/fystacklabs/apex:1.0.54": {"1.0.55", "1.0.66"},
			"mongo:7.0":                         {"7.0", "8.3.4"},
		},
		versionFile:    "stack.versions.yaml",
		composeEnvFile: ".fystack.compose.env",
	})
	cmd.SetArgs([]string{"--env", "dev", "update", "--all"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "stack.versions.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	gotFile := string(data)
	if !strings.Contains(gotFile, "image: docker.io/fystacklabs/apex:1.0.66") {
		t.Fatalf("missing updated apex image:\n%s", gotFile)
	}
	if !strings.Contains(gotFile, "image: redis:latest") {
		t.Fatalf("redis should be unchanged:\n%s", gotFile)
	}
	if !strings.Contains(gotFile, "image: mongo:7.0") {
		t.Fatalf("mongo should be skipped by app updates:\n%s", gotFile)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("expected no deploy runner calls, got %#v", runner.calls)
	}
	gotOut := out.String()
	if !strings.Contains(gotOut, "updated apex: 1.0.54 -> 1.0.66") {
		t.Fatalf("missing update output:\n%s", gotOut)
	}
	if !strings.Contains(gotOut, "run `fystack deploy`") {
		t.Fatalf("missing deploy reminder:\n%s", gotOut)
	}
}

func TestUpdateInteractiveFallbackSelectsAppUpdates(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "stack.versions.yaml"), `services:
  apex:
    env: FYSTACK_APEX_IMAGE
    image: docker.io/fystacklabs/apex:1.0.54
    environments: [dev]
  rescanner:
    env: FYSTACK_RESCANNER_IMAGE
    image: docker.io/fystacklabs/rescanner:1.0.1
    environments: [dev]
`)
	mustWrite(t, filepath.Join(root, "dev", "docker-compose.yaml"), `services:
  apex: {}
  rescanner: {}
`)

	var out bytes.Buffer
	runner := &fakeRunner{}
	cmd := newRootCommand(dependencies{
		workDir: root,
		in:      strings.NewReader("1\nno\n"),
		out:     &out,
		errOut:  &out,
		runner:  runner,
		tagLister: fakeTags{
			"docker.io/fystacklabs/apex:1.0.54":     {"1.0.66"},
			"docker.io/fystacklabs/rescanner:1.0.1": {"1.0.2"},
		},
		versionFile:    "stack.versions.yaml",
		composeEnvFile: ".fystack.compose.env",
	})
	cmd.SetArgs([]string{"--env", "dev", "update"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "stack.versions.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	gotFile := string(data)
	if !strings.Contains(gotFile, "image: docker.io/fystacklabs/apex:1.0.66") {
		t.Fatalf("missing updated apex image:\n%s", gotFile)
	}
	if !strings.Contains(gotFile, "image: docker.io/fystacklabs/rescanner:1.0.1") {
		t.Fatalf("rescanner should be unchanged:\n%s", gotFile)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("expected no deploy runner calls, got %#v", runner.calls)
	}
	gotOut := out.String()
	if !strings.Contains(gotOut, "Select app updates to apply") {
		t.Fatalf("missing update selector:\n%s", gotOut)
	}
	if !strings.Contains(gotOut, "Deploy updated images now?") {
		t.Fatalf("missing deploy prompt:\n%s", gotOut)
	}
}

func TestUpdateDeploysSelectedComposeServices(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "stack.versions.yaml"), `services:
  apex-migrate:
    env: FYSTACK_APEX_MIGRATE_IMAGE
    image: docker.io/fystacklabs/apex-migrate:1.0.24
    environments: [dev]
  mpcium:
    env: FYSTACK_MPCIUM_IMAGE
    image: docker.io/fystacklabs/mpcium:v1.0.0
    environments: [dev]
`)
	mustWrite(t, filepath.Join(root, "dev", "docker-compose.yaml"), `services:
  migrate: {}
  mpcium1: {}
  mpcium0: {}
`)

	var out bytes.Buffer
	runner := &fakeRunner{}
	cmd := newRootCommand(dependencies{
		workDir: root,
		out:     &out,
		errOut:  &out,
		runner:  runner,
		tagLister: fakeTags{
			"docker.io/fystacklabs/apex-migrate:1.0.24": {"1.0.26"},
			"docker.io/fystacklabs/mpcium:v1.0.0":       {"v1.0.1"},
		},
		versionFile:    "stack.versions.yaml",
		composeEnvFile: ".fystack.compose.env",
	})
	cmd.SetArgs([]string{"--env", "dev", "update", "apex-migrate", "mpcium", "--deploy"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected one deploy runner call, got %#v", runner.calls)
	}
	wantArgs := []string{"compose", "--env-file", filepath.Join(root, ".fystack.compose.env"), "-f", "docker-compose.yaml", "up", "-d", "migrate", "mpcium0", "mpcium1"}
	if !reflect.DeepEqual(runner.calls[0].args, wantArgs) {
		t.Fatalf("args mismatch\nwant: %#v\n got: %#v", wantArgs, runner.calls[0].args)
	}
	if !strings.Contains(out.String(), "deploying updated services: migrate, mpcium0, mpcium1") {
		t.Fatalf("missing deploy summary:\n%s", out.String())
	}
}

func TestUpdateNamedServiceReportsNoUpdates(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "stack.versions.yaml"), `services:
  apex:
    env: FYSTACK_APEX_IMAGE
    image: docker.io/fystacklabs/apex:1.0.54
    environments: [dev]
`)
	mustWrite(t, filepath.Join(root, "dev", "docker-compose.yaml"), "services: {}\n")

	var out bytes.Buffer
	cmd := newRootCommand(dependencies{
		workDir: root,
		in:      strings.NewReader("no\n"),
		out:     &out,
		errOut:  &out,
		runner:  &fakeRunner{},
		tagLister: fakeTags{
			"docker.io/fystacklabs/apex:1.0.54": {"1.0.54"},
		},
		versionFile:    "stack.versions.yaml",
		composeEnvFile: ".fystack.compose.env",
	})
	cmd.SetArgs([]string{"--env", "dev", "update", "apex"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "no semver updates available") {
		t.Fatalf("missing no updates output:\n%s", out.String())
	}
}

func TestDoctorChecksComposeVersionWithoutComposeEnvFile(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "stack.versions.yaml"), "services: {}\n")
	mustWrite(t, filepath.Join(root, "dev", "docker-compose.yaml"), "services: {}\n")
	mustWrite(t, filepath.Join(root, "dev", "config.yaml"), "name: test\n")
	mustWrite(t, filepath.Join(root, "dev", "config.rescanner.yaml"), "name: test\n")
	mustWrite(t, filepath.Join(root, "dev", "config.indexer.yaml"), "name: test\n")

	var out bytes.Buffer
	runner := &fakeRunner{}
	cmd := newRootCommand(dependencies{
		workDir:        root,
		out:            &out,
		errOut:         &out,
		runner:         runner,
		tagLister:      fakeTags{},
		versionFile:    "stack.versions.yaml",
		composeEnvFile: ".fystack.compose.env",
	})
	cmd.SetArgs([]string{"doctor", "--env", "dev"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected one runner call, got %d", len(runner.calls))
	}
	wantArgs := []string{"compose", "version"}
	if !reflect.DeepEqual(runner.calls[0].args, wantArgs) {
		t.Fatalf("args mismatch\nwant: %#v\n got: %#v", wantArgs, runner.calls[0].args)
	}
	if _, err := os.Stat(filepath.Join(root, ".fystack.compose.env")); !os.IsNotExist(err) {
		t.Fatalf("doctor should not create compose env file, stat err: %v", err)
	}
}

func TestSetupCopiesDevConfigsWithBinanceAndSkipsActions(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "stack.versions.yaml"), "services: {}\n")
	mustWrite(t, filepath.Join(root, "dev", "docker-compose.yaml"), "services: {}\n")
	mustWrite(t, filepath.Join(root, "dev", "config.yaml.template"), `price_providers:
  coinmarketcap:
    api_key: ""
  binance:
    endpoint: "https://api.binance.com/api/v3/ticker/price"
`)
	mustWrite(t, filepath.Join(root, "dev", "config.rescanner.yaml.template"), "rescanner: true\n")
	mustWrite(t, filepath.Join(root, "dev", "config.indexer.yaml.template"), "indexer: true\n")

	var out bytes.Buffer
	runner := &fakeRunner{}
	cmd := newRootCommand(dependencies{
		workDir:        root,
		in:             strings.NewReader("\n\nno\nno\n"),
		out:            &out,
		errOut:         &out,
		runner:         runner,
		tagLister:      fakeTags{},
		versionFile:    "stack.versions.yaml",
		composeEnvFile: ".fystack.compose.env",
	})
	cmd.SetArgs([]string{"setup"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	config, err := os.ReadFile(filepath.Join(root, "dev", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(config), `cmc-test-key`) {
		t.Fatalf("config should not contain a prompted API key for Binance:\n%s", config)
	}
	for _, path := range []string{"config.rescanner.yaml", "config.indexer.yaml"} {
		if _, err := os.Stat(filepath.Join(root, "dev", path)); err != nil {
			t.Fatalf("expected %s to be created: %v", path, err)
		}
	}
	if len(runner.calls) != 0 {
		t.Fatalf("expected setup to skip runner calls, got %#v", runner.calls)
	}
	got := out.String()
	if !strings.Contains(got, "setup complete") {
		t.Fatalf("missing setup completion output:\n%s", got)
	}
	if !strings.Contains(got, "using Binance price provider; no API key needed") {
		t.Fatalf("missing Binance no-key output:\n%s", got)
	}
}

func TestSetupCanOverwriteConfigsAndRequireCoinMarketCapKey(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "stack.versions.yaml"), "services: {}\n")
	mustWrite(t, filepath.Join(root, "dev", "docker-compose.yaml"), "services: {}\n")
	mustWrite(t, filepath.Join(root, "dev", "config.yaml.template"), `price_providers:
  coinmarketcap:
    api_key: ""
  binance:
    endpoint: "https://api.binance.com/api/v3/ticker/price"
`)
	mustWrite(t, filepath.Join(root, "dev", "config.rescanner.yaml.template"), "rescanner: true\n")
	mustWrite(t, filepath.Join(root, "dev", "config.indexer.yaml.template"), "indexer: true\n")
	mustWrite(t, filepath.Join(root, "dev", "config.yaml"), `price_providers:
  coinmarketcap:
    api_key: "old-key"
`)
	mustWrite(t, filepath.Join(root, "dev", "config.rescanner.yaml"), "old: rescanner\n")
	mustWrite(t, filepath.Join(root, "dev", "config.indexer.yaml"), "old: indexer\n")

	var out bytes.Buffer
	runner := &fakeRunner{}
	cmd := newRootCommand(dependencies{
		workDir:        root,
		in:             strings.NewReader("\nyes\n2\ncmc-test-key\nno\nno\n"),
		out:            &out,
		errOut:         &out,
		runner:         runner,
		tagLister:      fakeTags{},
		versionFile:    "stack.versions.yaml",
		composeEnvFile: ".fystack.compose.env",
	})
	cmd.SetArgs([]string{"setup"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	config, err := os.ReadFile(filepath.Join(root, "dev", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(config), `api_key: "cmc-test-key"`) {
		t.Fatalf("config did not contain prompted API key:\n%s", config)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("expected setup to skip runner calls, got %#v", runner.calls)
	}
	got := out.String()
	if !strings.Contains(got, "overwrote dev/config.yaml") {
		t.Fatalf("missing overwrite output:\n%s", got)
	}
}

func TestInitForceRunsNodeSetupWhenNodeConfigsExist(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "stack.versions.yaml"), "services: {}\n")
	mustWrite(t, filepath.Join(root, "dev", "docker-compose.yaml"), "services: {}\n")
	mustWrite(t, filepath.Join(root, "dev", "config.yaml"), `mpc:
  signer:
    local:
      pk_raw: ""
integrity:
  signer:
    ed25519:
      private_key: ""
`)
	mustWrite(t, filepath.Join(root, "dev", "config.rescanner.yaml"), "name: test\n")
	mustWrite(t, filepath.Join(root, "dev", "config.indexer.yaml"), "name: test\n")
	mustWrite(t, filepath.Join(root, "dev", "node-configs", "node0", "config.yaml"), "name: node0\n")

	var out bytes.Buffer
	runner := &mpciumFakeRunner{}
	cmd := newRootCommand(dependencies{
		workDir:        root,
		out:            &out,
		errOut:         &out,
		runner:         runner,
		tagLister:      fakeTags{},
		versionFile:    "stack.versions.yaml",
		composeEnvFile: ".fystack.compose.env",
	})
	cmd.SetArgs([]string{"init", "--env", "dev", "--force"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) == 0 {
		t.Fatal("expected runner calls for docker pull and mpcium-cli, got none")
	}
	if runner.calls[0].name != "docker" {
		t.Fatalf("expected first call to be docker, got %q", runner.calls[0].name)
	}
	if len(runner.calls[0].args) < 1 || runner.calls[0].args[0] != "pull" {
		t.Fatalf("expected docker pull as first call, got args %#v", runner.calls[0].args)
	}
	for _, call := range runner.calls {
		if strings.HasSuffix(call.name, "setup-nodes.sh") {
			t.Fatal("init should use Go-based node setup, not setup-nodes.sh")
		}
	}
	nodeConfigsDir := filepath.Join(root, "dev", "node-configs")
	for _, node := range []string{"node0", "node1", "node2"} {
		identDir := filepath.Join(nodeConfigsDir, node, "identity")
		if _, err := os.Stat(identDir); err != nil {
			t.Fatalf("expected %s identity dir to exist: %v", node, err)
		}
	}
}

// mpciumFakeRunner simulates docker by writing the files that mpcium-cli would produce.
type mpciumFakeRunner struct {
	calls []runnerCall
}

func (r *mpciumFakeRunner) LookPath(string) error { return nil }

func (r *mpciumFakeRunner) Run(_ context.Context, callDir, name string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, runnerCall{dir: callDir, name: name, args: append([]string(nil), args...)})
	if name != "docker" || len(args) == 0 || args[0] != "run" {
		return nil, nil
	}

	var mountSrc string
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "-v" {
			mountSrc = strings.SplitN(args[i+1], ":", 2)[0]
			break
		}
	}

	for i, arg := range args {
		if !strings.Contains(arg, "mpcium-cli") || i+1 >= len(args) {
			continue
		}
		mpcArgs := args[i+1:]
		switch mpcArgs[0] {
		case "generate-peers":
			peers := `{"node0":"peer-id-0","node1":"peer-id-1","node2":"peer-id-2"}`
			_ = os.WriteFile(filepath.Join(mountSrc, "peers.json"), []byte(peers), 0644)
		case "generate-initiator":
			_ = os.WriteFile(filepath.Join(mountSrc, "event_initiator.key"), []byte("testprivkey"), 0644)
			_ = os.WriteFile(filepath.Join(mountSrc, "event_initiator.identity.json"), []byte(`{"public_key":"testpubkey"}`), 0644)
		case "generate-identity":
			for j, a := range mpcArgs {
				if a == "--node" && j+1 < len(mpcArgs) {
					nodeName := mpcArgs[j+1]
					identDir := filepath.Join(mountSrc, "identity")
					_ = os.MkdirAll(identDir, 0755)
					_ = os.WriteFile(filepath.Join(identDir, nodeName+"_identity.json"), []byte(`{"peer_id":"pid","public_key":"pubkey"}`), 0644)
					_ = os.WriteFile(filepath.Join(identDir, nodeName+"_private.key"), []byte("privkey"), 0644)
					break
				}
			}
		}
		break
	}
	return nil, nil
}

func TestResetRemovesGeneratedFiles(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "stack.versions.yaml"), "services: {}\n")
	mustWrite(t, filepath.Join(root, "dev", "docker-compose.yaml"), "services: {}\n")
	mustWrite(t, filepath.Join(root, "dev", "config.yaml"), "name: test\n")
	mustWrite(t, filepath.Join(root, "dev", "config.rescanner.yaml"), "name: test\n")
	mustWrite(t, filepath.Join(root, "dev", "config.indexer.yaml"), "name: test\n")
	mustWrite(t, filepath.Join(root, "dev", "node-configs", "node0", "config.yaml"), "name: node0\n")
	mustWrite(t, filepath.Join(root, ".fystack.compose.env"), "FYSTACK_APEX_IMAGE=x\n")

	var out bytes.Buffer
	cmd := newRootCommand(dependencies{
		workDir:        root,
		in:             strings.NewReader("yes\n"),
		out:            &out,
		errOut:         &out,
		runner:         &fakeRunner{},
		tagLister:      fakeTags{},
		versionFile:    "stack.versions.yaml",
		composeEnvFile: ".fystack.compose.env",
	})
	cmd.SetArgs([]string{"reset"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(root, "dev", "config.yaml"),
		filepath.Join(root, "dev", "config.rescanner.yaml"),
		filepath.Join(root, "dev", "config.indexer.yaml"),
		filepath.Join(root, "dev", "node-configs"),
		filepath.Join(root, ".fystack.compose.env"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("expected %s to be removed, stat err: %v", path, err)
		}
	}
	if !strings.Contains(out.String(), "reset complete") {
		t.Fatalf("missing reset complete output:\n%s", out.String())
	}
}

func TestResetForceSkipsConfirmation(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "stack.versions.yaml"), "services: {}\n")
	mustWrite(t, filepath.Join(root, "dev", "docker-compose.yaml"), "services: {}\n")
	mustWrite(t, filepath.Join(root, "dev", "config.yaml"), "name: test\n")

	var out bytes.Buffer
	cmd := newRootCommand(dependencies{
		workDir:        root,
		out:            &out,
		errOut:         &out,
		runner:         &fakeRunner{},
		tagLister:      fakeTags{},
		versionFile:    "stack.versions.yaml",
		composeEnvFile: ".fystack.compose.env",
	})
	cmd.SetArgs([]string{"reset", "--force"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "dev", "config.yaml")); !os.IsNotExist(err) {
		t.Fatalf("expected config.yaml to be removed")
	}
}

func TestConfirmDefaultsNoFirstForDestructivePrompts(t *testing.T) {
	var out bytes.Buffer
	prompt := newPrompter(dependencies{
		in:  strings.NewReader("\n"),
		out: &out,
	})

	ok, err := prompt.confirm("Overwrite existing files?", false)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected empty answer to choose no")
	}
	got := out.String()
	noIndex := strings.Index(got, "1. no")
	yesIndex := strings.Index(got, "2. yes")
	if noIndex == -1 || yesIndex == -1 || noIndex > yesIndex {
		t.Fatalf("destructive prompt should render no before yes:\n%s", got)
	}
}

func TestDiscoverMPCServicesSortsByNodeIndex(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"node10", "node2", "node0"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0755); err != nil {
			t.Fatal(err)
		}
	}
	got := discoverMPCServices(root)
	want := []string{"mpcium0", "mpcium2", "mpcium10"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("want %#v, got %#v", want, got)
	}
}

func TestDestroyRunsComposeDownAndResetsFiles(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "stack.versions.yaml"), `services:
  apex:
    env: FYSTACK_APEX_IMAGE
    image: docker.io/fystacklabs/apex:1.0.54
    environments: [dev]
`)
	mustWrite(t, filepath.Join(root, "dev", "docker-compose.yaml"), "services: {}\n")
	mustWrite(t, filepath.Join(root, "dev", "config.yaml"), "name: test\n")
	mustWrite(t, filepath.Join(root, "dev", "config.rescanner.yaml"), "name: test\n")
	mustWrite(t, filepath.Join(root, "dev", "config.indexer.yaml"), "name: test\n")
	mustWrite(t, filepath.Join(root, "dev", "node-configs", "node0", "config.yaml"), "name: node0\n")
	mustWrite(t, filepath.Join(root, ".fystack.compose.env"), "FYSTACK_APEX_IMAGE=x\n")

	var out bytes.Buffer
	runner := &fakeRunner{}
	cmd := newRootCommand(dependencies{
		workDir:        root,
		in:             strings.NewReader("yes\n"),
		out:            &out,
		errOut:         &out,
		runner:         runner,
		tagLister:      fakeTags{},
		versionFile:    "stack.versions.yaml",
		composeEnvFile: ".fystack.compose.env",
	})
	cmd.SetArgs([]string{"--env", "dev", "destroy"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	if len(runner.calls) != 1 {
		t.Fatalf("expected one compose down call, got %d: %#v", len(runner.calls), runner.calls)
	}
	wantArgs := []string{"compose", "--env-file", filepath.Join(root, ".fystack.compose.env"), "-f", "docker-compose.yaml", "down", "--volumes", "--remove-orphans"}
	if !reflect.DeepEqual(runner.calls[0].args, wantArgs) {
		t.Fatalf("args mismatch\nwant: %#v\n got: %#v", wantArgs, runner.calls[0].args)
	}

	for _, path := range []string{
		filepath.Join(root, "dev", "config.yaml"),
		filepath.Join(root, "dev", "node-configs"),
		filepath.Join(root, ".fystack.compose.env"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("expected %s to be removed after destroy", path)
		}
	}

	got := out.String()
	if !strings.Contains(got, "Destroy plan:") {
		t.Fatalf("missing destroy plan output:\n%s", got)
	}
	if !strings.Contains(got, "Destroy complete.") {
		t.Fatalf("missing completion message:\n%s", got)
	}
}

func TestDestroyAbortedOnNo(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "stack.versions.yaml"), `services:
  apex:
    env: FYSTACK_APEX_IMAGE
    image: docker.io/fystacklabs/apex:1.0.54
    environments: [dev]
`)
	mustWrite(t, filepath.Join(root, "dev", "docker-compose.yaml"), "services: {}\n")
	mustWrite(t, filepath.Join(root, "dev", "config.yaml"), "name: test\n")

	var out bytes.Buffer
	runner := &fakeRunner{}
	cmd := newRootCommand(dependencies{
		workDir:        root,
		in:             strings.NewReader("no\n"),
		out:            &out,
		errOut:         &out,
		runner:         runner,
		tagLister:      fakeTags{},
		versionFile:    "stack.versions.yaml",
		composeEnvFile: ".fystack.compose.env",
	})
	cmd.SetArgs([]string{"--env", "dev", "destroy"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("expected no runner calls when destroy is canceled, got %#v", runner.calls)
	}
	if _, err := os.Stat(filepath.Join(root, "dev", "config.yaml")); err != nil {
		t.Fatal("config.yaml should still exist after canceled destroy")
	}
	if !strings.Contains(out.String(), "destroy canceled") {
		t.Fatalf("missing canceled message:\n%s", out.String())
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
