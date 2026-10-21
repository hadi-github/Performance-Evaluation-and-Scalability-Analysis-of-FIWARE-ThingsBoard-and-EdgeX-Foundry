# Yaade collections for the IoT benchmark platforms

Yaade runs on **ramonavm1** (`http://ramonavm1:9339`, user `admin`) and calls the platforms on
**ramonavm2** (`87.107.191.107`) through its server proxy, so no browser extension is needed.

Only one platform can run on ramonavm2 at a time (they share ports 8404 and 1883), so start the
platform you want to test first (`make up` / `make up N=3` in its directory).

## What each collection contains

| Collection | Workflow (numbered requests) | Values stored in env `ramonavm2` |
|---|---|---|
| IoT bench - ThingsBoard | login → create device → get device credentials → send `{"temperature": <int>}` → read latest telemetry → delete device | `tb_jwt`, `tb_refresh_token`, `tb_device_id`, `tb_device_token`, `tb_last_value` |
| IoT bench - FIWARE | Orion version → create service group (API key) → provision device → get device → send UltraLight `t\|<int>` → read entity from Orion → delete device, entity, service group | `fiware_apikey`, `fiware_device_id`, `fiware_entity_id`, `fiware_last_value` |
| IoT bench - EdgeX | ping → create device profile → create device → get device → send integer to device-rest → read latest reading from core-data → delete device | `edgex_device`, `edgex_device_id`, `edgex_last_value` |

**Clicking through in the UI:** select the `ramonavm2` environment and send the requests in order.
Request scripts generate fresh names (device name, API key) and a random integer `value`, and response
scripts store what the platform returns (JWT, ids, device token, ...). The next request then uses them
as `${...}` without any copy-paste.

**One click / one command:** every collection also has a job script, **Run full workflow**. It runs the
whole chain on the Yaade server with Jasmine assertions: it checks every status code and that the integer
read back equals the integer sent, then cleans up. Press **RUN** in the UI, or:

```bash
export YAADE_URL=http://ramonavm1:9339 YAADE_PASSWORD='<admin password>'
./run_workflow.py thingsboard      # or fiware / edgex; exit code 0 = all steps passed
```

## Re-creating the collections

```bash
./build_collections.py                     # all three
./build_collections.py edgex --sut-host 87.107.191.107
```

The script deletes and re-creates only the collections named `IoT bench - *`, so it is safe to re-run
after editing a request in the file.

## Notes

- Job scripts run requests with `exec()`, which does **not** run the requests' own response scripts.
  The job scripts therefore parse responses themselves, and stay in sync with the per-request scripts by
  hand.
- Jobs have a 30 s limit (`YAADE_SCRIPT_RUNNER_TIMEOUT`). The read-back steps poll every 300 ms
  because ingestion is asynchronous (TB batches writes, EdgeX goes through NATS).
- ThingsBoard uses the demo tenant `tenant@thingsboard.org` / `tenant` (created by `TB_LOAD_DEMO=true`).
