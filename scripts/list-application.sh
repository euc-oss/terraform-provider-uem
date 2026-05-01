#!/usr/bin/env bash
# List applications in Workspace ONE UEM.
# This helps you find Application IDs/UUIDs and metadata for Terraform inputs.
#
# Usage: ./list-application.sh [--env ENV_FILE] [--platform PLATFORM] [--app-type TYPE] [--page-size N]
#
# Examples:
#   ./list-application.sh
#   ./list-application.sh --platform AppleOsX
#   ./list-application.sh --app-type INTERNAL
#   ./list-application.sh --platform AppleOsX --app-type INTERNAL --page-size 200
#   ./list-application.sh --env .env.prod --platform AppleOsX

# shellcheck source=scripts/common.sh disable=SC1091
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/common.sh"

ENV_FILE=".env"
PLATFORM=""
APP_TYPE=""
PAGE_SIZE=""

while [[ $# -gt 0 ]]; do
    case $1 in
        --env)
            ENV_FILE="$2"
            shift 2
            ;;
        --platform)
            PLATFORM="$2"
            shift 2
            ;;
        --app-type)
            APP_TYPE="$2"
            shift 2
            ;;
        --page-size)
            PAGE_SIZE="$2"
            shift 2
            ;;
        -h|--help)
            echo "Usage: $0 [--env ENV_FILE] [--platform PLATFORM] [--app-type TYPE] [--page-size N]"
            echo ""
            echo "Options:"
            echo "  --env FILE        Environment file to use (default: .env)"
            echo "  --platform NAME   Filter by platform (Android, Apple iOS, AppleOsX, Windows 10, etc.)"
            echo "  --app-type TYPE   Filter by application type (INTERNAL, PUBLIC, etc.)"
            echo "  --page-size N     Number of results to fetch per page (max 500)"
            echo "  -h, --help        Show this help message"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

load_env "$ENV_FILE"

if [ -z "${TF_VAR_uem_instance_url:-}" ] || [ -z "${TF_VAR_uem_tenant_code:-}" ]; then
    echo "Error: TF_VAR_uem_instance_url and TF_VAR_uem_tenant_code must be set in $LOADED_ENV_FILE"
    exit 1
fi

# Re-export under the UEM_* names that the Go helper binary consumes.
export UEM_INSTANCE_URL="$TF_VAR_uem_instance_url"
export UEM_TENANT_CODE="$TF_VAR_uem_tenant_code"
export UEM_AUTH_METHOD="${TF_VAR_uem_auth_method:-}"
export UEM_USERNAME="${TF_VAR_uem_username:-}"
export UEM_PASSWORD="${TF_VAR_uem_password:-}"
export UEM_CLIENT_ID="${TF_VAR_uem_client_id:-}"
export UEM_CLIENT_SECRET="${TF_VAR_uem_client_secret:-}"
export UEM_OAUTH2_TOKEN_URL="${TF_VAR_uem_oauth2_token_url:-}"
export UEM_ORG_GROUP_ID="${TF_VAR_uem_org_group_id:-}"

FRIENDLY_NAME="${TF_VAR_uem_friendly_name:-Unknown}"

echo "=================================================="
echo "Workspace ONE Applications - $FRIENDLY_NAME"
echo "=================================================="
echo ""
echo "Environment: $LOADED_ENV_FILE"
echo "Instance: $UEM_INSTANCE_URL"
echo "Org Group ID: ${UEM_ORG_GROUP_ID:-Not Set}"

if [ -n "$PLATFORM" ]; then
    echo "Platform Filter: $PLATFORM"
fi
if [ -n "$APP_TYPE" ]; then
    echo "Application Type Filter: $APP_TYPE"
fi
if [ -n "$PAGE_SIZE" ]; then
    echo "Page Size: $PAGE_SIZE"
fi
echo ""

echo "Building application lister..."
OUTPUT_DIR="$REPO_ROOT/output"
mkdir -p "$OUTPUT_DIR"
go build -o "$OUTPUT_DIR/list-application" "$SCRIPTS_DIR/list-application.go" 2>/dev/null || {
    echo "Error: Failed to build list-application tool"
    exit 1
}

echo ""
echo "Fetching applications from Workspace ONE..."
echo ""

CMD=("$OUTPUT_DIR/list-application")
if [ -n "$PLATFORM" ]; then
    CMD+=(--platform "$PLATFORM")
fi
if [ -n "$APP_TYPE" ]; then
    CMD+=(--app-type "$APP_TYPE")
fi
if [ -n "$PAGE_SIZE" ]; then
    CMD+=(--page-size "$PAGE_SIZE")
fi

"${CMD[@]}"

echo ""
echo "=================================================="
