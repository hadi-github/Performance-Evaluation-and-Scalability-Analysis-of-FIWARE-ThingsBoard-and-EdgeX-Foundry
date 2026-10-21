#!/usr/bin/env python3
"""Create (or re-create) the Yaade collections for the IoT benchmark platforms.

One collection per platform (ThingsBoard, FIWARE, EdgeX). Each collection has:
  - an environment "ramonavm2" (proxy = Yaade server) with the platform URLs and inputs,
  - numbered requests that walk the full workflow: auth/provision -> send an integer -> read it back -> clean up.
    Every request's response script stores what the platform returned (JWT, ids, device token, ...) into the
    environment, so the next request can simply be clicked in the UI,
  - a job script "Run full workflow" that executes the whole chain server-side with Jasmine assertions.
    Run it from the UI (RUN button) or with:  ./run_workflow.py <thingsboard|fiware|edgex>

Usage:
  YAADE_URL=http://ramonavm1:9339 YAADE_PASSWORD=... ./build_collections.py [--sut-host 87.107.191.107]
Re-running deletes and re-creates only the collections this script owns (matched by name).
"""
import argparse
import json
import os
import sys
import urllib.request
from http.cookiejar import CookieJar

PREFIX = "IoT bench - "
ENV_NAME = "ramonavm2"

# Shared Jasmine prelude for job scripts. Steps depend on each other, so run them in order and stop at the
# first failure. exec() runs requests server-side but NOT their response scripts, so the jobs parse responses
# themselves. There is no setTimeout in job scripts; pause() busy-waits (jobs are limited to 30 s in total).
JOB_PRELUDE = """
jasmine.getEnv().configure({ random: false, stopOnSpecFailure: true });
var R = __IDS__;
function pause(ms) { var t = Date.now(); while (Date.now() - t < ms) {} }
async function call(step) {
  var r = await exec(R[step], env.name);
  log(step + " -> HTTP " + r.status);
  return r;
}
// Re-run a request until check(res) is true (async ingestion paths need a moment).
async function poll(step, check, tries) {
  var r = null;
  for (var i = 0; i < tries; i++) {
    r = await call(step);
    if (check(r)) return r;
    pause(300);
  }
  return r;
}
"""


def req(name, method, uri, headers=None, body=None, content_type=None, request_script="", response_script=""):
    return {
        "name": name,
        "method": method,
        "uri": uri,
        "headers": [{"key": k, "value": v, "isEnabled": True} for k, v in (headers or {}).items()],
        "params": [],
        "body": body or "",
        "contentType": content_type or "",
        "requestScript": request_script.strip(),
        "responseScript": response_script.strip(),
    }


