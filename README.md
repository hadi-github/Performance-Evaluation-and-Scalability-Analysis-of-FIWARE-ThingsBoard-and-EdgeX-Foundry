the ```server``` directory contains fiware modules
and the ```db``` directory should be deployed on another server as external db

after deploying db containers the haproxy will run too, open server directory
```bash
docker compose up -d
```
then deploy nginx proxy yml file in ```nginx``` directory
```bash
docker compose -f nginx-compose.yml up -d
```
