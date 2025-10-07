# EdgeX Foundry — Multiple Nodes

```server/``` folder contains the files and deployments for core moduels of edgex platform
```db/``` folder contains the configuration for a replicated master/slave deployment of redis db

## Services and actual host ports (extracted from compose)
- ui: 4000
- nginx: 8080 (mapped to container 80)
- nginx additional: 7896, 1026, 4041
- edgex services (where published explicitly): 59701, 59882, 59880, 59881, 6379, 59986, 59900, 59720, 59860, 59861

some services are deploymed with multiple instances and some are external too
there is 3 instances of core-metadata and core-data modules ( the requests will handle by nginx )
and the database deployment

## Commands and Scripts
-  To check the health of services there is a ```check_health.sh``` script which will test the api responding to all health apis of main modules and will return a report of the state of services
  ```bash
  ./check_health.sh
  ```
- config the platform deployment
search for environment variables that are for database hostname (they are set 192.168.2.105 which was the db host for our case) and change the hostname and other credentials for connecting to db
if you want to edit ports make sure that you will update the nginx to route to those too

- To deploy docker located at ```server/``` folder:
  ```bash
  docker compose up -d
  sleep 30 && ../check_health.sh # should see that all services are healthy
  ```

- To deploy redis on another server
```bash
docker comopse up -d
```

## nginx config

  the services will recieve requests with a load balancer nginx proxy that will redirect requests to the specified service

  7896 -> core-data
  1026 -> core-metadata
  4041 -> device-rest

  the config is located at ```nginx.conf``` file