# --------------------------------------------------------------------------------------------- ThingsBoard
TB_AUTH = {"X-Authorization": "Bearer ${tb_jwt}"}
THINGSBOARD = {
    "name": PREFIX + "ThingsBoard",
    "description": "ThingsBoard CE 4.3 behind HAProxy (:8080). Login -> create device -> get its access token -> "
                   "send an integer over the device HTTP API -> read the latest value back -> delete the device.",
    "env": {
        "tb_url": "http://{sut}:8080",
        "tb_user": "tenant@thingsboard.org",
        "tb_pass": "tenant",
        "tb_device_name": "yaade-device",
        "value": "42",
    },
    "requests": [
        ("login", req("1. Login (tenant) -> tb_jwt", "POST", "${tb_url}/api/auth/login",
            {"Content-Type": "application/json"},
            '{"username": "${tb_user}", "password": "${tb_pass}"}', "application/json",
            response_script="""
if (res.status === 200) {
  env.set("tb_jwt", jp("$.token", res.body))
  env.set("tb_refresh_token", jp("$.refreshToken", res.body))
}""")),
        ("create_device", req("2. Create device -> tb_device_id", "POST", "${tb_url}/api/device",
            dict(TB_AUTH, **{"Content-Type": "application/json"}),
            '{"name": "${tb_device_name}", "type": "default"}', "application/json",
            request_script='env.set("tb_device_name", "yaade-" + DateTime.utc().toFormat("yyyyMMdd-HHmmss"))',
            response_script="""
if (res.status === 200) {
  env.set("tb_device_id", jp("$.id.id", res.body))
}""")),
        ("get_credentials", req("3. Get device credentials -> tb_device_token", "GET",
            "${tb_url}/api/device/${tb_device_id}/credentials", TB_AUTH,
            response_script="""
if (res.status === 200) {
  env.set("tb_device_token", jp("$.credentialsId", res.body))
}""")),
        ("send", req("4. Send integer telemetry (device token)", "POST",
            "${tb_url}/api/v1/${tb_device_token}/telemetry",
            {"Content-Type": "application/json"}, '{"temperature": ${value}}', "application/json",
            request_script='env.set("value", String(rand.integer(1, 1000)))')),
        ("read", req("5. Read latest telemetry -> tb_last_value", "GET",
            "${tb_url}/api/plugins/telemetry/DEVICE/${tb_device_id}/values/timeseries?keys=temperature", TB_AUTH,
            response_script="""
if (res.status === 200) {
  env.set("tb_last_value", String(jp("$.temperature[0].value", res.body)))
}""")),
        ("delete_device", req("6. Delete device (cleanup)", "DELETE",
            "${tb_url}/api/device/${tb_device_id}", TB_AUTH)),
    ],
    "job": """
describe("ThingsBoard: provision, send an integer, read it back", function () {
  it("logs in as the tenant", async function () {
    var r = await call("login");
    expect(r.status).toBe(200);
    env.set("tb_jwt", jp("$.token", r.body));
  });
  it("creates a device", async function () {
    env.set("tb_device_name", "yaade-job-" + Date.now());
    var r = await call("create_device");
    expect(r.status).toBe(200);
    env.set("tb_device_id", jp("$.id.id", r.body));
  });
  it("reads the device access token", async function () {
    var r = await call("get_credentials");
    expect(r.status).toBe(200);
    env.set("tb_device_token", jp("$.credentialsId", r.body));
  });
  it("sends an integer as the device", async function () {
    env.set("value", String(rand.integer(1, 1000)));
    var r = await call("send");
    expect(r.status).toBe(200);
  });
  it("reads the same integer back", async function () {
    var want = env.get("value");
    var r = await poll("read", function (x) {
      return x.status === 200 && String(jp("$.temperature[0].value", x.body)) === want;
    }, 30);
    expect(r.status).toBe(200);
    var got = String(jp("$.temperature[0].value", r.body));
    env.set("tb_last_value", got);
    log("sent " + want + ", read back " + got);
    expect(got).toBe(want);
  });
  it("deletes the device", async function () {
    var r = await call("delete_device");
    expect(r.status).toBe(200);
  });
});
""",
}

