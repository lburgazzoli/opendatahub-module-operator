#!/bin/sh
set -eu

usage() {
    echo "Usage: $0 <module> [operator args...]" >&2
    echo "   or: ODH_MODULE=<module> $0 [operator args...]" >&2
    echo >&2
    echo "Available modules:" >&2
    sed 's/^/  - /' /opt/odh-modules/modules.txt >&2
}

module="${ODH_MODULE:-}"

if [ "${1:-}" = "--help" ] || [ "${1:-}" = "-h" ]; then
    usage
    exit 0
fi

if [ -z "${module}" ] && [ $# -gt 0 ] && grep -Fxq "$1" /opt/odh-modules/modules.txt; then
    module="$1"
    shift
fi

if [ -z "${module}" ]; then
    echo "no module selected" >&2
    usage
    exit 1
fi

if ! grep -Fxq "${module}" /opt/odh-modules/modules.txt; then
    echo "unknown module: ${module}" >&2
    usage
    exit 1
fi

export ODH_MODULE_OPERATOR_MANIFESTS_PATH="/opt/odh-modules/manifests/${module}"
exec "/opt/odh-modules/bin/${module}/manager" operator "$@"
