#!/usr/bin/env python3
"""Run the "Run full workflow" job script of a platform's Yaade collection and print the result.

Usage:
  YAADE_URL=http://ramonavm1:9339 YAADE_PASSWORD=... ./run_workflow.py <thingsboard|fiware|edgex> [...]
Exit code is 0 only if every step of every requested workflow passed.
"""
import argparse
import os
import sys

from build_collections import ENV_NAME, PLATFORMS, Yaade


def run(y, platform):
    name = PLATFORMS[platform]["name"]
    col = next((c for c in y.call("GET", "/api/collection") or [] if c["data"].get("name") == name), None)
    if col is None:
        sys.exit("collection %r not found - run build_collections.py first" % name)
    ref = next((s for s in col.get("scripts", []) if s["data"].get("name") == "Run full workflow"), None)
    if ref is None:
        sys.exit("job script not found in %r" % name)
    script = y.call("GET", "/api/scripts/%d" % ref["id"])
    res = y.call("POST", "/api/run-script", {"script": script, "envName": ENV_NAME}) or {}

    print("== %s" % platform)
    report = res.get("jasmineReport") or {}
    ok = bool(res.get("success")) and report.get("status") == "passed"
    for suite in report.get("suites") or []:
        for spec in suite.get("specs") or []:
            mark = {"passed": "PASS", "failed": "FAIL"}.get(spec.get("status"), spec.get("status", "?").upper())
            print("  [%s] %s" % (mark, spec.get("description")))
            for f in spec.get("failedExpectations") or []:
                print("         %s" % f.get("message"))
    for line in res.get("logs") or []:
        print("  log: %s" % line.get("message"))
    if res.get("error"):
        print("  error: %s" % res["error"])
    print("  => %s" % ("PASSED" if ok else "FAILED"))
    return ok


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--yaade-url", default=os.environ.get("YAADE_URL", "http://ramonavm1:9339"))
    ap.add_argument("--user", default=os.environ.get("YAADE_USER", "admin"))
    ap.add_argument("platforms", nargs="+", choices=list(PLATFORMS))
    a = ap.parse_args()
    password = os.environ.get("YAADE_PASSWORD") or sys.exit("set YAADE_PASSWORD")
    y = Yaade(a.yaade_url, a.user, password)
    results = [run(y, p) for p in a.platforms]
    sys.exit(0 if all(results) else 1)


if __name__ == "__main__":
    main()