# --------------------------------------------------------------------------------------------- FIWARE
FW_HDR = {"fiware-service": "${fiware_service}", "fiware-servicepath": "${fiware_servicepath}"}
FW_JSON = dict(FW_HDR, **{"Content-Type": "application/json"})
FIWARE = {
    "name": PREFIX + "FIWARE",
    "description": "Orion 4.5 + IoT Agent UL 3.7 behind HAProxy. Create a service group (API key) -> provision a "
                   "device -> send an UltraLight integer measure over HTTP (:7896) -> read the entity from Orion "
                   "(:1026) -> clean up device, entity and service group.",
    "env": {
        "orion_url": "http://{sut}:1026",
        "iota_north_url": "http://{sut}:4041",
        "iota_http_url": "http://{sut}:7896",
        "fiware_service": "openiot",
        "fiware_servicepath": "/",
        "fiware_apikey": "yaadekey",
        "fiware_device_id": "yaade-dev",
        "fiware_entity_id": "urn:ngsi-ld:Sensor:yaade-dev",
        "value": "42",
    },
    "requests": [
        ("version", req("0. Orion version (health)", "GET", "${orion_url}/version")),
        ("create_group", req("1. Create service group (API key)", "POST", "${iota_north_url}/iot/services", FW_JSON,
            '{"services": [{"apikey": "${fiware_apikey}", "entity_type": "Sensor", "resource": "/iot/d"}]}',
            "application/json",
            request_script="""
var id = "yaade" + DateTime.utc().toFormat("yyyyMMddHHmmss")
env.set("fiware_apikey", id)
env.set("fiware_device_id", "dev-" + id)
env.set("fiware_entity_id", "urn:ngsi-ld:Sensor:" + id)""")),
        ("create_device", req("2. Provision device", "POST", "${iota_north_url}/iot/devices", FW_JSON,
            '{"devices": [{"device_id": "${fiware_device_id}", "apikey": "${fiware_apikey}", '
            '"entity_name": "${fiware_entity_id}", "entity_type": "Sensor", "protocol": "PDI-IoTA-UltraLight", '
            '"attributes": [{"object_id": "t", "name": "temperature", "type": "Integer"}]}]}',
            "application/json")),
        ("get_device", req("3. Get provisioned device", "GET",
            "${iota_north_url}/iot/devices/${fiware_device_id}", FW_HDR,
            response_script="""
if (res.status === 200) {
  env.set("fiware_entity_id", jp("$.entity_name", res.body))
}""")),
        ("send", req("4. Send integer measure (UltraLight over HTTP)", "POST",
            "${iota_http_url}/iot/d?k=${fiware_apikey}&i=${fiware_device_id}",
            {"Content-Type": "text/plain"}, "t|${value}", "text/plain",
            request_script='env.set("value", String(rand.integer(1, 1000)))')),
        ("read", req("5. Read entity from Orion -> fiware_last_value", "GET",
            "${orion_url}/v2/entities/${fiware_entity_id}?options=keyValues", FW_HDR,
            response_script="""
if (res.status === 200) {
  env.set("fiware_last_value", String(jp("$.temperature", res.body)))
}""")),
        ("delete_device", req("6. Delete device (cleanup)", "DELETE",
            "${iota_north_url}/iot/devices/${fiware_device_id}", FW_HDR)),
        ("delete_entity", req("7. Delete Orion entity (cleanup)", "DELETE",
            "${orion_url}/v2/entities/${fiware_entity_id}", FW_HDR)),
        ("delete_group", req("8. Delete service group (cleanup)", "DELETE",
            "${iota_north_url}/iot/services?resource=/iot/d&apikey=${fiware_apikey}", FW_HDR)),
    ],
    "job": """
describe("FIWARE: provision, send an integer, read it back", function () {
  it("Orion is reachable", async function () {
    var r = await call("version");
    expect(r.status).toBe(200);
  });
  it("creates a service group", async function () {
    var id = "yaadejob" + Date.now();
    env.set("fiware_apikey", id);
    env.set("fiware_device_id", "dev-" + id);
    env.set("fiware_entity_id", "urn:ngsi-ld:Sensor:" + id);
    var r = await call("create_group");
    expect(r.status).toBe(201);
  });
  it("provisions a device", async function () {
    var r = await call("create_device");
    expect(r.status).toBe(201);
  });
  it("reads the provisioned device", async function () {
    var r = await call("get_device");
    expect(r.status).toBe(200);
    expect(jp("$.entity_name", r.body)).toBe(env.get("fiware_entity_id"));
  });
  it("sends an integer measure", async function () {
    env.set("value", String(rand.integer(1, 1000)));
    var r = await call("send");
    expect(r.status).toBe(200);
  });
  it("reads the same integer back from Orion", async function () {
    var want = env.get("value");
    var r = await poll("read", function (x) {
      return x.status === 200 && String(jp("$.temperature", x.body)) === want;
    }, 20);
    expect(r.status).toBe(200);
    var got = String(jp("$.temperature", r.body));
    env.set("fiware_last_value", got);
    log("sent " + want + ", read back " + got);
    expect(got).toBe(want);
  });
  it("cleans up device, entity and service group", async function () {
    expect((await call("delete_device")).status).toBe(204);
    expect((await call("delete_entity")).status).toBe(204);
    expect((await call("delete_group")).status).toBe(204);
  });
});
""",
}

