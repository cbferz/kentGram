#!/usr/bin/env bash
set -Eeuo pipefail

DOMAIN="api.kentgram.tendercheck.pro"
EXPECTED_IP="150.241.70.250"

echo "== kentGram VPS preflight =="
echo "Domain: $DOMAIN"
echo "Expected VPS IP: $EXPECTED_IP"
echo

if ! command -v getent >/dev/null 2>&1; then
  echo "ERROR: getent is not installed."
  exit 1
fi

RESOLVED_IP="$(getent ahostsv4 "$DOMAIN" | awk 'NR==1 {print $1}')"
if [[ -z "$RESOLVED_IP" ]]; then
  echo "ERROR: DNS lookup failed for $DOMAIN"
  exit 1
fi

echo "DNS IPv4: $RESOLVED_IP"
if [[ "$RESOLVED_IP" != "$EXPECTED_IP" ]]; then
  echo "WARNING: DNS does not currently resolve to the expected VPS IP."
else
  echo "OK: DNS points to the expected VPS IP."
fi

echo
echo "== Required ports (80/443) =="
if command -v ss >/dev/null 2>&1; then
  if ss -ltnp | grep -E ':(80|443)[[:space:]]'; then
    echo "WARNING: One or more web ports already have listeners. Do not stop them blindly."
  else
    echo "No listeners detected on TCP 80/443."
  fi
else
  echo "WARNING: ss is unavailable; cannot inspect listening ports."
fi

echo
echo "== Existing web services =="
for service in caddy nginx apache2; do
  if command -v systemctl >/dev/null 2>&1 && systemctl is-active --quiet "$service"; then
    echo "ACTIVE: $service"
  fi
done

echo
echo "== Docker =="
if command -v docker >/dev/null 2>&1; then
  docker --version
  docker compose version
else
  echo "Docker is not installed."
fi

echo
echo "Preflight only: no packages, services, firewall rules, or files were changed."
