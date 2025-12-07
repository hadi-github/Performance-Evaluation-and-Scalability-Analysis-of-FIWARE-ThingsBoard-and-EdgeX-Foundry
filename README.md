# EdgeX Foundry - Single Operation (Monolithic)

This branch contains the deployment configuration for running the EdgeX Foundry platform as a single-node monolithic stack. This includes all core services, supporting services, and the Redis database running in one Docker Compose environment.

## 🏗️ Architecture

All components run in a single docker-compose environment on one machine.

## 🚀 Deployment Guide

### 1. Prerequisites

This deployment uses an external Docker network to allow easy attachment of other tools (like testing scripts) if needed. You must create it first:

```bash
docker network create edgex-network
```

### 2. Start the Platform

Run the following command in this directory:

```bash
docker compose up -d
```

### 3. Verify Deployment

Check if the containers are running:

```bash
docker compose ps
```

### 4. Services & Ports

The following core services are exposed on the host:

| Service | Port | Description |
|---------|------|-------------|
| **Consul** | `8500` | Configuration & Registry |
| **Core Data** | `59880` | Persists data from devices |
| **Core Metadata** | `59881` | Manages metadata about devices |
| **Core Command** | `59882` | Manages commands to devices |
| **Device Virtual** | `59900` | Simulates devices |
| **Device REST** | `59986` | REST Interface for devices |
| **App Rules Engine** | `59701` | Rules Engine (eKuiper) |
| **Support Notifications** | `59860` | Alerts & Notifications |
| **Support Scheduler** | `59861` | Scheduling service |
| **EdgeX UI** | `4000` | Web Management Interface |

### 5. Health Checks

A script is provided to check the health of all services.

```bash
chmod +x check_health.sh
./check_health.sh
```

You can also manually check a service, for example Core Data:
`curl http://localhost:59880/api/v3/ping`

## 🛑 Shutdown

To stop and remove the containers:

```bash
docker compose down
```

To also remove the data volumes (Redis data, Consul config, eKuiper data):

```bash
docker compose down -v
```