# --------------------------------------------------------------------------------------------- EdgeX
EDGEX = {
    "name": PREFIX + "EdgeX",
    "description": "EdgeX 4.0 (NATS build) behind HAProxy. Create a device profile -> create a device on device-rest "
                   "-> send an integer reading (:59986) -> read it back from core-data (:59880) -> delete the device.",
    "env": {
        "edgex_meta_url": "http://{sut}:59881",
        "edgex_data_url": "http://{sut}:59880",
        "edgex_rest_url": "http://{sut}:59986",
        "edgex_profile": "yaade-profile",
        "edgex_device": "yaade-device",
        "value": "42",
    },
    "requests": [
        ("ping", req("0. core-metadata ping (health)", "GET", "${edgex_meta_url}/api/v3/ping")),
        ("create_profile", req("1. Create device profile (201, or 409 if it exists)", "POST",
            "${edgex_meta_url}/api/v3/deviceprofile", {"Content-Type": "application/json"},
            '[{"apiVersion": "v3", "profile": {"name": "${edgex_profile}", "manufacturer": "yaade", "model": "v1", '
            '"labels": ["yaade"], "deviceResources": [{"name": "temperature", "isHidden": false, '
            '"properties": {"valueType": "Int64", "readWrite": "RW"}}], "deviceCommands": []}}]',
            "application/json")),
        ("create_device", req("2. Create device -> edgex_device_id", "POST", "${edgex_meta_url}/api/v3/device",
            {"Content-Type": "application/json"},
            '[{"apiVersion": "v3", "device": {"name": "${edgex_device}", "serviceName": "device-rest", '
            '"profileName": "${edgex_profile}", "adminState": "UNLOCKED", "operatingState": "UP", '
            '"protocols": {"other": {"Address": "yaade"}}}}]',
            "application/json",
            request_script='env.set("edgex_device", "yaade-" + DateTime.utc().toFormat("yyyyMMdd-HHmmss"))',
            response_script="""
if (res.status === 207 && jp("$[0].statusCode", res.body) === 201) {
  env.set("edgex_device_id", jp("$[0].id", res.body))
}""")),
        ("get_device", req("3. Get device", "GET", "${edgex_meta_url}/api/v3/device/name/${edgex_device}")),
        ("send", req("4. Send integer reading (device-rest)", "POST",
            "${edgex_rest_url}/api/v3/resource/${edgex_device}/temperature",
            {"Content-Type": "text/plain"}, "${value}", "text/plain",
            request_script='env.set("value", String(rand.integer(1, 1000)))')),
        ("read", req("5. Read latest reading -> edgex_last_value", "GET",
            "${edgex_data_url}/api/v3/reading/device/name/${edgex_device}?limit=1",
            response_script="""
if (res.status === 200) {
  env.set("edgex_last_value", String(jp("$.readings[0].value", res.body)))
}""")),
        ("delete_device", req("6. Delete device (cleanup)", "DELETE",
            "${edgex_meta_url}/api/v3/device/name/${edgex_device}")),
    ],
    "job": """
describe("EdgeX: provision, send an integer, read it back", function () {
  it("core-metadata is reachable", async function () {
    var r = await call("ping");
    expect(r.status).toBe(200);
  });
  it("creates the device profile (or it already exists)", async function () {
    var r = await call("create_profile");
    expect(r.status).toBe(207);
    expect([201, 409]).toContain(jp("$[0].statusCode", r.body));
  });
  it("creates a device", async function () {
    env.set("edgex_device", "yaade-job-" + Date.now());
    var r = await call("create_device");
    expect(r.status).toBe(207);
    expect(jp("$[0].statusCode", r.body)).toBe(201);
    env.set("edgex_device_id", jp("$[0].id", r.body));
  });
  it("reads the device", async function () {
    var r = await call("get_device");
    expect(r.status).toBe(200);
  });
  it("sends an integer reading", async function () {
    env.set("value", String(rand.integer(1, 1000)));
    // device-rest learns about new devices asynchronously (system event over NATS): retry until it accepts.
    var r = await poll("send", function (x) { return x.status === 200; }, 20);
    expect(r.status).toBe(200);
  });
  it("reads the same integer back from core-data", async function () {
    var want = env.get("value");
    var r = await poll("read", function (x) {
      return x.status === 200 && String(jp("$.readings[0].value", x.body)) === want;
    }, 20);
    expect(r.status).toBe(200);
    var got = String(jp("$.readings[0].value", r.body));
    env.set("edgex_last_value", got);
    log("sent " + want + ", read back " + got);
    expect(got).toBe(want);
  });
  it("deletes the device", async function () {
    var r = await call("delete_device");
    expect(r.status).toBe(200);
  });
});
""",
}

