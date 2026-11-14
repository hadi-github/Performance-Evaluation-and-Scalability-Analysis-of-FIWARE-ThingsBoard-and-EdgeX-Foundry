#!/usr/bin/env bash
# Allow the monitoring ports on the SUT only from the Prometheus host (ramonavm1); the platform API ports stay open.
#   ./firewall.sh apply     add the rules now (idempotent)
#   ./firewall.sh install   apply + install a systemd unit that re-applies them after Docker starts (survives reboot)
#   ./firewall.sh remove    delete the rules
#
# node-exporter uses the host network (INPUT chain); the other exporters and HAProxy's stats/metrics port 8404 are
# Docker-published, and Docker's DNAT bypasses INPUT, so those go into DOCKER-USER, matched on the original port.
set -euo pipefail
# The allowed source is the load host. Read it from the deployment configuration
# so a new network does not silently leave the exporters firewalled off.
_here="$(cd "$(dirname "$0")" && pwd)"
if [[ -z "${ALLOW_FROM:-}" && -f "$_here/../../.env" ]]; then
  # shellcheck source=../iotbench-env.sh
  source "$_here/../iotbench-env.sh" 2>/dev/null || true
fi
ALLOW="${ALLOW_FROM:-${LOAD_IP:-87.107.191.32}}"
HOST_PORTS="9100"
DOCKER_PORTS="8089 9187 9216 9308 7777 8404"
TAG="iot-bench-monitoring"

# One rule per port. $1 = -C (check) | -I (insert) | -D (delete)
rule() {
  local op="$1" chain="$2" port="$3"
  if [[ "$chain" == INPUT ]]; then
    iptables "$op" INPUT -p tcp --dport "$port" ! -s "$ALLOW" -m comment --comment "$TAG" -j DROP
  else
    iptables "$op" DOCKER-USER -p tcp -m conntrack --ctorigdstport "$port" --ctdir ORIGINAL ! -s "$ALLOW" \
      -m comment --comment "$TAG" -j DROP
  fi
}

apply() {
  iptables -L DOCKER-USER -n >/dev/null 2>&1 || iptables -N DOCKER-USER
  for p in $HOST_PORTS; do rule -C INPUT "$p" 2>/dev/null || rule -I INPUT "$p"; done
  for p in $DOCKER_PORTS; do rule -C DOCKER-USER "$p" 2>/dev/null || rule -I DOCKER-USER "$p"; done
  echo "monitoring ports ($HOST_PORTS $DOCKER_PORTS) restricted to $ALLOW"
}

remove() {
  for p in $HOST_PORTS; do while rule -D INPUT "$p" 2>/dev/null; do :; done; done
  for p in $DOCKER_PORTS; do while rule -D DOCKER-USER "$p" 2>/dev/null; do :; done; done
  echo "rules removed"
}

case "${1:-}" in
  apply) apply ;;
  remove) remove ;;
  install)
    apply
    install -m 0755 "$0" /usr/local/sbin/iot-bench-firewall
    cat > /etc/systemd/system/iot-bench-firewall.service <<UNIT
[Unit]
Description=Restrict IoT benchmark monitoring ports to the Prometheus host
After=docker.service
Requires=docker.service

[Service]
Type=oneshot
Environment=ALLOW_FROM=$ALLOW
ExecStart=/usr/local/sbin/iot-bench-firewall apply
RemainAfterExit=yes

[Install]
WantedBy=multi-user.target
UNIT
    systemctl daemon-reload
    systemctl enable iot-bench-firewall.service >/dev/null
    echo "installed iot-bench-firewall.service (re-applied after every Docker start)"
    ;;
  *) sed -n '2,8p' "$0" | sed 's/^# \{0,1\}//'; exit 2 ;;
esac
