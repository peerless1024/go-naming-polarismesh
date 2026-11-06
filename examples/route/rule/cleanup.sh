#!/bin/bash
set -euo pipefail
COMMON_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../internal" && pwd)"
source "${COMMON_DIR}/e2e_common.sh"
feature_cleanup_init route-rule
feature_parse_cleanup_args "$@"
NAMESPACE="go-naming-polarismesh-e2e"
SERVICE_NAME="trpc.polaris.e2e.RouteRule"
RULE_NAME="go-naming-polarismesh-e2e-route-rule"

status=0
if [[ "${DELETE_SERVICE}" == "true" && "${DRY_RUN}" != "true" ]]; then
    if response="$(api_request GET \
        "/naming/v2/routings?name=${RULE_NAME}&namespace=${NAMESPACE}&service=${SERVICE_NAME}&offset=0&limit=20")"; then
        if jq -e '.code == 200000' >/dev/null <<<"${response}"; then
            while IFS= read -r id; do
                [[ -n "${id}" ]] || continue
                delete_response="$(api_request POST /naming/v2/routings/delete \
                    "[{\"id\":\"${id}\",\"name\":\"${RULE_NAME}\",\"namespace\":\"${NAMESPACE}\"}]")" || true
                if ! api_success "${delete_response}"; then
                    disable_response="$(api_request PUT /naming/v2/routings/enable \
                        "[{\"id\":\"${id}\",\"enable\":false}]")" || true
                    if ! api_success "${disable_response}"; then
                        echo "delete or disable route rule ${id} failed" >&2
                        status=1
                    fi
                fi
            done < <(jq -r --arg name "${RULE_NAME}" \
                '.data[]? | select(.name == $name) | .id' <<<"${response}" 2>/dev/null || true)
        else
            echo "query route rule ${RULE_NAME} failed: ${response}" >&2
            status=1
        fi
    else
        echo "query route rule ${RULE_NAME} failed" >&2
        status=1
    fi
fi
if [[ "${DELETE_SERVICE}" == "true" && "${DRY_RUN}" == "true" ]]; then
    echo "Remote rule: ${RULE_NAME}"
fi

feature_common_cleanup "${SERVICE_NAME}" || status=1
exit "${status}"
