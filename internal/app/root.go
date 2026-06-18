package app

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/fystack/fystack-selfhost-scripts/internal/compose"
	"github.com/fystack/fystack-selfhost-scripts/internal/mask"
	"github.com/fystack/fystack-selfhost-scripts/internal/registry"
	"github.com/fystack/fystack-selfhost-scripts/internal/semver"
	"github.com/fystack/fystack-selfhost-scripts/internal/stack"
	"github.com/fystack/fystack-selfhost-scripts/internal/versions"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	Version = "dev"
	Commit  = "unknown"
)

type dependencies struct {
	workDir        string
	in             io.Reader
	out            io.Writer
	errOut         io.Writer
	runner         compose.Runner
	tagLister      registry.TagLister
	versionFile    string
	composeEnvFile string
}

type options struct {
	env string
}

func NewRootCommand() *cobra.Command {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	return newRootCommand(dependencies{
		workDir:        cwd,
		in:             os.Stdin,
		out:            os.Stdout,
		errOut:         os.Stderr,
		runner:         compose.OSRunner{},
		tagLister:      registry.RemoteTagLister{},
		versionFile:    "stack.versions.yaml",
		composeEnvFile: ".fystack.compose.env",
	})
}

func newRootCommand(deps dependencies) *cobra.Command {
	opts := &options{env: "dev"}
	root := &cobra.Command{
		Use:           "fystack",
		Short:         "Manage the Fystack self-host Docker stack",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringVar(&opts.env, "env", "dev", "stack environment: dev or prod")

	root.AddCommand(
		versionCommand(deps),
		doctorCommand(deps, opts),
		setupCommand(deps, opts),
		initCommand(deps, opts),
		resetCommand(deps, opts),
		deployCommand(deps, opts),
		restartCommand(deps, opts),
		statusCommand(deps, opts),
		logsCommand(deps, opts),
		checkUpdatesCommand(deps, opts),
		updateCommand(deps, opts),
	)

	return root
}

func versionCommand(deps dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print CLI version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintf(deps.out, "fystack %s (%s)\n", Version, Commit)
		},
	}
}

func setupCommand(deps dependencies, opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: "Run a guided setup for the self-host stack",
		RunE: func(cmd *cobra.Command, args []string) error {
			prompt := newPrompter(deps)
			prompt.banner()
			envName, err := prompt.selectOption("Select environment", []promptOption{
				{Value: "dev", Label: "dev - local Docker stack"},
				{Value: "prod", Label: "prod - production compose stack"},
			}, defaultEnvironmentIndex(opts.env))
			if err != nil {
				return err
			}
			opts.env = envName

			env, err := resolveEnv(deps, opts.env)
			if err != nil {
				return err
			}
			if env.Name != "dev" {
				return errors.New("guided setup is only implemented for --env dev")
			}

			overwriteConfigs := false
			if existing, err := existingDevConfigFiles(deps); err != nil {
				return err
			} else if len(existing) > 0 {
				overwriteConfigs, err = prompt.confirm("Overwrite existing dev config files from templates?", false)
				if err != nil {
					return err
				}
			}

			copied, skipped, overwritten, err := ensureDevConfigFiles(deps, overwriteConfigs)
			if err != nil {
				return err
			}
			for _, path := range copied {
				fmt.Fprintf(deps.out, "created %s\n", path)
			}
			for _, path := range overwritten {
				fmt.Fprintf(deps.out, "overwrote %s\n", path)
			}
			for _, path := range skipped {
				fmt.Fprintf(deps.out, "kept existing %s\n", path)
			}

			configPath := filepath.Join(deps.workDir, "dev", "config.yaml")
			priceProvider, err := prompt.selectOption("Select price provider", []promptOption{
				{Value: "binance", Label: "Binance - no API key required"},
				{Value: "coinmarketcap", Label: "CoinMarketCap - API key required"},
			}, 0)
			if err != nil {
				return err
			}
			if priceProvider == "coinmarketcap" {
				currentAPIKey, err := yamlStringAt(configPath, "price_providers", "coinmarketcap", "api_key")
				if err != nil {
					return err
				}
				question := "CoinMarketCap API key"
				if currentAPIKey != "" {
					question = "CoinMarketCap API key (leave empty to keep current value)"
				}
				apiKey, err := prompt.input(question, currentAPIKey == "")
				if err != nil {
					return err
				}
				if apiKey != "" {
					if err := setYAMLStringAt(configPath, apiKey, "price_providers", "coinmarketcap", "api_key"); err != nil {
						return err
					}
					fmt.Fprintln(deps.out, "updated dev/config.yaml")
				}
			} else {
				fmt.Fprintln(deps.out, "using Binance price provider; no API key needed")
			}

			generateNodes, err := prompt.confirm("Generate MPC node configs now?", true)
			if err != nil {
				return err
			}
			if generateNodes {
				overwriteNodes := false
				if hasEntries(filepath.Join(deps.workDir, "dev", "node-configs")) {
					overwriteNodes, err = prompt.confirm("Overwrite existing MPC node configs?", false)
					if err != nil {
						return err
					}
				}
				if err := runInit(cmd.Context(), deps, env, overwriteNodes); err != nil {
					return err
				}
			}

			deployNow, err := prompt.confirm("Deploy the dev stack now?", false)
			if err != nil {
				return err
			}
			if deployNow {
				if err := runDeploy(cmd.Context(), deps, env, opts.env); err != nil {
					return err
				}
			}

			fmt.Fprintln(deps.out, "setup complete")
			return nil
		},
	}
}

