# FIWARE - Single Operation (Monolithic)

This branch contains the deployment configuration for running the FIWARE platform (Orion Context Broker, IoT Agent, and MongoDB) as a single-node monolithic stack. This is suitable for development, testing, and low-load scenarios.

## 🏗️ Architecture

All components run in a single docker-compose environment on one machine.

*   **Orion Context Broker**: The core component managing context information.
*   **IoT Agent (UltraLight)**: Handles device connectivity and protocol translation.
*   **MongoDB**: The database backend for Orion and IoT Agent.

## 🚀 Deployment Guide

### 1. Configuration (.env)

A `.env` file is included to configure version tags and ports. You can modify this file to change versions or avoid port conflicts.

**Default `.env` values:**
```ini
ORION_VERSION=3.7.0
ULTRALIGHT_VERSION=1.25.0
MONGO_DB_VERSION=4.4
ORION_PORT=1026
IOTA_NORTH_PORT=4041
IOTA_SOUTH_PORT=7896
MONGO_DB_PORT=27017
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

You should see `fiware-orion`, `fiware-iot-agent`, and `db-mongo` in the `Up` state.

### 4. Health Checks

*   **Orion Version**: `curl http://localhost:1026/version`
*   **IoT Agent Health**: `curl http://localhost:4041/iot/about`

## 🛑 Shutdown

To stop and remove the containers:

```bash
docker compose down
```

To also remove the data volume (reset database):

```bash
docker compose down -v
```