#!/bin/bash

set -euo pipefail

COMMON_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../internal" && pwd)"
source "${COMMON_DIR}/e2e_common.sh"

feature_common_init registry-metadata
feature_usage() {
    echo "Usage: ./examples/registry/metadata/verify.sh --polaris-server <host> [--debug]"
}
feature_parse_transport_args "$@"

SERVICE_NAME="trpc.polaris.registry.metadata.Echo"
PROVIDER_PORT="${PROVIDER_PORT:-28271}"
CONFLICT_PORT="${CONFLICT_PORT:-28272}"
PROVIDER_VERSION="${PROVIDER_VERSION:-v1.2.3}"

build_registry_metadata() {
    (
        cd "${REPO_ROOT}"
        env GOCACHE="${GOCACHE:-/private/tmp/go-naming-polarismesh-cache}" \
            go build -o "${BUILD_DIR}/provider" ./examples/registry/metadata/provider
    )
}

start_dynamic_provider() {
    local workdir="${BUILD_DIR}/dynamic-run"
    rm -rf "${workdir}"
    mkdir -p "${workdir}"
    cp "${REPO_ROOT}/examples/registry/metadata/provider/trpc_go.yaml" \
        "${workdir}/trpc_go.yaml"
    local debug_flag=()
    if [[ "${DEBUG_MODE:-false}" == "true" ]]; then
        debug_flag=(-debug)
    fi
    (
        cd "${workdir}"
        exec nohup "${BUILD_DIR}/provider" \
            -conf trpc_go.yaml \
            -polaris-server "${POLARIS_SERVER}" \
            -polaris-port "${POLARIS_GRPC_PORT}" \
            -namespace "${NAMESPACE}" \
            -service "${SERVICE_NAME}" \
            -version "${PROVIDER_VERSION}" \
            -metadata-mode dynamic \
            -port "${PROVIDER_PORT}" \
            ${debug_flag[@]+"${debug_flag[@]}"}
    ) > "${LOG_DIR}/provider.log" 2>&1 &
    local pid=$!
    disown "${pid}" 2>/dev/null || true
    printf '%s|provider|%s\n' "${pid}" "${PROVIDER_PORT}" >> "${PID_FILE}"
}

case_dynamic_metadata() {
    local response
    response="$(api_request GET \
        "/naming/v1/instances?namespace=${NAMESPACE}&service=${SERVICE_NAME}")"
    if ! jq -e --arg version "${PROVIDER_VERSION}" --argjson port "${PROVIDER_PORT}" '
        [.instances[]? | select(. != null)] as $instances
        | ($instances | length) == 1
        and $instances[0].host != ""
        and $instances[0].port == $port
        and $instances[0].version == $version
        and $instances[0].metadata["registry.static"] == "provider-yaml"
        and $instances[0].metadata["registry.runtime"] == "dynamic"
        and $instances[0].metadata["registry.version"] == $version
        and $instances[0].metadata["ai-agent-name"] == "dynamic-registry-agent"
        and $instances[0].metadata["a2a.transport"] == "JSONRPC"
        and $instances[0].metadata["a2a.path"] == "/a2a"
        and ($instances[0].metadata["a2a.agent.card"] | length) > 0
    ' >/dev/null <<<"${response}"; then
        echo "dynamic metadata assertion failed: ${response}" >&2
        return 1
    fi
    echo "[case 1.1 dynamic metadata registration] PASS"
}

case_conflict_fail_fast() {
    local workdir="${BUILD_DIR}/conflict-run"
    local exit_code
    rm -rf "${workdir}"
    mkdir -p "${workdir}"
    cp "${REPO_ROOT}/examples/registry/metadata/provider/trpc_go-conflict.yaml" \
        "${workdir}/trpc_go.yaml"
    set +e
    (
        cd "${workdir}"
        timeout 20s "${BUILD_DIR}/provider" \
            -conf trpc_go.yaml \
            -polaris-server "${POLARIS_SERVER}" \
            -polaris-port "${POLARIS_GRPC_PORT}" \
            -namespace "${NAMESPACE}" \
            -service "${SERVICE_NAME}" \
            -version "${PROVIDER_VERSION}" \
            -metadata-mode conflict \
            -port "${CONFLICT_PORT}"
    ) > "${LOG_DIR}/provider-conflict.log" 2>&1
    exit_code=$?
    set -e
    if [[ "${exit_code}" -eq 0 || "${exit_code}" -eq 124 ]]; then
        echo "conflict provider exited ${exit_code}, expected fail-fast" >&2
        cat "${LOG_DIR}/provider-conflict.log" >&2
        return 1
    fi
    if ! grep -q "reserved agent key" "${LOG_DIR}/provider-conflict.log"; then
        echo "conflict provider failed without expected metadata error" >&2
        cat "${LOG_DIR}/provider-conflict.log" >&2
        return 1
    fi
    echo "[case 1.2 reserved metadata key conflict] PASS"
}

build_registry_metadata
ensure_namespace
ensure_service "${SERVICE_NAME}"
start_dynamic_provider
wait_for_instances "${SERVICE_NAME}" 1
case_dynamic_metadata
case_conflict_fail_fast
