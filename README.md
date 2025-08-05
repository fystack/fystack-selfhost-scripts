# MPCIUM Quick Start Guide

Welcome to MPCIUM! This guide will help you get your MPC (Multi-Party Computation) cluster up and running in minutes.

## Prerequisites

- Docker and Docker Compose installed
- Bash shell
- Internet connection

## Quick Start Steps

### 1. Docker Login

First, authenticate with the Fystack Labs Docker registry:

```bash
docker login -u fystacklabs
```

When prompted, enter your password.

### 2. Make Setup Script Executable

Make the node configuration script executable:

```bash
chmod +x ./dev/node-configs/setup-nodes.sh
```

### 3. Generate Node Configurations

Run the setup script to generate all necessary configurations:

```bash
./dev/node-configs/setup-nodes.sh
```

This script will:

- Generate 3 MPC nodes with 2-of-3 threshold
- Create all necessary identity files and configurations
- Set up the required directory structure

### 4. Start the MPC Cluster

Launch all services using Docker Compose:

```bash
docker-compose -f ./dev/docker-compose.yaml up -d
```

This will start:

- **NATS messaging server** (port 4222)
- **Consul service discovery** (port 8500)
- **3 MPC nodes** (node0, node1, node2)
- **Automatic peer registration**

### 5. Verify the Setup

Check that all services are running:

```bash
docker-compose -f ./dev/docker-compose.yaml ps
```

You should see all services in the "Up" state.

### 6. View Logs (Optional)

Monitor the cluster logs:

```bash
# View all service logs
docker-compose -f ./dev/docker-compose.yaml logs -f

# View logs from a specific node
docker-compose -f ./dev/docker-compose.yaml logs -f mpcium-node0
```

### 7. Stop the Cluster

When you're done, stop all services:

```bash
docker-compose -f ./dev/docker-compose.yaml down
```

## What's Running

Your MPCIUM cluster includes:

| Service         | Purpose                                | Port  |
| --------------- | -------------------------------------- | ----- |
| **NATS Server** | Messaging layer for node communication | 4222  |
| **Consul**      | Service discovery and health checks    | 8500  |
| **PostgreSQL**  | Database for custody operations        | 5432  |
| **Redis**       | In-memory data store                   | 6379  |
| **MongoDB**     | Document database                      | 27017 |
| **Apex API**    | Main API service                       | 8150  |
| **Migrate**     | Database migration service             | -     |
| **MPC Node 0**  | First MPC node                         | 8080  |
| **MPC Node 1**  | Second MPC node                        | 8081  |
| **MPC Node 2**  | Third MPC node                         | 8082  |
| **MPCIUM Init** | Peer registration service              | -     |
