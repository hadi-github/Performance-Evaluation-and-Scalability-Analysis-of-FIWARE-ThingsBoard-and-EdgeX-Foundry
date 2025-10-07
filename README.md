# EdgeX Foundry — Multi-Node Deployment

This project provides a multi-node deployment setup for the **EdgeX Foundry** platform.  
It includes configurations for the core services, Redis database replication, and an Nginx load balancer.

---

## Project Structure

- **`server/`** — Contains the Docker Compose setup and deployment files for the EdgeX core modules.  
- **`db/`** — Contains the configuration for a replicated Redis master–slave database deployment.

---

## Services and Host Ports

Extracted from the Docker Compose configuration:

| Service | Host Port(s) | Notes |
|----------|---------------|-------|
| UI | `4000` | Web interface |
| Nginx | `8080` → container port `80` | Main proxy |
| Nginx (additional) | `7896`, `1026`, `4041` | Service-specific routes |
| EdgeX services (explicitly published) | `59701`, `59882`, `59880`, `59881`, `6379`, `59986`, `59900`, `59720`, `59860`, `59861` | Core and supporting services |

Some services are deployed with multiple instances (e.g., **core-metadata**, **core-data**) and balanced through **Nginx**.  
The Redis database is deployed separately on another host.

---

## Deployment Guide

### 1. Configure Environment Variables
Before deployment, update environment variables for the database connection:  
- Change any hostnames currently set to `192.168.2.105` to match your Redis host.  
- Update credentials as needed.  
- If you change ports, also update the corresponding routes in the **Nginx configuration**.

---

### 2. Deploy the Core Services
From inside the `server/` directory:

```bash
docker compose up -d
sleep 30 && ../check_health.sh
```

After ~30 seconds, the health check script should report all services as **healthy**.

---

### 3. Deploy Redis (on a separate host)
From inside the `db/` directory:

```bash
docker compose up -d
```

This starts the Redis master–slave setup.

---

## Health Check Script

Use the provided `check_health.sh` script to verify that all main modules respond correctly to their health APIs:

```bash
./check_health.sh
```

The script outputs a summary report of the current service states.

---

## Nginx Configuration

Nginx acts as a load balancer and reverse proxy, routing incoming requests to the correct EdgeX services:

| Port | Service |
|------|----------|
| `7896` | core-data |
| `1026` | core-metadata |
| `4041` | device-rest |

Configuration is located in the `nginx.conf` file.
