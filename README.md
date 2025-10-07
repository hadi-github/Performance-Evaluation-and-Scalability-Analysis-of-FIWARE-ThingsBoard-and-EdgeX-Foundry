# ThingsBoard — Multiple Instances (concise)

This branch contains compose files for running multiple ThingsBoard instances. Below are the host ports used, a short description of the nginx and pgpool roles, and minimal commands to deploy them in the recommended order.

Ports (host)
- ThingsBoard UI/API instances (container 9090): host ports 8082, 8083, 8084
- Aggregator / frontend (nginx): 8081
- PostgreSQL (cluster members): host ports (internal 5432 per node; check `db/` compose for exact host bindings)
- Pgpool (Postgres connection pool / replication manager): commonly published on 5432 (check `db/` compose)

What nginx (in `server/`) does
- Acts as the HTTP reverse proxy and load-balancer for multiple ThingsBoard instances. It exposes a single frontend port (8081) and routes traffic to the backend ThingsBoard containers running on the host ports above.

What pgpool (in `db/`) does
- Pgpool provides connection pooling, load balancing and basic replication/health management for PostgreSQL nodes. Applications connect to pgpool (usually on 5432) instead of connecting directly to individual Postgres nodes.

Recommended minimal deploy flow
1. Deploy DB stack (Postgres nodes + pgpool):

```bash
docker compose up -d
```

2. Deploy ThingsBoard application instances (server folder):

```bash
docker compose up -d
```

3. Deploy nginx frontend (to consolidate routes):

```bash
cd nginx
docker compose -f nginx-compose.yml up -d
```

Quick tests

```bash
# Check frontend
curl -sS http://localhost:8081/ | head -n 5

# Check one ThingsBoard instance
curl -sS http://localhost:8082/api/status | jq .

# Check pgpool connectivity (TCP)
nc -zv localhost 5432
```

Notes
- the deployment of thingsboard is only tb-core containers, for the first time connecting to db you should uncomment the ```INSTALL_TB``` and ```LOAD_DEMO``` to make the tb-core run installation script for that database (if you just ncomment them for one container its enough)
after that you should watch tb-node logs and wait until it says installation finished then the container will restart and each restart will be exited cause its wants to create tables but they are available there already
so now that installation is complete you should down the deployment and comment those env variables and run it again
this time tb-node will connect to db and will work completely