# Finding: EdgeX core-metadata panics when a device is added while device-rest is scaled out

Platform finding from the exploratory capacity check (2026-10-22). EdgeX 4.0.2 (Odesa) built with NATS
(`include_nats_messaging`), `go-mod-messaging` v4.0.3, the stack in `edgex/compose.yaml`.

## Symptom

`POST /api/v3/device` to core-metadata gets `EOF` (no response). core-metadata exits with

```
panic: send on closed channel
github.com/edgexfoundry/go-mod-messaging/v4/internal/pkg/nats.(*Client).Subscribe.func1(...)
	github.com/edgexfoundry/go-mod-messaging/v4@v4.0.3/internal/pkg/nats/client.go:180
github.com/nats-io/nats%2ego.(*Conn).waitForMsgs(...)
	github.com/nats-io/nats.go@v1.50.0/nats.go:3520
```

and is restarted by its restart policy. The device-rest replicas, which depend on core-metadata, restart with it.

## Trigger and mechanism

1. Adding a device runs a **device-service validation** by request-reply over the message bus
   (`edgex-go` v4.0.2 `internal/core/metadata/application/device.go` `AddDevice` → `notify.go`
   `validateDeviceCallback` → `messagingClient.Request`). `PatchDevice` does the same. The request topic is built from
   the device's service name (`<base>/<serviceName>/` + `common.ValidateDeviceSubscribeTopic`), so it addresses
   the service, not one instance of it.
2. In this stack the device-rest replicas share one service name and deliberately use **no NATS queue group**, so
   every replica gets the device add/update system events and keeps an identical device cache
   (`edgex/compose.yaml`, service `device-rest`). Consequently **every replica also receives the validation request
   and replies**.
3. `go-mod-messaging` v4.0.3 `internal/pkg/request.go` `DoRequest` subscribes to the response topic with a channel
   of capacity 1, returns on the first reply, and its deferred cleanup unsubscribes and then `close(messages)`.
   A later reply that is already being delivered by the NATS callback (`client.go:180`, `tc.Messages <- env`) sends on
   the closed channel: the process panics.

The outcome is a race between the extra replies and the cleanup, so it does not fire on every request at every
replica count (see the observations below).

## Observations (ramonavm2, 8 vCPU, pool cores 4-7)

| Configuration | Event | core-metadata panics/restarts |
|---|---|---|
| N = 4 (4 device-rest replicas), cold start | start-up (each replica adds its bundled devices) | 4 restarts before any client request |
| N = 4 | `POST /device` (k6 setup, three attempts: `EXPL-edgex-trace`, `-trace2`, `-trace3`, the last with the export service stopped) | 3 of 3 crashed (restart 5 and 6, then 1 after a manual restart) |
| N = 2, cold start | start-up, then one `POST /device`, then one `POST /device?bypassValidation=true` | 0 (all 201) |
| N = 1 → provision 1000 devices → scale to N = 4 (`provision-then-scale.sh`) | provisioning at one replica, scale-out | 0 (gate: 4 × 1000/1000 devices accepted) |

## Minimal reproduction

```sh
cd edgex && make clean && make up N=4             # 4 device-rest replicas, core NATS, no queue group on device-rest
docker inspect -f '{{.RestartCount}}' edgex-core-metadata-1     # typically > 0 already after start-up
curl -X POST localhost:59881/api/v3/deviceprofile -H 'Content-Type: application/json' -d '[{"apiVersion":"v3","profile":
  {"name":"repro","manufacturer":"x","model":"x","deviceResources":[{"name":"r","properties":{"valueType":"Object","readWrite":"RW"}}]}}]'
curl -X POST localhost:59881/api/v3/device -H 'Content-Type: application/json' -d '[{"apiVersion":"v3","device":{"name":"repro-1",
  "serviceName":"device-rest","profileName":"repro","adminState":"UNLOCKED","operatingState":"UP","protocols":{"other":{"Address":"x"}}}}]'
# -> curl: empty reply / EOF; docker logs edgex-core-metadata-1 | grep -A8 '^panic:'
```

At N = 2 the same steps did not crash in one attempt (race); at N = 4 every attempt crashed.

## Operational consequence

- **Devices cannot be added (or updated) while the deployment is scaled out**: the request fails and core-metadata
  restarts, taking the device-rest replicas with it. A scaled EdgeX deployment of this shape can only be provisioned
  before scaling out.
- The scale-out start-up itself can crash core-metadata (bundled devices), which recovers through the restart policy.
- Ingestion does not use request-reply: once devices exist, readings, core-data persistence and the export path
  are not affected (the exploratory runs monitor this: see below).

## How the benchmark handles it (user decision, 2026-10-22)

- **Provision, then scale, at every k** (`edgex/scripts/provision-then-scale.sh prepare N D RUN`): cold start at one
  replica on the final pool cpuset, create the profile and the D devices, scale to N, start the export service.
  The generator's EdgeX setup reuses existing devices and posts none (`workloads/k6/lib/adapters.js`).
- **Health gate before each run:** every device-rest replica, addressed directly, accepts a reading for all D
  devices, and core-metadata has zero panics and restarts. A failed gate makes the run INVALID (setup failure).
- **During the run:** `provision-then-scale.sh check RUN` compares core-metadata restarts/panics/start time with the
  gate. Any new panic or restart makes the run **FAIL (platform defect)**; the log excerpt is kept as evidence.
- Recorded as a procedure deviation in each run's `manifest.json` (`deviations`, from the plan).
- The NATS queue-group configuration is **not** changed.

Not used, for reference: core-metadata accepts `POST /api/v3/device?bypassValidation=true`, which skips the
validation request-reply (an API option of the client, not platform configuration). It was only exercised at N = 2.
