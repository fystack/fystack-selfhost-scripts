package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fystack/fystack-selfhost-scripts/internal/stack"
)

const defaultMPCIUMCLIImage = "docker.io/fystacklabs/mpcium-cli:0.3.5"

type nodeSetupOptions struct {
	nodes         int
	threshold     int
	environment   string
	encryptKeys   bool
	overwrite     bool
	natsURL       string
	consulAddress string
	cliImage      string
}

func defaultNodeSetupOptions(overwrite bool) nodeSetupOptions {
	return nodeSetupOptions{
		nodes:         3,
		threshold:     2,
		environment:   "development",
		overwrite:     overwrite,
		natsURL:       "nats://nats-server:4222",
		consulAddress: "consul:8500",
		cliImage:      defaultMPCIUMCLIImage,
	}
}

func runNodeSetup(ctx context.Context, deps dependencies, env stack.Environment, opts nodeSetupOptions) error {
	if env.Name != "dev" {
		return errors.New("node setup is only implemented for --env dev")
	}
	if opts.nodes <= 0 {
		return errors.New("nodes must be greater than zero")
	}
	if opts.threshold <= 0 || opts.threshold >= opts.nodes {
		return fmt.Errorf("MPC threshold (%d) must be greater than zero and less than number of nodes (%d)", opts.threshold, opts.nodes)
	}
	if opts.environment == "" {
		opts.environment = "development"
	}
	if opts.natsURL == "" {
		opts.natsURL = "nats://nats-server:4222"
	}
	if opts.consulAddress == "" {
		opts.consulAddress = "consul:8500"
	}
	if opts.cliImage == "" {
		opts.cliImage = defaultMPCIUMCLIImage
	}
	for _, path := range env.RequiredConfigFiles {
		if err := requireFile(deps.workDir, path); err != nil {
			return fmt.Errorf("%w\nrun `fystack setup` or copy the matching .template file first", err)
		}
	}

	baseDir := filepath.Join(deps.workDir, "dev", "node-configs")
	if hasEntries(baseDir) && !opts.overwrite {
		fmt.Fprintln(deps.out, "dev node-configs already exist; skipping setup")
		return nil
	}
	if err := deps.runner.LookPath("docker"); err != nil {
		return fmt.Errorf("docker is required for mpcium identity generation: %w", err)
	}
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return err
	}

	fmt.Fprintln(deps.out, "[INFO] Pulling mpcium-cli Docker image...")
	if out, err := deps.runner.Run(ctx, deps.workDir, "docker", "pull", opts.cliImage); err != nil {
		writeMasked(deps.out, out)
		return fmt.Errorf("pull mpcium-cli image: %w", err)
	}

	if err := cleanNodeSetup(baseDir, opts); err != nil {
		return err
	}
	if err := generatePeers(ctx, deps, baseDir, opts); err != nil {
		return err
	}
	if err := writeClusterConfig(baseDir, opts); err != nil {
		return err
	}
	if err := configureEventInitiator(ctx, deps, baseDir, opts); err != nil {
		return err
	}
	if err := configureIntegritySigner(deps, baseDir, opts); err != nil {
		return err
	}
	if err := createNodeDirectories(baseDir, opts.nodes); err != nil {
		return err
	}
	if err := generateNodeIdentities(ctx, deps, baseDir, opts); err != nil {
		return err
	}
	if err := distributeNodeIdentities(baseDir, opts.nodes); err != nil {
		return err
	}
	if err := fixNodeFilePermissions(baseDir); err != nil {
		return err
	}

	fmt.Fprintln(deps.out, "[SUCCESS] MPCIUM node configuration generated")
	fmt.Fprintf(deps.out, "[INFO] Generated files in %s\n", relPath(deps.workDir, baseDir))
	fmt.Fprintln(deps.out, "[WARNING] Store the BadgerDB password from dev/node-configs/config.yaml securely")
	return nil
}

func cleanNodeSetup(baseDir string, opts nodeSetupOptions) error {
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "node") {
			if err := os.RemoveAll(filepath.Join(baseDir, entry.Name())); err != nil {
				return err
			}
		}
	}
	for _, name := range []string{"peers.json", "config.yaml"} {
		if err := removeIfExists(filepath.Join(baseDir, name)); err != nil {
			return err
		}
	}
	if opts.overwrite {
		for _, name := range []string{"event_initiator.identity.json", "event_initiator.key", "event_initiator.key.age", "integrity_signer.key"} {
			if err := removeIfExists(filepath.Join(baseDir, name)); err != nil {
				return err
			}
		}
	}
	return nil
}

