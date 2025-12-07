# Grafana Setup - Visualization Hub

This branch contains the configuration for Grafana, used to visualize metrics from both the IoT platforms (via Prometheus) and the load test results (via InfluxDB).

## 🏗️ Services

*   **Grafana**: The visualization dashboard.
*   **Grafana Image Renderer**: Plugin for server-side rendering of panels (useful for reports).
*   **InfluxDB (v2)**: Time-series database used specifically to store raw results from JMeter.

## 🚀 Deployment Guide

### 1. Start the Services

Run the following command in this directory:

```bash
docker compose up -d
```

### 2. Configure InfluxDB

JMeter needs to write data to InfluxDB, and Grafana needs to read from it.

1.  Access InfluxDB UI at `http://localhost:8086`.
2.  Complete the initial setup (create an organization and bucket, e.g., `jmeter`).
3.  **Generate an API Token**.
4.  Save this token for later use in:
    *   Grafana Data Source configuration.
    *   JMeter Test Plans (Backend Listener).

### 3. Configure Grafana

1.  Access Grafana at `http://localhost:3000`.
    *   Default login: `admin` / `admin` (unless configured otherwise).
2.  **Data Sources**:
    *   Go to **Configuration > Data Sources**.
    *   **Prometheus**: Should be pre-configured to point to your Prometheus instance (e.g., `http://prometheus:9090`).
    *   **InfluxDB**: Add a new InfluxDB source.
        *   Query Language: `Flux` (if using InfluxDB v2).
        *   URL: `http://influxdb:8086`.
        *   Organization: (Your Org).
        *   Token: (The token you generated).
        *   Default Bucket: `jmeter`.
3.  **Dashboards**:
    *   Pre-provisioned dashboards are located in `grafana/provisioning/dashboards/`.
    *   You should see dashboards for "JMeter Performance", "Node Exporter", etc.

## 📂 File Structure

*   `docker-compose.yml`: Deploys Grafana, Renderer, and InfluxDB.
*   `grafana/provisioning/`: Automated configuration for data sources and dashboards.
*   `influx.yml`: Separate compose file if you wish to run InfluxDB standalone.