#!/bin/bash
set -euo pipefail
COMMON_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../internal" && pwd)"
source "${COMMON_DIR}/e2e_common.sh"
feature_cleanup_init circuitbreaker
feature_parse_cleanup_args "$@"

SERVICE_NAME="trpc.polaris.e2e.CircuitBreaker"
status=0
if [[ "${DELETE_SERVICE}" == "true" && "${DRY_RUN}" != "true" ]]; then
    for name in \
        go-naming-polarismesh-e2e-cb-instance \
        go-naming-polarismesh-e2e-cb-service \
        go-naming-polarismesh-e2e-cb-method; do
        if ! delete_rule_by_name "${name}"; then
            echo "delete rule ${name} failed" >&2
            status=1
        fi
    done
fi
if [[ "${DELETE_SERVICE}" == "true" && "${DRY_RUN}" == "true" ]]; then
    echo "Remote rules: go-naming-polarismesh-e2e-cb-{instance,service,method}"
fi
feature_common_cleanup "${SERVICE_NAME}" || status=1
exit "${status}"
