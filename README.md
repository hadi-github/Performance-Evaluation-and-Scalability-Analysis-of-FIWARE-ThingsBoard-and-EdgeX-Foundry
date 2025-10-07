# ThingsBoard — Single Instance (quick start)


Quick deploy

```bash
# From this worktree
docker compose up -d
```

Stop and cleanup

```bash
docker compose down
```

Notes
- the deployment of thingsboard is only tb-core containers, for the first time connecting to db you should uncomment the ```INSTALL_TB``` and ```LOAD_DEMO``` to make the tb-core run installation script for that database (if you just ncomment them for one container its enough)
after that you should watch tb-node logs and wait until it says installation finished then the container will restart and each restart will be exited cause its wants to create tables but they are available there already
so now that installation is complete you should down the deployment and comment those env variables and run it again
this time tb-node will connect to db and will work completely