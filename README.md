# ThingsBoard - Single Operation (Monolithic)

This branch contains the deployment configuration for running the ThingsBoard platform as a single-node monolithic stack, including an internal PostgreSQL database.

## 🏗️ Architecture

*   **ThingsBoard Node**: Monolithic service handling core, rule engine, and transport.
*   **PostgreSQL**: Dedicated database for storing entities and telemetry.

Both services run in the same Docker Compose environment.

## 🚀 Deployment Guide

### 1. Prerequisites

This deployment requires an external network named `tb-network`.

```bash
docker network create tb-network
```

### 2. Initial Deployment (Installation)

For the **first run only**, ThingsBoard needs to install the database schema and load demo data.

1.  Start the stack:
    ```bash
    docker compose up -d
    ```

2.  **Monitor Logs**:
    Watch the logs to see the installation progress.
    ```bash
    docker compose logs -f thingsboard-allinone
    ```
    Wait until you see a message indicating `ThingsBoard installation finished` or the container restarts. The container might exit after installation; this is normal.

### 3. Regular Operation

Once installed, you should technically disable the installation flags, although the current configuration leaves them enabled for convenience in this test setup (it might restart a few times before detecting the DB is already installed).

To verify it's running:
```bash
docker compose ps
```

### 4. Access & Ports

| Service | Host Port | Internal Port | Description |
|---------|-----------|---------------|-------------|
| **Web UI** | `8081` | `9090` | Main Dashboard |
| **MQTT** | `1884` | `1883` | Device Connectivity |
| **CoAP** | `5689-5694`| `5683-5688` | CoAP Transport |

*   **Default User**: `tenant@thingsboard.org`
*   **Default Password**: `tenant`

## 🛑 Shutdown

To stop and remove the containers:

```bash
docker compose down
```

To remove the database volume (RESET EVERYTHING):

```bash
docker compose down -v
```