func generatePeers(ctx context.Context, deps dependencies, baseDir string, opts nodeSetupOptions) error {
	fmt.Fprintln(deps.out, "[INFO] Generating peer configuration...")
	out, err := runMPCIUMCLI(ctx, deps, baseDir, opts.cliImage, "generate-peers", "-n", fmt.Sprint(opts.nodes), "-o", "peers.json")
	writeMasked(deps.out, out)
	if err != nil {
		return err
	}
	if err := requireFile(baseDir, "peers.json"); err != nil {
		return err
	}
	peers, err := readPeerIDs(filepath.Join(baseDir, "peers.json"))
	if err != nil {
		return err
	}
	for _, peer := range peers {
		fmt.Fprintf(deps.out, "[INFO] %s: %s\n", peer.name, peer.id)
	}
	return nil
}

func writeClusterConfig(baseDir string, opts nodeSetupOptions) error {
	password, err := randomPassword(32)
	if err != nil {
		return err
	}
	chainCode, err := randomHex(32)
	if err != nil {
		return err
	}
	content := fmt.Sprintf(`nats:
  url: %s

consul:
  address: %s

mpc_threshold: %d
environment: %s
badger_password: "%s"
event_initiator_pubkey: "PLACEHOLDER_WILL_BE_UPDATED"
chain_code: "%s"
db_path: "."
backup_enabled: true
backup_period_seconds: 300
backup_dir: backups
max_concurrent_keygen: 2
max_concurrent_signing: 10
session_warm_up_delay_ms: 500
`, opts.natsURL, opts.consulAddress, opts.threshold, opts.environment, password, chainCode)
	return os.WriteFile(filepath.Join(baseDir, "config.yaml"), []byte(content), 0644)
}

func configureEventInitiator(ctx context.Context, deps dependencies, baseDir string, opts nodeSetupOptions) error {
	mainConfig := filepath.Join(deps.workDir, "dev", "config.yaml")
	keyPath := filepath.Join(baseDir, "event_initiator.key")
	identityPath := filepath.Join(baseDir, "event_initiator.identity.json")
	existingKey, _ := yamlStringAt(mainConfig, "mpc", "signer", "local", "pk_raw")

	if !opts.overwrite && existingKey != "" && fileExists(keyPath) && fileExists(identityPath) {
		key, err := os.ReadFile(keyPath)
		if err != nil {
			return err
		}
		if strings.TrimSpace(string(key)) != existingKey {
			return errors.New("event initiator key does not match dev/config.yaml mpc.signer.local.pk_raw; rerun with --force to regenerate")
		}
		fmt.Fprintln(deps.out, "[INFO] Reusing existing event initiator")
	} else {
		args := []string{"generate-initiator", "--overwrite"}
		if opts.encryptKeys {
			args = append(args, "--encrypt")
		}
		fmt.Fprintln(deps.out, "[INFO] Generating event initiator...")
		out, err := runMPCIUMCLI(ctx, deps, baseDir, opts.cliImage, args...)
		writeMasked(deps.out, out)
		if err != nil {
			return err
		}
	}

	var identity struct {
		PublicKey string `json:"public_key"`
	}
	if err := readJSON(identityPath, &identity); err != nil {
		return err
	}
	if identity.PublicKey == "" {
		return errors.New("event initiator identity missing public_key")
	}
	if err := setYAMLStringAt(filepath.Join(baseDir, "config.yaml"), identity.PublicKey, "event_initiator_pubkey"); err != nil {
		return err
	}

	if opts.encryptKeys {
		return errors.New("encrypted event initiator keys are not supported by the dev config writer yet")
	}
	privateKey, err := os.ReadFile(keyPath)
	if err != nil {
		return err
	}
	return setYAMLStringAt(mainConfig, strings.TrimSpace(string(privateKey)), "mpc", "signer", "local", "pk_raw")
}

