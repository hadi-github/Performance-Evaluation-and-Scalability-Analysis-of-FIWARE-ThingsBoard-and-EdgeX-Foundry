#!/usr/bin/env bash
# Build the amended device-rest image: upstream v4.0.2 + the profile-fetch dedup
# patch. Opt-in only -- nothing calls this unless a run sets
# IOTBENCH_EDGEX_DISCOVERY_AMENDMENT=1.
#
#   deploy/edgex/scripts/build-patched-device-rest.sh [--tag TAG] [--verify-only]
#
# Exits non-zero on any doubt. The point is not "did docker build succeed" -- a
# build can succeed while the patch is absent (see the -mod=readonly note in
# amendment/Dockerfile.device-rest) -- but "does the produced binary actually
# contain the deduplicated loop".
#
# Provenance for the amendment:
#   research/findings/edgex-profile-fetch-dedup-amendment-20261115.md
set -euo pipefail
cd "$(dirname "$0")/.."

REPO=$(cd ../.. && pwd)
VERSION="${EDGEX_DEVICE_REST_VERSION:-4.0.2}"
TAG="edgex-amended/device-rest:${VERSION}-p1"
PATCH_FILE=amendment/device-sdk-dedup-profile-fetch.patch
# Same default as compose.yaml; a mirror keeps the build alive where Docker Hub
# resets connections. Override with GO_BUILDER_BASE / DOCKER_RUNTIME_BASE.
BASE_IMAGE="${GO_BUILDER_BASE:-golang:1.25-alpine3.22}"
RUNTIME_IMAGE="${DOCKER_RUNTIME_BASE:-alpine:3.22}"
SDK_DIR=vendor/github.com/edgexfoundry/device-sdk-go/v4
MARKER=seenProfile

VERIFY_ONLY=0
while [[ $# -gt 0 ]]; do
  case "$1" in
    --tag) TAG="${2:?tag}"; shift 2 ;;
    --verify-only) VERIFY_ONLY=1; shift ;;
    -h|--help) sed -n '2,18p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done

# The dedup loop must be genuinely in effect. A binary grep cannot prove this
# (Go strips local variable names even without -s -w, so `seenProfile` is absent
# from the shipped binary). The build stage runs a behavioural test instead;
# see amendment/Dockerfile.device-rest. This wrapper only checks the image
# exists and carries the expected entrypoint, so an operator gets a clear
# message rather than a silent fallback to a stock image.
verify_image() { # $1=image tag
  echo "verifying image $1"
  docker image inspect "$1" >/dev/null || {
    echo "image $1 was not produced" >&2; exit 1
  }
  docker run --rm --entrypoint /device-rest "$1" --version 2>&1 | head -2 || true
}

if (( VERIFY_ONLY )); then
  verify_image "$TAG"
  exit 0
fi

command -v docker >/dev/null || { echo "docker required" >&2; exit 2; }
[[ -f "$PATCH_FILE" ]] || { echo "patch missing: $PATCH_FILE" >&2; exit 2; }

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
src="$work/src"

echo "fetching device-rest-go v$VERSION source (no git/apk needed in the image)"
mkdir -p "$src"
curl -fsSL "https://codeload.github.com/edgexfoundry/device-rest-go/tar.gz/refs/tags/v${VERSION}" \
  | tar -xz -C "$src" --strip-components=1

[[ -f "$src/$SDK_DIR/internal/cache/init.go" ]] || {
  echo "vendored device-sdk-go not found in the fetched source; refusing to patch" >&2
  exit 3
}
# Pin the pre-image. If upstream restructures the file the patch context would
# fail anyway, but this makes the intent explicit and the error legible.
pre=$(sha256sum "$src/$SDK_DIR/internal/cache/init.go" | cut -d' ' -f1)
echo "pre-patch init.go sha256: $pre"
if grep -q "$MARKER" "$src/$SDK_DIR/internal/cache/init.go"; then
  echo "source already contains $MARKER; refusing (would double-patch)" >&2
  exit 3
fi

echo "applying $PATCH_FILE"
patch -p1 -d "$src/$SDK_DIR" < "$PATCH_FILE"
grep -q "$MARKER" "$src/$SDK_DIR/internal/cache/init.go" || {
  echo "patch reported success but marker is absent; refusing" >&2
  exit 3
}

echo "building $TAG (BASE=$BASE_IMAGE RUNTIME_BASE=$RUNTIME_IMAGE)"
# The Dockerfile copies this test into the vendored SDK and runs it as a build
# gate, so it must be part of the build context.
mkdir -p "$src/amendment"
cp "$PATCH_FILE" "$src/amendment/"
cp amendment/sdk_dedup_profile_fetch_test.go "$src/amendment/"
cp amendment/Dockerfile.device-rest "$src/amendment/"

docker build \
  -f "$src/amendment/Dockerfile.device-rest" \
  --build-arg "BASE=$BASE_IMAGE" \
  --build-arg "RUNTIME_BASE=$RUNTIME_IMAGE" \
  --build-arg "DEVICE_REST_VERSION=$VERSION" \
  -t "$TAG" \
  "$src"

verify_image "$TAG"
echo "built $TAG"
echo "$TAG"