func doctorCommand(deps dependencies, opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check local prerequisites and stack files",
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := resolveEnv(deps, opts.env)
			if err != nil {
				return err
			}

			checks := []string{
				deps.versionFile,
				env.ComposeFile,
			}
			for _, path := range checks {
				if err := requireFile(deps.workDir, path); err != nil {
					return err
				}
			}
			if env.Name == "dev" {
				for _, path := range env.RequiredConfigFiles {
					if err := requireFile(deps.workDir, path); err != nil {
						return fmt.Errorf("%w\nrun `fystack setup` or copy the matching .template file first", err)
					}
				}
			}
			if err := deps.runner.LookPath("docker"); err != nil {
				return fmt.Errorf("docker is required: %w", err)
			}
			if _, err := deps.runner.Run(cmd.Context(), deps.workDir, "docker", "compose", "version"); err != nil {
				return fmt.Errorf("docker compose is required: %w", err)
			}
			fmt.Fprintf(deps.out, "doctor ok for %s\n", env.Name)
			return nil
		},
	}
}

func initCommand(deps dependencies, opts *options) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Generate MPC node configs for the dev stack",
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := resolveEnv(deps, opts.env)
			if err != nil {
				return err
			}
			if env.Name != "dev" {
				return errors.New("init is only implemented for --env dev")
			}
			return runInit(cmd.Context(), deps, env, force)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "overwrite existing dev node configs")
	return cmd
}

