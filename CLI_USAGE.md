# Fystack CLI Usage Guide

The `fystack` CLI manages the entire self-host stack lifecycle: guided setup, MPC node initialization, Docker Compose operations, image version management, and clean resets.

---

## Installation

### Option 1 — Install binary (recommended)

```bash
go install github.com/fystack/fystack-selfhost-scripts/cmd/fystack@latest
```

This places the `fystack` binary in `$GOPATH/bin` (typically `~/go/bin`). Make sure that directory is on your `$PATH`:

```bash
export PATH="$PATH:$(go env GOPATH)/bin"
```

Verify:

```bash
fystack version
```

### Option 2 — Build from source

```bash
git clone git@github.com:fystack/fystack-selfhost-scripts.git
cd fystack-selfhost-scripts
go build -o fystack ./cmd/fystack
sudo mv fystack /usr/local/bin/
```

### Option 3 — Run directly with `go run`

From the repository root, prefix every command with `go run ./cmd/fystack`:

```bash
go run ./cmd/fystack <command>
```

All examples in this document use `fystack <command>`.

---

## Prerequisites

- Docker and Docker Compose are installed and running.
- Go 1.26 or newer is installed when building from source or using `go run`.
- You are logged in to the Fystack Labs Docker registry:

```bash
docker login -u fystacklabs
```

- A CoinMarketCap API key is needed only if you choose CoinMarketCap as the price provider. Binance does not require a key.
- On Windows, run the CLI inside WSL2.

---

## Fast Path

For a first local setup:

```bash
fystack setup
```

The guided setup:

- lets you choose the environment with arrow keys
- creates missing dev config files from templates
- asks whether to overwrite existing dev config files
- lets you choose Binance or CoinMarketCap as the price provider
- prompts for a CoinMarketCap API key only when CoinMarketCap is selected
- generates MPC node configs using the bundled `mpcium-cli` container
- asks whether to deploy the dev stack immediately

Interactive selections use Bubble Tea. Use arrow keys, `j`/`k`, or Enter to accept.

---

## Manual Path

Run each step individually:

```bash
fystack doctor --env dev
fystack init --env dev
fystack deploy --env dev
fystack status --env dev
```

After deployment, open the portal at `http://localhost:8015`.

---

## Global Flags

All commands accept:

```bash
--env dev     # default
--env prod
```

---

## Commands

### `setup`

Run the guided first-time setup:

```bash
fystack setup
```

Creates these files when missing:

```text
dev/config.yaml
dev/config.rescanner.yaml
dev/config.indexer.yaml
```

If any dev config files already exist, setup asks whether to overwrite them from the templates. If you choose CoinMarketCap and `dev/config.yaml` already has a key, pressing Enter keeps the current value.

---

### `doctor`

Check local prerequisites and required stack files:

```bash
fystack doctor --env dev
```

Checks:

- `stack.versions.yaml`
- the selected environment's Docker Compose file
- required dev config files
- Docker availability
- Docker Compose availability

Run this when setup fails or before deploying.

---

### `init`

Generate MPC node configuration files for the dev stack:

```bash
fystack init --env dev
```

Generates peer identities, a cluster config, and per-node identity material using the bundled `mpcium-cli` Docker image. If `dev/node-configs` already contains files the command skips generation to avoid overwriting existing node material.

To intentionally regenerate:

```bash
fystack init --env dev --force
```

---

### `deploy`

Deploy the selected Docker Compose stack:

```bash
fystack deploy --env dev
```

For `dev`, the CLI starts infrastructure services first, discovers generated MPC node configs, then starts the MPC services. If no node configs exist, run `fystack init --env dev` first.

The CLI writes `.fystack.compose.env` before running Docker Compose. This generated file contains image pins from `stack.versions.yaml`.

---

### `status`

Show Docker Compose service status:

```bash
fystack status --env dev
```

---

### `restart`

Restart selected Docker Compose services:

```bash
fystack restart --env dev
```

With no service names, an interactive checkbox list opens. Use Space to select, `a` to toggle all, Enter to restart.

