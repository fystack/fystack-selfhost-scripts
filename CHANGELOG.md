# Release

## v0.1.5

Add `sender_name`, `sender_email` to `config.yaml`

```
email:
  resend_api_key: ""
  sender_name: "Fystack"
  sender_email: "noreply@fystack.io"
```

```sh
cd dev

docker compose pull migrate apex
docker compose up -d --no-deps --force-recreate migrate
docker compose up -d --no-deps --force-recreate apex
```

## v0.1.6

- remove `api_key`, `api_secret` from `config.yaml`

```
email:
  resend_api_key: "<API_KEY>"
```

## v0.1.7

### New MPC Signer Configuration Format

**Old format:**

```yaml
mpc:
  event_initiator_pk_raw: "ef8e3b8f43ef46a1433d71b8906ee80bb16c672a0545dd093b86fe44173c2085"
  event_initiator_encrypted_pk_path: ""
  event_initiator_encrypted_pk_password: ""
```

**New format:**

```yaml
mpc:
  signer:
    type: "local"
    local:
      pk_raw: "0a3ec46083c593a303391ea15331a2943010d3d3103a5e0add9ff5ac91dd0ad9"
      encrypted_pk_path: "event_initiator.key.age"
      encrypted_pk_password: ""
```

**Key differences:**

- Moved from flat structure to nested `signer` object
- Added `type` field to support multiple signer backends ("local" or "kms")
- Renamed `event_initiator_pk_raw` → `local.pk_raw`
- Renamed `event_initiator_encrypted_pk_path` → `local.encrypted_pk_path`
- Renamed `event_initiator_encrypted_pk_password` → `local.encrypted_pk_password`

### New Configuration Sections

Add the following new sections to your `config.yaml`:

```yaml
secret_store:
  type: "config"
  config:
    region: ap-southeast-1
    prefix: ""
    endpoint: http://localhost:4566
    access_key_id: test
    secret_access_key: test
  memguard_enclave:
    enabled: true
    encrypt_critical: true

address_risk_providers:
  webacy:
    api_key: "" # Optional can leave empty, Get free api key here: https://developers.webacy.co/, can

system_admins:
  - "devteam@fystack.io"

logging:
  mode: "stdout" # Options: "file", "stdout", "both"
  format: "json" # Options: "json", "pretty"
  blockchain:
    enabled: true
    stdout: true
  api:
    enabled: true
    stdout: true
  sweeper:
    enabled: true
    stdout: true
```

### New Configuration Sections (continued)

```yaml
ha:
  enabled: false # Set to true to enable HA functionality
  node_id: "" # Unique node identifier (auto-generated if empty)
  election:
    session_ttl: 10s # How long to keep session alive without renewal
    retry_interval: 5s # How often to attempt leadership election
    lock_delay: 1s # Delay after session destruction before re-acquisition
    stop_timeout: 30s # Max time to wait for services to stop gracefully

grace_shutdown_period: 15s # Time to wait for graceful shutdown before forcing termination

rate_limit:
  requests_per_minute: 300 # Number of requests allowed per minute per IP

telegram:
  bot_token: "" # Get token from @BotFather on Telegram
  long_polling: true
  bot_name: "@YourBotName"
```

**Creating a Telegram Bot using BotFather:**

1. Open Telegram and search for [@BotFather](https://t.me/botfather)
2. Start a chat and send `/newbot`
3. Choose a name for your bot (e.g., "Fystack Alert Bot")
4. Choose a username ending in 'bot' (e.g., "fystack_alert_bot")
5. Copy the provided token and paste it into `telegram.bot_token`
6. Update `telegram.bot_name` with your bot's username (e.g., "@fystack_alert_bot")

**Note:** Set `ha.enabled: false` for single-node deployments.

```sh
cd dev

docker compose pull migrate apex
docker compose up -d --no-deps --force-recreate migrate
docker compose up -d --no-deps --force-recreate apex
```

### New Service: Multichain Indexer

Added `multichain-indexer` service to docker-compose.yaml for indexing blockchain data.

**Configuration Setup:**

1. Copy the config template:

```sh
cd dev
cp config.indexer.yaml.template config.indexer.yaml
```

2. Add Tron Shasta testnet to `config.yaml` under `networks`:

```yaml
TRON_SHASTA_TESTNET:
  enabled: true
  rpc_nodes:
    - url: "https://api.shasta.trongrid.io/"
```

3. Start the multichain-indexer container:

```sh
docker compose up -d multichain-indexer
```

## v0.1.8

### New Integrity Signer Configuration

**⚠️ WARNING: MUST FOLLOW EVERY STEP EXACTLY ⚠️**

**Step 1: Pull latest images and migrate database**

```sh
cd dev

docker compose pull migrate apex
docker compose up -d --no-deps --force-recreate migrate
```

Wait for migration to complete successfully.

**Step 2: Update config.yaml**

Add the following new `integrity` section to your `config.yaml`:

```yaml
integrity:
  signer:
    version: 1
    type: "ed25519" # Options: "ed25519" or "kms"
    ed25519:
      private_key: "" # 32 byte ed25519 seed (64 hex characters)
```

**Generate the ed25519 private key:**

```sh
openssl rand -hex 32
```

Copy the output and paste it into the `private_key` field in the `integrity` section at the bottom of your `config.yaml`.

**Step 3: Run the one-time balance integrity migration**

This migration moves balance data from Consul to the database:

```sh
docker run --rm \
  --network dev_apex \
  -v "$(pwd)/config.yaml:/root/config.yaml:ro" \
  fystacklabs/balance-integrity-migrate:1.0.0
```

Wait for the migration to complete successfully.

**Step 4: Restart apex**

```sh
docker compose up -d --no-deps --force-recreate apex
```