func resetCommand(deps dependencies, _ *options) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "reset",
		Short: "Remove generated files to restore the repository to a clean state",
		Long: `Removes files generated by setup/init so the working tree matches
a fresh clone. Specifically removes:
  dev/config.yaml
  dev/config.rescanner.yaml
  dev/config.indexer.yaml
  dev/node-configs/
  .fystack.compose.env

Templates and source files are never touched.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !force {
				prompt := newPrompter(deps)
				ok, err := prompt.confirm("Remove all generated dev config and node files?", false)
				if err != nil {
					return err
				}
				if !ok {
					fmt.Fprintln(deps.out, "reset canceled")
					return nil
				}
			}
			return runReset(deps)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "skip confirmation prompt")
	return cmd
}

func runReset(deps dependencies) error {
	targets := []string{
		filepath.Join(deps.workDir, "dev", "config.yaml"),
		filepath.Join(deps.workDir, "dev", "config.rescanner.yaml"),
		filepath.Join(deps.workDir, "dev", "config.indexer.yaml"),
		filepath.Join(deps.workDir, "dev", "node-configs"),
		filepath.Join(deps.workDir, deps.composeEnvFile),
	}
	for _, path := range targets {
		if err := removeIfExists(path); err != nil {
			return fmt.Errorf("remove %s: %w", path, err)
		}
		rel, _ := filepath.Rel(deps.workDir, path)
		fmt.Fprintf(deps.out, "removed %s\n", rel)
	}
	fmt.Fprintln(deps.out, "reset complete")
	return nil
}

func deployCommand(deps dependencies, opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "deploy",
		Short: "Deploy the selected Docker Compose stack",
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := resolveEnv(deps, opts.env)
			if err != nil {
				return err
			}
			return runDeploy(cmd.Context(), deps, env, opts.env)
		},
	}
}

func restartCommand(deps dependencies, opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "restart [service...]",
		Short: "Restart selected Docker Compose services",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := resolveEnv(deps, opts.env)
			if err != nil {
				return err
			}
			if err := writeComposeEnv(deps, opts.env); err != nil {
				return err
			}

			services := args
			if len(services) == 0 {
				available, err := composeServices(filepath.Join(deps.workDir, env.ComposeFile))
				if err != nil {
					return err
				}
				if len(available) == 0 {
					return errors.New("no compose services found")
				}
				prompt := newPrompter(deps)
				services, err = prompt.selectValues("Select services to restart", available)
				if err != nil {
					return err
				}
				if len(services) == 0 {
					fmt.Fprintln(deps.out, "no services selected")
					return nil
				}
			}

			out, err := runCompose(cmd.Context(), deps, env, append([]string{"restart"}, services...)...)
			writeMasked(deps.out, out)
			return err
		},
	}
}

func statusCommand(deps dependencies, opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show Docker Compose service status",
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := resolveEnv(deps, opts.env)
			if err != nil {
				return err
			}
			if err := writeComposeEnv(deps, opts.env); err != nil {
				return err
			}
			out, err := runCompose(cmd.Context(), deps, env, "ps")
			writeMasked(deps.out, out)
			return err
		},
	}
}

func logsCommand(deps dependencies, opts *options) *cobra.Command {
	var tail string
	cmd := &cobra.Command{
		Use:   "logs [service]",
		Short: "Show Docker Compose logs",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := resolveEnv(deps, opts.env)
			if err != nil {
				return err
			}
			if err := writeComposeEnv(deps, opts.env); err != nil {
				return err
			}
			composeArgs := []string{"logs", "--tail", tail}
			if len(args) == 1 {
				composeArgs = append(composeArgs, args[0])
			}
			out, err := runCompose(cmd.Context(), deps, env, composeArgs...)
			writeMasked(deps.out, out)
			return err
		},
	}
	cmd.Flags().StringVar(&tail, "tail", "200", "number of log lines to print")
	return cmd
}

func checkUpdatesCommand(deps dependencies, opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "check-updates",
		Short: "Check app Docker image tags for newer semver releases",
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := resolveEnv(deps, opts.env)
			if err != nil {
				return err
			}
			results, err := findImageUpdates(cmd.Context(), deps, env.Name)
			if err != nil {
				return err
			}
			results = appImageUpdates(results)
			if len(results) == 0 {
				fmt.Fprintf(deps.out, "no services configured for %s\n", env.Name)
				return nil
			}
			for _, result := range results {
				if result.Err != nil {
					fmt.Fprintf(deps.out, "%s: update check failed: %v\n", result.Service.Name, result.Err)
					continue
				}
				if result.Skipped {
					fmt.Fprintf(deps.out, "%s: skipped non-semver tag %q\n", result.Service.Name, result.Current)
					continue
				}
				if !result.Available {
					fmt.Fprintf(deps.out, "%s: current %s\n", result.Service.Name, result.Current)
					continue
				}
				fmt.Fprintf(deps.out, "%s: update available %s -> %s\n", result.Service.Name, result.Current, result.Latest)
			}
			return nil
		},
	}
}

func updateCommand(deps dependencies, opts *options) *cobra.Command {
	var all bool
	var deploy bool
	cmd := &cobra.Command{
		Use:   "update [service...]",
		Short: "Update pinned app Docker image versions",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := resolveEnv(deps, opts.env)
			if err != nil {
				return err
			}
			results, err := findImageUpdates(cmd.Context(), deps, env.Name)
			if err != nil {
				return err
			}
			available := availableUpdates(results)
			if len(available) == 0 {
				fmt.Fprintln(deps.out, "no semver updates available")
				return nil
			}

			selected, prompted, err := selectUpdates(deps, available, all, args)
			if err != nil {
				return err
			}
			if len(selected) == 0 {
				fmt.Fprintln(deps.out, "no updates selected")
				return nil
			}

			updates := make(map[string]string, len(selected))
			for _, update := range selected {
				updates[update.Service.Name] = update.LatestImage
			}
			if err := versions.UpdateImages(filepath.Join(deps.workDir, deps.versionFile), updates); err != nil {
				return err
			}
			for _, update := range selected {
				fmt.Fprintf(deps.out, "updated %s: %s -> %s\n", update.Service.Name, update.Current, update.Latest)
			}

			if !deploy && prompted {
				prompt := newPrompter(deps)
				deploy, err = prompt.confirm("Deploy updated images now?", false)
				if err != nil {
					return err
				}
			}
			if deploy {
				return runDeployUpdates(cmd.Context(), deps, env, opts.env, selected)
			}
			fmt.Fprintln(deps.out, "run `fystack deploy` when you are ready to apply the new pins")
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "update all services with semver updates")
	cmd.Flags().BoolVar(&deploy, "deploy", false, "deploy after updating version pins")
	return cmd
}

type imageUpdate struct {
	Service     versions.Service
	Current     string
	Latest      string
	LatestImage string
	Available   bool
	Skipped     bool
	Err         error
}

func findImageUpdates(ctx context.Context, deps dependencies, envName string) ([]imageUpdate, error) {
	defs, err := loadVersions(deps)
	if err != nil {
		return nil, err
	}
	services := defs.ForEnvironment(envName)
	results := make([]imageUpdate, 0, len(services))
	for _, svc := range services {
		current, ok := registry.ImageTag(svc.Image)
		result := imageUpdate{Service: svc, Current: current}
		if !ok || !semver.Valid(current) {
			result.Skipped = true
			results = append(results, result)
			continue
		}
		tags, err := deps.tagLister.Tags(ctx, svc.Image)
		if err != nil {
			result.Err = err
			results = append(results, result)
			continue
		}
		latest := semver.Latest(tags)
		result.Latest = latest
		if latest != "" && semver.Compare(latest, current) > 0 {
			result.Available = true
			result.LatestImage = replaceImageTag(svc.Image, latest)
		}
		results = append(results, result)
	}
	return results, nil
}

func availableUpdates(results []imageUpdate) []imageUpdate {
	available := make([]imageUpdate, 0, len(results))
	for _, result := range appImageUpdates(results) {
		if result.Available {
			available = append(available, result)
		}
	}
	return available
}

func appImageUpdates(results []imageUpdate) []imageUpdate {
	apps := make([]imageUpdate, 0, len(results))
	for _, result := range results {
		if !isInfrastructureService(result.Service.Name) {
			apps = append(apps, result)
		}
	}
	return apps
}

func isInfrastructureService(name string) bool {
	switch name {
	case "mongo-dev", "mongo-prod", "postgres", "redis", "nats-dev", "nats-prod", "consul":
		return true
	default:
		return false
	}
}

func selectUpdates(deps dependencies, available []imageUpdate, all bool, args []string) ([]imageUpdate, bool, error) {
	if all {
		return available, false, nil
	}
	if len(args) > 0 {
		byName := make(map[string]imageUpdate, len(available))
		for _, update := range available {
			byName[update.Service.Name] = update
		}
		selected := make([]imageUpdate, 0, len(args))
		for _, name := range args {
			update, ok := byName[name]
			if !ok {
				return nil, false, fmt.Errorf("no semver update available for %s", name)
			}
			selected = append(selected, update)
		}
		return selected, false, nil
	}
	prompt := newPrompter(deps)
	selected, err := prompt.selectUpdates("Select app updates to apply", available)
	return selected, true, err
}

func replaceImageTag(image, tag string) string {
	idx := strings.LastIndex(image, ":")
	if idx < 0 || idx == len(image)-1 {
		return image + ":" + tag
	}
	if slash := strings.LastIndex(image, "/"); slash > idx {
		return image + ":" + tag
	}
	return image[:idx+1] + tag
}

func runInit(ctx context.Context, deps dependencies, env stack.Environment, force bool) error {
	opts := defaultNodeSetupOptions(force)
	return runNodeSetup(ctx, deps, env, opts)
}

func runDeploy(ctx context.Context, deps dependencies, env stack.Environment, envName string) error {
	if err := writeComposeEnv(deps, envName); err != nil {
		return err
	}
	if env.Name == "dev" {
		out, err := runCompose(ctx, deps, env, append([]string{"up", "-d"}, env.DevInfrastructureServices...)...)
		writeMasked(deps.out, out)
		if err != nil {
			return err
		}
		mpcServices := discoverMPCServices(filepath.Join(deps.workDir, "dev", "node-configs"))
		if len(mpcServices) == 0 {
			return errors.New("no MPCIUM node configs found; run `fystack init --env dev` first")
		}
		out, err = runCompose(ctx, deps, env, append([]string{"up", "-d"}, mpcServices...)...)
		writeMasked(deps.out, out)
		return err
	}
	out, err := runCompose(ctx, deps, env, "up", "-d")
	writeMasked(deps.out, out)
	return err
}

func runDeployUpdates(ctx context.Context, deps dependencies, env stack.Environment, envName string, updates []imageUpdate) error {
	if err := writeComposeEnv(deps, envName); err != nil {
		return err
	}
	services, err := composeServicesForUpdates(deps, env, updates)
	if err != nil {
		return err
	}
	if len(services) == 0 {
		return errors.New("no compose services found for selected updates")
	}
	fmt.Fprintf(deps.out, "deploying updated services: %s\n", strings.Join(services, ", "))
	out, err := runCompose(ctx, deps, env, append([]string{"up", "-d"}, services...)...)
	writeMasked(deps.out, out)
	return err
}

func composeServicesForUpdates(deps dependencies, env stack.Environment, updates []imageUpdate) ([]string, error) {
	available, err := composeServices(filepath.Join(deps.workDir, env.ComposeFile))
	if err != nil {
		return nil, err
	}
	availableSet := make(map[string]bool, len(available))
	for _, service := range available {
		availableSet[service] = true
	}

	selectedSet := make(map[string]bool, len(updates))
	for _, update := range updates {
		name := update.Service.Name
		if name == "apex-migrate" {
			name = "migrate"
		}
		if name == "mpcium" {
			for _, service := range discoverMPCServices(filepath.Join(deps.workDir, "dev", "node-configs")) {
				if availableSet[service] {
					selectedSet[service] = true
				}
			}
			for _, service := range available {
				if strings.HasPrefix(service, "mpcium") {
					selectedSet[service] = true
				}
			}
			continue
		}
		if availableSet[name] {
			selectedSet[name] = true
		}
	}

	services := make([]string, 0, len(selectedSet))
	for service := range selectedSet {
		services = append(services, service)
	}
	sort.Slice(services, func(i, j int) bool {
		return semver.NaturalLess(services[i], services[j])
	})
	return services, nil
}

func resolveEnv(deps dependencies, name string) (stack.Environment, error) {
	env, err := stack.Resolve(name)
	if err != nil {
		return stack.Environment{}, err
	}
	if err := requireFile(deps.workDir, env.ComposeFile); err != nil {
		return stack.Environment{}, err
	}
	return env, nil
}

func runCompose(ctx context.Context, deps dependencies, env stack.Environment, args ...string) ([]byte, error) {
	envFile := filepath.Join(deps.workDir, deps.composeEnvFile)
	composeArgs := append([]string{"compose", "--env-file", envFile, "-f", "docker-compose.yaml"}, args...)
	return deps.runner.Run(ctx, filepath.Join(deps.workDir, env.WorkDir), "docker", composeArgs...)
}

func writeComposeEnv(deps dependencies, envName string) error {
	defs, err := loadVersions(deps)
	if err != nil {
		return err
	}
	content := defs.EnvFile(envName)
	path := filepath.Join(deps.workDir, deps.composeEnvFile)
	return os.WriteFile(path, []byte(content), 0644)
}

func loadVersions(deps dependencies) (versions.Definitions, error) {
	return versions.Load(filepath.Join(deps.workDir, deps.versionFile))
}

func composeServices(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	root := yamlNodeAt(&doc)
	servicesNode := yamlNodeAt(root, "services")
	if servicesNode == nil || servicesNode.Kind != yaml.MappingNode {
		return nil, nil
	}
	services := make([]string, 0, len(servicesNode.Content)/2)
	for i := 0; i+1 < len(servicesNode.Content); i += 2 {
		services = append(services, servicesNode.Content[i].Value)
	}
	sort.Strings(services)
	return services, nil
}

func requireFile(root, rel string) error {
	path := filepath.Join(root, rel)
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("required file missing: %s", rel)
	}
	if info.IsDir() {
		return fmt.Errorf("required file is a directory: %s", rel)
	}
	return nil
}

func hasEntries(path string) bool {
	entries, err := os.ReadDir(path)
	return err == nil && len(entries) > 0
}

func discoverMPCServices(nodeConfigDir string) []string {
	entries, err := os.ReadDir(nodeConfigDir)
	if err != nil {
		return nil
	}
	services := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "node") {
			continue
		}
		index := strings.TrimPrefix(entry.Name(), "node")
		if index == "" {
			continue
		}
		services = append(services, "mpcium"+index)
	}
	sort.Slice(services, func(i, j int) bool {
		return semver.NaturalLess(strings.TrimPrefix(services[i], "mpcium"), strings.TrimPrefix(services[j], "mpcium"))
	})
	return services
}

func writeMasked(w io.Writer, out []byte) {
	if len(out) == 0 {
		return
	}
	fmt.Fprint(w, mask.Sensitive(string(out)))
}

type promptOption struct {
	Value string
	Label string
}

type prompter struct {
	deps   dependencies
	reader *bufio.Reader
	colors bool
}

func newPrompter(deps dependencies) *prompter {
	in := deps.in
	if in == nil {
		in = os.Stdin
	}
	deps.in = in
	outFile, _ := deps.out.(*os.File)
	colors := outFile != nil && isInteractive(outFile) && os.Getenv("NO_COLOR") == ""
	return &prompter{deps: deps, reader: bufio.NewReader(in), colors: colors}
}

func (p *prompter) banner() {
	lines := strings.Split(strings.TrimRight(setupBanner, "\n"), "\n")
	brand := setupBanner
	if p.colors {
		styled := make([]string, 0, len(lines))
		for i, line := range lines {
			switch {
			case i < 2:
				styled = append(styled, bannerTopStyle.Render(line))
			case i < 4:
				styled = append(styled, bannerMidStyle.Render(line))
			case i == 4:
				styled = append(styled, bannerBottomStyle.Render(line))
			case i == 5:
				styled = append(styled, bannerShadowStyle.Render(line))
			default:
				styled = append(styled, bannerCaptionStyle.Render(line))
			}
		}
		brand = strings.Join(styled, "\n")
	}
	fmt.Fprintf(p.deps.out, "%s\n\n", brand)
}

func (p *prompter) confirm(question string, defaultYes bool) (bool, error) {
	options := []promptOption{
		{Value: "no", Label: "no"},
		{Value: "yes", Label: "yes"},
	}
	if defaultYes {
		options = []promptOption{
			{Value: "yes", Label: "yes"},
			{Value: "no", Label: "no"},
		}
	}
	value, err := p.selectOption(question, options, 0)
	if err != nil {
		return false, err
	}
	return value == "yes", nil
}

func (p *prompter) selectOption(question string, options []promptOption, selected int) (string, error) {
	if len(options) == 0 {
		return "", errors.New("prompt has no options")
	}
	if selected < 0 || selected >= len(options) {
		selected = 0
	}
	if input, ok := p.deps.in.(*os.File); ok && isInteractive(input) {
		return p.selectOptionInteractive(input, question, options, selected)
	}

	fmt.Fprintln(p.deps.out, p.paint(questionStyle, question))
	for i, option := range options {
		fmt.Fprintf(p.deps.out, "  %d. %s\n", i+1, option.Label)
	}
	fmt.Fprintf(p.deps.out, "%s [%s]: ", p.paint(accentStyle, "Choose"), options[selected].Value)
	line, err := p.reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	answer := strings.TrimSpace(strings.ToLower(line))
	if answer == "" {
		return options[selected].Value, nil
	}
	for i, option := range options {
		if answer == option.Value || answer == strings.ToLower(option.Label) || answer == fmt.Sprint(i+1) {
			return option.Value, nil
		}
	}
	return "", fmt.Errorf("invalid choice %q", strings.TrimSpace(line))
}

func (p *prompter) selectOptionInteractive(input *os.File, question string, options []promptOption, selected int) (string, error) {
	model := selectModel{
		question: question,
		options:  options,
		selected: selected,
		colors:   p.colors,
	}
	program := tea.NewProgram(model, tea.WithInput(input), tea.WithOutput(p.deps.out))
	result, err := program.Run()
	if err != nil {
		return "", err
	}
	final, ok := result.(selectModel)
	if !ok {
		return "", errors.New("unexpected prompt model result")
	}
	if final.canceled {
		return "", errors.New("setup canceled")
	}
	return options[final.selected].Value, nil
}

func (p *prompter) selectUpdates(question string, updates []imageUpdate) ([]imageUpdate, error) {
	options := make([]multiSelectOption, 0, len(updates))
	for _, update := range updates {
		options = append(options, multiSelectOption{
			Value: update.Service.Name,
			Label: fmt.Sprintf("%s %s -> %s", update.Service.Name, update.Current, update.Latest),
		})
	}
	selected, err := p.selectMulti(question, options, "all, none, or numbers")
	if err != nil {
		return nil, err
	}
	byName := make(map[string]imageUpdate, len(updates))
	for _, update := range updates {
		byName[update.Service.Name] = update
	}
	out := make([]imageUpdate, 0, len(selected))
	for _, value := range selected {
		out = append(out, byName[value])
	}
	return out, nil
}

func (p *prompter) selectValues(question string, values []string) ([]string, error) {
	options := make([]multiSelectOption, 0, len(values))
	for _, value := range values {
		options = append(options, multiSelectOption{Value: value, Label: value})
	}
	return p.selectMulti(question, options, "all, none, or numbers")
}

func (p *prompter) selectMulti(question string, options []multiSelectOption, promptHint string) ([]string, error) {
	if input, ok := p.deps.in.(*os.File); ok && isInteractive(input) {
		return p.selectMultiInteractive(input, question, options)
	}

	fmt.Fprintln(p.deps.out, p.paint(questionStyle, question))
	for i, option := range options {
		fmt.Fprintf(p.deps.out, "  %d. %s\n", i+1, option.Label)
	}
	fmt.Fprintf(p.deps.out, "%s [%s]: ", p.paint(accentStyle, "Choose"), promptHint)
	line, err := p.reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	answer := strings.TrimSpace(strings.ToLower(line))
	if answer == "" || answer == "none" {
		return nil, nil
	}
	if answer == "all" {
		selected := make([]string, 0, len(options))
		for _, option := range options {
			selected = append(selected, option.Value)
		}
		return selected, nil
	}
	fields := strings.FieldsFunc(answer, func(r rune) bool {
		return r == ',' || r == ' '
	})
	selected := make([]string, 0, len(fields))
	for _, field := range fields {
		var index int
		if _, err := fmt.Sscanf(field, "%d", &index); err != nil || index < 1 || index > len(options) {
			return nil, fmt.Errorf("invalid selection %q", field)
		}
		selected = append(selected, options[index-1].Value)
	}
	return selected, nil
}

func (p *prompter) selectMultiInteractive(input *os.File, question string, options []multiSelectOption) ([]string, error) {
	model := multiSelectModel{
		question: question,
		options:  options,
		checked:  make(map[int]bool, len(options)),
		colors:   p.colors,
	}
	program := tea.NewProgram(model, tea.WithInput(input), tea.WithOutput(p.deps.out))
	result, err := program.Run()
	if err != nil {
		return nil, err
	}
	final, ok := result.(multiSelectModel)
	if !ok {
		return nil, errors.New("unexpected prompt model result")
	}
	if final.canceled {
		return nil, errors.New("selection canceled")
	}
	selected := make([]string, 0, len(final.checked))
	for i, option := range options {
		if final.checked[i] {
			selected = append(selected, option.Value)
		}
	}
	return selected, nil
}

type selectModel struct {
	question string
	options  []promptOption
	selected int
	colors   bool
	canceled bool
}

func (m selectModel) Init() tea.Cmd {
	return nil
}

func (m selectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "ctrl+c", "esc":
		m.canceled = true
		return m, tea.Quit
	case "up", "k":
		m.selected = (m.selected + len(m.options) - 1) % len(m.options)
	case "down", "j":
		m.selected = (m.selected + 1) % len(m.options)
	case "enter", " ":
		return m, tea.Quit
	}
	return m, nil
}

func (m selectModel) View() string {
	var buf strings.Builder
	buf.WriteString(renderMaybe(m.colors, questionStyle, m.question))
	buf.WriteString("\n")
	for i, option := range m.options {
		prefix := "  "
		label := renderMaybe(m.colors, dimStyle, option.Label)
		if i == m.selected {
			prefix = renderMaybe(m.colors, selectedStyle, "> ")
			label = renderMaybe(m.colors, selectedStyle, option.Label)
		}
		buf.WriteString(prefix)
		buf.WriteString(label)
		buf.WriteString("\n")
	}
	buf.WriteString(renderMaybe(m.colors, dimStyle, "Use ↑/↓, j/k, enter to select. Esc cancels."))
	buf.WriteString("\n")
	return buf.String()
}

type multiSelectModel struct {
	question string
	options  []multiSelectOption
	selected int
	checked  map[int]bool
	colors   bool
	canceled bool
}

type multiSelectOption struct {
	Value string
	Label string
}

func (m multiSelectModel) Init() tea.Cmd {
	return nil
}

func (m multiSelectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "ctrl+c", "esc":
		m.canceled = true
		return m, tea.Quit
	case "up", "k":
		m.selected = (m.selected + len(m.options) - 1) % len(m.options)
	case "down", "j":
		m.selected = (m.selected + 1) % len(m.options)
	case " ":
		m.checked[m.selected] = !m.checked[m.selected]
	case "a":
		allChecked := len(m.checked) == len(m.options)
		for i := range m.options {
			m.checked[i] = !allChecked
		}
	case "enter":
		return m, tea.Quit
	}
	return m, nil
}

func (m multiSelectModel) View() string {
	var buf strings.Builder
	buf.WriteString(renderMaybe(m.colors, questionStyle, m.question))
	buf.WriteString("\n")
	for i, option := range m.options {
		prefix := "  "
		check := "[ ]"
		if m.checked[i] {
			check = "[x]"
		}
		label := fmt.Sprintf("%s %s", check, option.Label)
		label = renderMaybe(m.colors, dimStyle, label)
		if i == m.selected {
			prefix = renderMaybe(m.colors, selectedStyle, "> ")
			label = renderMaybe(m.colors, selectedStyle, label)
		}
		buf.WriteString(prefix)
		buf.WriteString(label)
		buf.WriteString("\n")
	}
	buf.WriteString(renderMaybe(m.colors, dimStyle, "Space toggles, a toggles all, enter applies. Esc cancels."))
	buf.WriteString("\n")
	return buf.String()
}

func (p *prompter) input(question string, required bool) (string, error) {
	for {
		fmt.Fprintf(p.deps.out, "%s: ", p.paint(questionStyle, question))
		line, err := p.reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", err
		}
		value := strings.TrimSpace(line)
		if value != "" || !required {
			return value, nil
		}
		fmt.Fprintln(p.deps.out, p.paint(warningStyle, "value is required"))
	}
}

var (
	bannerTopStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7EA0FF"))
	bannerMidStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#4F7DEF"))
	bannerBottomStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#315EDC"))
	bannerShadowStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#2447A8"))
	bannerCaptionStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#4F7DEF"))
	questionStyle      = lipgloss.NewStyle().Bold(true)
	accentStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("#4F7DEF"))
	dimStyle           = lipgloss.NewStyle().Faint(true)
	selectedStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#4F7DEF"))
	warningStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
)

func (p *prompter) paint(style lipgloss.Style, text string) string {
	if !p.colors {
		return text
	}
	return style.Render(text)
}

func renderMaybe(colors bool, style lipgloss.Style, text string) string {
	if !colors {
		return text
	}
	return style.Render(text)
}

func isInteractive(input *os.File) bool {
	info, err := input.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func defaultEnvironmentIndex(name string) int {
	if name == "prod" {
		return 1
	}
	return 0
}

type devConfigFile struct {
	Template string
	Target   string
}

var devConfigFiles = []devConfigFile{
	{Template: "dev/config.yaml.template", Target: "dev/config.yaml"},
	{Template: "dev/config.rescanner.yaml.template", Target: "dev/config.rescanner.yaml"},
	{Template: "dev/config.indexer.yaml.template", Target: "dev/config.indexer.yaml"},
}

func existingDevConfigFiles(deps dependencies) ([]string, error) {
	var existing []string
	for _, file := range devConfigFiles {
		targetPath := filepath.Join(deps.workDir, file.Target)
		if _, err := os.Stat(targetPath); err == nil {
			existing = append(existing, file.Target)
		} else if !os.IsNotExist(err) {
			return nil, err
		}
	}
	return existing, nil
}

func ensureDevConfigFiles(deps dependencies, overwrite bool) ([]string, []string, []string, error) {
	var copied []string
	var skipped []string
	var overwritten []string
	for _, file := range devConfigFiles {
		targetPath := filepath.Join(deps.workDir, file.Target)
		if _, err := os.Stat(targetPath); err == nil {
			if !overwrite {
				skipped = append(skipped, file.Target)
				continue
			}
			overwritten = append(overwritten, file.Target)
		} else if !os.IsNotExist(err) {
			return nil, nil, nil, err
		}

		templatePath := filepath.Join(deps.workDir, file.Template)
		data, err := os.ReadFile(templatePath)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("read %s: %w", file.Template, err)
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return nil, nil, nil, err
		}
		if err := os.WriteFile(targetPath, data, 0644); err != nil {
			return nil, nil, nil, err
		}
		if !overwrite || !slices.Contains(overwritten, file.Target) {
			copied = append(copied, file.Target)
		}
	}
	return copied, skipped, overwritten, nil
}


func yamlStringAt(path string, keys ...string) (string, error) {
	node, err := loadYAMLDocument(path)
	if err != nil {
		return "", err
	}
	value := yamlNodeAt(node, keys...)
	if value == nil {
		return "", fmt.Errorf("missing yaml value %s in %s", strings.Join(keys, "."), path)
	}
	return value.Value, nil
}

func setYAMLStringAt(path, value string, keys ...string) error {
	node, err := loadYAMLDocument(path)
	if err != nil {
		return err
	}
	target := yamlNodeAt(node, keys...)
	if target == nil {
		return fmt.Errorf("missing yaml value %s in %s", strings.Join(keys, "."), path)
	}
	target.Kind = yaml.ScalarNode
	target.Tag = "!!str"
	target.Value = value

	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(node); err != nil {
		_ = encoder.Close()
		return err
	}
	if err := encoder.Close(); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0644)
}

func loadYAMLDocument(path string) (*yaml.Node, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return nil, err
	}
	return &node, nil
}

func yamlNodeAt(node *yaml.Node, keys ...string) *yaml.Node {
	if node == nil {
		return nil
	}
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		node = node.Content[0]
	}
	for _, key := range keys {
		if node.Kind != yaml.MappingNode {
			return nil
		}
		var next *yaml.Node
		for i := 0; i+1 < len(node.Content); i += 2 {
			if node.Content[i].Value == key {
				next = node.Content[i+1]
				break
			}
		}
		if next == nil {
			return nil
		}
		node = next
	}
	return node
}