func configureIntegritySigner(deps dependencies, baseDir string, opts nodeSetupOptions) error {
	keyPath := filepath.Join(baseDir, "integrity_signer.key")
	key := ""
	if !opts.overwrite && fileExists(keyPath) {
		data, err := os.ReadFile(keyPath)
		if err != nil {
			return err
		}
		key = strings.TrimSpace(string(data))
		if len(key) != 64 {
			return errors.New("invalid existing integrity_signer.key length; rerun with --force to regenerate")
		}
		fmt.Fprintln(deps.out, "[INFO] Reusing existing integrity signer key")
	} else {
		generated, err := randomHex(32)
		if err != nil {
			return err
		}
		key = generated
		if err := os.WriteFile(keyPath, []byte(key), 0644); err != nil {
			return err
		}
	}
	return setYAMLStringAt(filepath.Join(deps.workDir, "dev", "config.yaml"), key, "integrity", "signer", "ed25519", "private_key")
}

func createNodeDirectories(baseDir string, nodes int) error {
	for i := 0; i < nodes; i++ {
		nodeDir := filepath.Join(baseDir, fmt.Sprintf("node%d", i))
		if err := os.MkdirAll(filepath.Join(nodeDir, "identity"), 0755); err != nil {
			return err
		}
		if err := copyFile(filepath.Join(baseDir, "config.yaml"), filepath.Join(nodeDir, "config.yaml"), 0644); err != nil {
			return err
		}
		if err := copyFile(filepath.Join(baseDir, "peers.json"), filepath.Join(nodeDir, "peers.json"), 0644); err != nil {
			return err
		}
	}
	return nil
}

func generateNodeIdentities(ctx context.Context, deps dependencies, baseDir string, opts nodeSetupOptions) error {
	for i := 0; i < opts.nodes; i++ {
		name := fmt.Sprintf("node%d", i)
		nodeDir := filepath.Join(baseDir, name)
		args := []string{"generate-identity", "--node", name}
		if opts.encryptKeys {
			args = append(args, "--encrypt")
		}
		fmt.Fprintf(deps.out, "[INFO] Generating identity for %s...\n", name)
		out, err := runMPCIUMCLI(ctx, deps, nodeDir, opts.cliImage, args...)
		writeMasked(deps.out, out)
		if err != nil {
			return err
		}
	}
	return nil
}

func distributeNodeIdentities(baseDir string, nodes int) error {
	for i := 0; i < nodes; i++ {
		sourceNode := fmt.Sprintf("node%d", i)
		source := filepath.Join(baseDir, sourceNode, "identity", sourceNode+"_identity.json")
		if err := requireFile(filepath.Join(baseDir, sourceNode, "identity"), sourceNode+"_identity.json"); err != nil {
			return err
		}
		for j := 0; j < nodes; j++ {
			if i == j {
				continue
			}
			target := filepath.Join(baseDir, fmt.Sprintf("node%d", j), "identity", sourceNode+"_identity.json")
			if err := copyFile(source, target, 0644); err != nil {
				return err
			}
		}
	}
	return nil
}

func fixNodeFilePermissions(baseDir string) error {
	return filepath.WalkDir(baseDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		return os.Chmod(path, 0644)
	})
}

func runMPCIUMCLI(ctx context.Context, deps dependencies, workDir, image string, args ...string) ([]byte, error) {
	user := fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid())
	dockerArgs := []string{"run", "--rm", "--user", user, "-e", "USER=nonroot", "-v", workDir + ":/data", "-w", "/data", image}
	dockerArgs = append(dockerArgs, args...)
	return deps.runner.Run(ctx, deps.workDir, "docker", dockerArgs...)
}

type peerID struct {
	name string
	id   string
}

func readPeerIDs(path string) ([]peerID, error) {
	var peers map[string]string
	if err := readJSON(path, &peers); err != nil {
		return nil, err
	}
	out := make([]peerID, 0, len(peers))
	for name, id := range peers {
		out = append(out, peerID{name: name, id: id})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].name < out[j].name
	})
	return out, nil
}

func readJSON(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

func randomPassword(length int) (string, error) {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	var buf strings.Builder
	buf.Grow(length)
	max := big.NewInt(int64(len(alphabet)))
	for buf.Len() < length {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		buf.WriteByte(alphabet[n.Int64()])
	}
	return buf.String(), nil
}

func randomHex(bytes int) (string, error) {
	data := make([]byte, bytes)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return hex.EncodeToString(data), nil
}

func copyFile(source, target string, mode fs.FileMode) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}
	return os.WriteFile(target, data, mode)
}

func removeIfExists(path string) error {
	err := os.RemoveAll(path)
	if err == nil || os.IsNotExist(err) {
		return nil
	}
	return err
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func relPath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return rel
}
