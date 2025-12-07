# JMeter Workloads - Traffic Simulation

This branch contains the Apache JMeter test plans (`.jmx`) used to generate simulated IoT traffic against the platforms.

## 📂 Test Plans

Located in `platforms/`:

*   **`edgex/edgex-plan.jmx`**: Simulates devices sending data to EdgeX (Core Data / MQTT).
*   **`fiware/fiware-plan.jmx`**: Simulates devices sending UltraLight/JSON measurements to IoT Agent.
*   **`thingsboard/tb-plan.jmx`**: Simulates devices sending telemetry via MQTT/HTTP to ThingsBoard.

## 🚀 How to Run

### 1. Prerequisites

*   Apache JMeter (v5.5+) installed on your test client machine.
*   **InfluxDB** deployed (see `grafana-setup` branch) for collecting results.

### 2. Configuration

Open the desired `.jmx` file in JMeter GUI:

1.  **User Defined Variables**: Look for the "Config" element or Test Plan root.
    *   `TARGET_HOST`: IP address of the platform (Load Balancer or Node 1).
    *   `TARGET_PORT`: Port of the service (e.g., `4041`, `1883`, `8081`).
    *   `NUM_THREADS` (Users): Concurrency level.
    *   `DURATION`: Test duration.
2.  **Backend Listener**:
    *   Locate the "Backend Listener" element.
    *   Update `influxdbUrl`: `http://<INFLUX_HOST>:8086/write?db=jmeter` (check API compatibility for Influx v2).
    *   Update `influxdbToken`: Your InfluxDB API Token.

### 3. Execution (CLI Mode)

For actual benchmarking, always run in Non-GUI mode:

```bash
# Template
jmeter -n -t <test_plan>.jmx -l results.jtl -e -o ./report

# Example
jmeter -n -t platforms/fiware/fiware-plan.jmx -l fiware-run1.jtl
```

### 4. Analysis

*   **Real-time**: Watch the Grafana "JMeter Performance" dashboard.
*   **Post-test**: Open `results.jtl` in JMeter GUI or check the HTML report generated in the `./report` folder.