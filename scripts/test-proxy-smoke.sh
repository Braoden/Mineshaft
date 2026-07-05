#!/bin/bash

# Quick smoke test for ms-proxy
# Minimal setup to verify basic functionality

set -e

PROXY_PORT=${PROXY_PORT:-9876}
PROXY_ADMIN_PORT=${PROXY_ADMIN_PORT:-9877}
TOWN_ROOT=${TOWN_ROOT:-${HOME}/ms}
TEST_DIR="/tmp/ms-proxy-quick-test"

cleanup() {
    echo "Cleaning up..."
    if [[ -n "${SERVER_PID}" ]] && kill -0 "${SERVER_PID}" 2>/dev/null; then
        kill "${SERVER_PID}" 2>/dev/null || true
        wait "${SERVER_PID}" 2>/dev/null || true
    fi
    rm -rf "${TEST_DIR}"
}
trap cleanup EXIT

rm -rf "${TEST_DIR}"
mkdir -p "${TEST_DIR}/ca" "${TEST_DIR}/bin"

echo "=== Building binaries..."
go build -o "${TEST_DIR}/bin/ms-proxy-server" ./cmd/ms-proxy-server
go build -o "${TEST_DIR}/bin/ms-proxy-client" ./cmd/ms-proxy-client
echo "OK Binaries built"

echo ""
echo "=== Starting proxy server on port ${PROXY_PORT}..."
"${TEST_DIR}/bin/ms-proxy-server" \
    --listen "127.0.0.1:${PROXY_PORT}" \
    --admin-listen "127.0.0.1:${PROXY_ADMIN_PORT}" \
    --ca-dir "${TEST_DIR}/ca" \
    --town-root "${TOWN_ROOT}" \
    > "${TEST_DIR}/server.log" 2>&1 &
SERVER_PID=$!

# Wait for admin port to be ready
for i in $(seq 1 30); do
    if lsof -nP -iTCP:"${PROXY_ADMIN_PORT}" -sTCP:LISTEN >/dev/null 2>&1; then
        break
    fi
    if ! kill -0 "${SERVER_PID}" 2>/dev/null; then
        echo "FAIL Server exited prematurely"
        cat "${TEST_DIR}/server.log"
        exit 1
    fi
    sleep 0.2
done

echo "OK Server started (PID: ${SERVER_PID})"

echo ""
echo "=== Testing server health..."
if lsof -nP -iTCP:"${PROXY_PORT}" -sTCP:LISTEN >/dev/null 2>&1; then
    echo "OK Server listening on port ${PROXY_PORT}"
else
    echo "FAIL Server not listening on port ${PROXY_PORT}"
    cat "${TEST_DIR}/server.log"
    exit 1
fi

echo ""
echo "=== Issuing test certificate via admin API..."
CERT_RESPONSE=$(curl -sf \
    -X POST \
    -H "Content-Type: application/json" \
    -d '{"rig":"testrig","name":"testminer"}' \
    "http://127.0.0.1:${PROXY_ADMIN_PORT}/v1/admin/issue-cert")

if [[ -z "${CERT_RESPONSE}" ]]; then
    echo "FAIL Failed to issue certificate"
    cat "${TEST_DIR}/server.log" | tail -10
    exit 1
fi

# Extract cert, key, and CA from JSON response and write to files
echo "${CERT_RESPONSE}" | python3 -c "
import sys, json
d = json.load(sys.stdin)
open('${TEST_DIR}/client.crt', 'w').write(d['cert'])
open('${TEST_DIR}/client.key', 'w').write(d['key'])
open('${TEST_DIR}/ca-client.crt', 'w').write(d['ca'])
print(d['cn'])
" && echo "OK Certificate issued"
chmod 600 "${TEST_DIR}/client.key"

echo ""
echo "=== Testing mTLS request (ms version)..."
# The server cert has DNS SAN 'ms-proxy-server', so we use --resolve to map that hostname.
HTTP_CODE=$(curl -s -o "${TEST_DIR}/response.json" -w "%{http_code}" \
    --cert "${TEST_DIR}/client.crt" \
    --key "${TEST_DIR}/client.key" \
    --cacert "${TEST_DIR}/ca-client.crt" \
    --resolve "ms-proxy-server:${PROXY_PORT}:127.0.0.1" \
    --max-time 10 \
    -X POST \
    -H "Content-Type: application/json" \
    -d '{"argv":["ms","version"]}' \
    "https://ms-proxy-server:${PROXY_PORT}/v1/exec" 2>"${TEST_DIR}/curl_err.log" || echo "000")

if [[ "${HTTP_CODE}" == "000" ]]; then
    echo "FAIL mTLS connection failed"
    cat "${TEST_DIR}/curl_err.log"
    cat "${TEST_DIR}/server.log" | tail -10
    exit 1
elif [[ "${HTTP_CODE}" == "200" ]]; then
    echo "OK mTLS request successful (HTTP 200)"
    echo "  Response: $(cat "${TEST_DIR}/response.json" | head -c 120)"
else
    echo "OK mTLS connected (HTTP ${HTTP_CODE})"
    echo "  Response: $(cat "${TEST_DIR}/response.json" | head -c 120)"
fi

echo ""
echo "=== Testing ms-proxy-client end-to-end..."
# Symlink ms-proxy-client as 'ms' and run it with proxy env vars.
# The client uses toolNameFromArg0 to determine argv[0] (= "ms"),
# then sends {"argv":["ms","version",...]} to the proxy.
ln -sf "${TEST_DIR}/bin/ms-proxy-client" "${TEST_DIR}/bin/ms"

export MS_PROXY_URL="https://ms-proxy-server:${PROXY_PORT}"
export MS_PROXY_CERT="${TEST_DIR}/client.crt"
export MS_PROXY_KEY="${TEST_DIR}/client.key"
export MS_PROXY_CA="${TEST_DIR}/ca-client.crt"

# Add a /etc/hosts-style resolve for the ms-proxy-server hostname.
# The Go TLS client in ms-proxy-client verifies the server cert's DNS SAN,
# so we need the hostname to resolve to 127.0.0.1. Since we can't use
# curl's --resolve here, we set MS_PROXY_URL to the IP but tell the TLS
# client to expect the SAN hostname. Actually, the simpler approach: the
# server cert includes 127.0.0.1 as an IP SAN, so connect by IP.
export MS_PROXY_URL="https://127.0.0.1:${PROXY_PORT}"

E2E_OUTPUT=$("${TEST_DIR}/bin/ms" version 2>"${TEST_DIR}/e2e_err.log" || true)
E2E_ERR=$(cat "${TEST_DIR}/e2e_err.log" 2>/dev/null)

if [[ -n "${E2E_OUTPUT}" ]]; then
    echo "OK ms-proxy-client e2e passed"
    echo "  stdout: ${E2E_OUTPUT:0:120}"
else
    # The client may print errors to stderr — check if TLS connected at all
    if echo "${E2E_ERR}" | grep -q "proxy request failed"; then
        echo "FAIL ms-proxy-client could not connect to proxy"
        echo "  stderr: ${E2E_ERR:0:200}"
        cat "${TEST_DIR}/server.log" | tail -5
        exit 1
    else
        echo "OK ms-proxy-client connected (non-zero exit is expected if ms has no town)"
        echo "  stderr: ${E2E_ERR:0:200}"
    fi
fi

echo ""
echo "=== All smoke tests passed ==="
