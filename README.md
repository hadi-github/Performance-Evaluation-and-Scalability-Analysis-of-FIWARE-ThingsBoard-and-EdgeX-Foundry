# Prometheus Setup - Monitoring Agents

This branch configures the monitoring stack responsible for collecting system and application metrics from your deployment nodes.

## 🏗️ Services & Exporters

*   **Prometheus**: The core metrics server that scrapes data from exporters.
*   **Node Exporter**: Collects OS-level metrics (CPU, RAM, Disk, Network).
*   **cAdvisor**: Collects container-level metrics (Docker usage).
*   **MongoDB Exporter**: Specific metrics for FIWARE DB.
*   **Redis Exporter**: Specific metrics for EdgeX DB.
*   **PostgreSQL Exporter**: Specific metrics for ThingsBoard DB.

## 🚀 Deployment Guide

This stack should be deployed on **every node** you wish to monitor, or configured centrally to scrape remote targets.

### 1. Configuration (`prometheus.yml`)

Before starting, edit `prometheus.yml` to define your scrape targets.

```yaml
scrape_configs:
  - job_name: 'node-exporter'
    static_configs:
      - targets: ['192.168.2.105:9100', '192.168.2.106:9100'] # Add your Node IPs
```

### 2. Start Monitoring

```bash
docker compose up -d
```

### 3. Verify

*   **Prometheus UI**: `http://localhost:9090`
*   **Targets**: Check `http://localhost:9090/targets` to see if all exporters are `UP`.

## 📋 Exporter Ports

| Exporter | Port | Description |
|----------|------|-------------|
| **Node Exporter** | `9100` | System Metrics |
| **cAdvisor** | `8080` | Container Metrics |
| **Mongo Exporter** | `9216` | MongoDB Metrics |
| **Redis Exporter** | `9121` | Redis Metrics |
| **Postgres Exporter** | `9187` | PostgreSQL Metrics |