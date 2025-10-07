# EdgeX — single deployment 
Services and common host ports (mapped by the compose files in this branch):

- core-metadata: 1026
- core-data: 7896
- core-command: 59882
- device-virtual: 59900
- device-rest: 4041
- app-rules-engine: 59701
- support-notifications: 59860
- support-scheduler: 59861

Quick start

```bash
# From this worktree:
docker compose up -d
```

Health checks (simple script)

This branch includes a tiny helper script `check_health.sh` that probes EdgeX service ping endpoints on localhost and reports a simple healthy/unhealthy result. It checks each service's `/api/v3/ping` endpoint with a 5s curl timeout.

To run the health check:

```bash
chmod +x check_health.sh
./check_health.sh
```

Output will be a short list of services with HEALTHY / UNHEALTHY status. This is intentionally simple — it assumes services are published on localhost at the ports listed above and that the `/api/v3/ping` endpoint returns a JSON containing `apiVersion` when healthy. Adjust ports in the script if your compose uses different host ports or environment variables.

Notes

- This README is intentionally minimal; expand it with run examples, logs, or diagnostics as needed.