Restart specific services directly:

```bash
fystack restart apex rescanner --env dev
fystack restart mpcium0 mpcium1 mpcium2 --env dev
```

---

### `logs`

Show recent Docker Compose logs:

```bash
fystack logs --env dev
fystack logs apex --env dev
fystack logs mpcium0 --tail 500 --env dev
```

The default tail is `200`.

---

### `check-updates`

Check Docker image tags for newer semver releases (read-only):

```bash
fystack check-updates --env dev
```

Reports available newer tags but does not rewrite `stack.versions.yaml` or restart services.

---

### `update`

Update pinned Docker image versions in `stack.versions.yaml`:

```bash
fystack update --env dev
```

With no service names, an interactive checkbox list of available app updates opens. After writing the selected pins, the CLI asks whether to deploy those updated services now.

Update every available semver-tagged service:

```bash
fystack update --env dev --all
```

Update and deploy immediately:

```bash
fystack update --env dev --all --deploy
```

Update specific services:

```bash
fystack update --env dev apex mpcium
```

`--all` and named-service updates are script-friendly: they update pins and print a deploy reminder without opening prompts unless `--deploy` is passed.

Infrastructure services (MongoDB, PostgreSQL, Redis, NATS, Consul) are intentionally excluded from app updates.

---

### `reset`

Remove all generated files to restore the working tree to a clean clone state:

```bash
fystack reset
```

Prompts for confirmation before removing:

```text
dev/config.yaml
dev/config.rescanner.yaml
dev/config.indexer.yaml
dev/node-configs/
.fystack.compose.env
```

Templates and source files are never touched. Skip the confirmation prompt for scripts:

```bash
fystack reset --force
```

After a reset, run `fystack setup` to start fresh.

---

### `version`

Print the CLI version:

```bash
fystack version
```

---

## Files Managed by the CLI

| File | Description |
|------|-------------|
| `dev/config.yaml` | Main Apex dev config. `setup` writes the CoinMarketCap API key and the event initiator private key here. |
| `dev/config.rescanner.yaml` | Rescanner dev config, copied from template when missing. |
| `dev/config.indexer.yaml` | Multichain indexer dev config, copied from template when missing. |
| `dev/node-configs/` | MPC node config directory generated by `init`. |
| `.fystack.compose.env` | Generated Compose environment file written by commands that run Docker Compose. |
| `stack.versions.yaml` | Source of Docker image pins. The `update` command rewrites this file. |

---

## Common Workflows

### First setup without immediate deploy

```bash
fystack setup           # choose "no" when asked to deploy
fystack doctor --env dev
fystack deploy --env dev
```

### Check the stack after deploy

```bash
fystack status --env dev
fystack logs --env dev
```

### Restart selected services

```bash
fystack restart --env dev
# or by name:
fystack restart apex rescanner --env dev
```

### Check and apply image updates

```bash
fystack check-updates --env dev
fystack update --env dev apex mpcium
fystack deploy --env dev
```

Or in one step:

```bash
fystack update --env dev --all --deploy
```

### Start fresh

```bash
fystack reset --force
fystack setup
```

---

## Troubleshooting

### Missing config files

If `doctor`, `init`, or `deploy` reports missing dev config files:

```bash
fystack setup
```

Or copy the templates manually:

```bash
cp ./dev/config.yaml.template ./dev/config.yaml
cp ./dev/config.rescanner.yaml.template ./dev/config.rescanner.yaml
cp ./dev/config.indexer.yaml.template ./dev/config.indexer.yaml
```

### No MPC node configs found

```bash
fystack init --env dev
fystack deploy --env dev
```

### Docker login or image pull failures

```bash
docker compose version
docker login -u fystacklabs
fystack doctor --env dev
fystack deploy --env dev
```

### Existing node configs are reused

`init` skips generation when `dev/node-configs` already has entries. Use `--force` to regenerate, or choose overwrite in guided setup. Only regenerate when you intentionally want new node material — existing key shares will be invalidated.
