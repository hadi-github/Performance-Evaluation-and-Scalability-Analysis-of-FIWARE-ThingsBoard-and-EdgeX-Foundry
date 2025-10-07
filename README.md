# FIWARE — Multiple Instances (concise)

This README explains the multi-instance FIWARE layout, lists the host ports used by this branch, and provides a simple run order and commands.

Overview and directories
- `server/` — FIWARE services and the nginx proxy compose used to route requests to the platform instances.
- `db/` — MongoDB replica-set compose and an HAProxy instance that fronts the replica set for clients.

Ports required (host ports published by compose files in this branch)
- Orion instances (container 1026): host ports 1027, 1028, 1029
- IoT Agent (north API container 4041): host ports 4042, 4043, 4044
- IoT Agent (south/device port container 7896): host ports 7897, 7898, 7899
- MongoDB replica members (container 27017): host ports 27027, 27028, 27029
- HAProxy (fronting Mongo): 27019
- Mosquitto (MQTT): 1883 (and WebSocket 9001)
- RabbitMQ (AMQP): 5672 and management UI: 15672
- Nginx (reverse proxy / status): 1026, 7896, 4041 and HTTP status on 8080

Short description of nginx (in `server/`)
- Purpose: a reverse proxy that accepts incoming HTTP requests on published host ports (e.g. 1026, 4041, 7896) and routes them to the appropriate FIWARE internal service instance(s). It can also provide a central status page (on 8080) and handle TLS/virtual-hosting if configured.
- When to run: after the database layer and platform services are running.

Short description of HAProxy (in `db/`)
- Purpose: HAProxy is used to front the MongoDB replica set. Clients connect to HAProxy (port 27019) which load-balances read/write traffic across the replica members and simplifies client configuration.
- When to run: start the DB stack first; HAProxy will be available as part of the `db/` compose and should come up before platform services that depend on Mongo.

Recommended deployment flow (minimal, reliable order)
1. Deploy DB and HAProxy (so Mongo is available and reachable via the HAProxy endpoint):

```bash
# from the repo root or inside the db/ folder
docker compose up -d
```

This starts the Mongo replica nodes and the HAProxy front-end on host port 27019.

2. Deploy the FIWARE platform services (server folder):

```bash
docker compose up -d
```

This will bring up Orion, IoT-Agent, Mosquitto, RabbitMQ, and other services. They should be configured to talk to Mongo via the HAProxy endpoint (27019) 

3. Deploy the nginx reverse-proxy

```bash
cd nginx
docker compose -f nginx-compose.yml up -d
```

The nginx proxy consolidates endpoints and exposes the user-facing host ports listed above.

4. Test endpoints & health checks

After the services are up, test the endpoints on the host ports to verify availability