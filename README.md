# EdgeX Foundry — Multiple Nodes

Short notes for the multi-node EdgeX deployment used in experiments.

## Services and actual host ports (extracted from compose)
- ui: 4000
- nginx: 8080 (mapped to container 80)
- nginx additional: 7896, 1026, 4041
- edgex services (where published explicitly): 59701, 59882, 59880, 59881, 6379, 59986, 59900, 59720, 59860, 59861

Commands
```bash
docker compose up -d
```
```bash
docker compose down
```
```bash
docker compose logs -f <service-name>
```

Notes

You can expand this file with architecture diagrams, example configs, and troubleshooting steps.
# EdgeX Foundry — Multiple Nodes

This branch contains deployment files and notes for running EdgeX Foundry across multiple nodes (or multiple service instances) for scalability tests.

## Services and Ports
- Core Data: `localhost:48080`
- Core Metadata: `localhost:48081`
- Core Command: `localhost:48082`
- Support Logging: `localhost:48061`
- Support Notifications: `localhost:48060`

## Commands and Scripts
- To deploy services:
  ```bash
  docker compose up -d
  # EdgeX Foundry — Multiple Nodes

  This branch contains deployment files and notes for running EdgeX Foundry across multiple nodes (or multiple service instances) for scalability tests.

  ## Services and Ports (common defaults)
  - Core Data: `localhost:48080`
  - Core Metadata: `localhost:48081`
  - Core Command: `localhost:48082`
  - Support Logging: `localhost:48061`
  - Support Notifications: `localhost:48060`

  ## Commands and Scripts
  - Deploy services:
    ```bash
    docker compose up -d
    ```
  - Check logs:
    ```bash
    docker logs <service-name>
    ```

  Refer to the `deploy/` or `server/` directory for additional scripts and configuration files.

  Note: verify the actual ports in the `docker-compose.yml` or compose files in this directory; the values above are typical defaults and may differ in customized setups.