PLATFORMS = {"thingsboard": THINGSBOARD, "fiware": FIWARE, "edgex": EDGEX}


class Yaade:
    def __init__(self, base, user, password):
        self.base = base.rstrip("/")
        self.opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(CookieJar()))
        self.call("POST", "/api/login", {"username": user, "password": password})

    def call(self, method, path, body=None):
        data = None if body is None else json.dumps(body).encode()
        r = urllib.request.Request(self.base + path, data=data, method=method,
                                   headers={"Content-Type": "application/json"})
        with self.opener.open(r, timeout=60) as resp:
            raw = resp.read()
        return json.loads(raw) if raw.strip() else None


def build(y, spec, sut):
    for c in y.call("GET", "/api/collection") or []:
        if c["data"].get("name") == spec["name"]:
            y.call("DELETE", "/api/collection/%d" % c["id"])
    col = y.call("POST", "/api/collection", {"name": spec["name"], "groups": []})
    cid = col["id"]
    env = {k: v.replace("{sut}", sut) for k, v in spec["env"].items()}
    y.call("POST", "/api/collection/%d/envs/%s" % (cid, ENV_NAME), {"data": env, "proxy": "server"})

    ids = {}
    for rank, (key, data) in enumerate(spec["requests"]):
        data = dict(data, rank=rank)
        ids[key] = y.call("POST", "/api/request", {"collectionId": cid, "type": "REST", "data": data})["id"]

    # The collection description doubles as in-app documentation.
    col = next(c for c in y.call("GET", "/api/collection") if c["id"] == cid)
    col["data"]["description"] = spec["description"]
    y.call("PUT", "/api/collection", {"id": cid, "ownerId": col["ownerId"], "version": col.get("version"),
                                      "data": col["data"]})

    script = JOB_PRELUDE.replace("__IDS__", json.dumps(ids)) + spec["job"]
    job = y.call("POST", "/api/scripts", {"collectionId": cid, "data": {"name": "Run full workflow"}})
    y.call("PUT", "/api/scripts", {"id": job["id"], "collectionId": cid, "ownerId": job.get("ownerId"),
                                   "version": job.get("version"),
                                   "data": {"name": "Run full workflow", "script": script, "selectedEnvName": ENV_NAME,
                                            "enabled": False, "cronExpression": ""}})
    return cid, job["id"]


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--yaade-url", default=os.environ.get("YAADE_URL", "http://ramonavm1:9339"))
    ap.add_argument("--user", default=os.environ.get("YAADE_USER", "admin"))
    ap.add_argument("--sut-host", default=os.environ.get("SUT_HOST", "87.107.191.107"))
    ap.add_argument("platforms", nargs="*", default=list(PLATFORMS), choices=list(PLATFORMS))
    a = ap.parse_args()
    password = os.environ.get("YAADE_PASSWORD") or sys.exit("set YAADE_PASSWORD")
    y = Yaade(a.yaade_url, a.user, password)
    for p in a.platforms:
        cid, sid = build(y, PLATFORMS[p], a.sut_host)
        print("%-12s collection=%d  job-script=%d" % (p, cid, sid))


if __name__ == "__main__":
    main()
