#!/usr/bin/env bash
# List all profiles in Workspace ONE UEM
# This helps you find Profile IDs for importing into Terraform
#
# Usage: ./list-profiles.sh [--env ENV_FILE] [--platform PLATFORM]
#
# Examples:
#   ./list-profiles.sh                                    # Use .env, list all profiles
#   ./list-profiles.sh --env .env.prod                    # Use .env.prod
#   ./list-profiles.sh --platform Android                 # List only Android profiles
#   ./list-profiles.sh --env .env.prod --platform Android # Use .env.prod, Android only

# shellcheck source=scripts/common.sh disable=SC1091
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/common.sh"

# Default values
ENV_FILE=".env"
PLATFORM=""

# Parse command line arguments
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
        -h|--help)
            echo "Usage: $0 [--env ENV_FILE] [--platform PLATFORM]"
            echo ""
            echo "Options:"
            echo "  --env FILE        Environment file to use (default: .env)"
            echo "  --platform NAME   Filter by platform (Android, Apple iOS, AppleOsX, Windows 10, etc.)"
            echo "  -h, --help        Show this help message"
            echo ""
            echo "Examples:"
            echo "  $0                                    # Use .env, list all profiles"
            echo "  $0 --env .env.prod                    # Use .env.prod"
            echo "  $0 --platform Android                 # List only Android profiles"
            echo "  $0 --env .env.prod --platform Android # Use .env.prod, Android only"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

# Load environment
load_env "$ENV_FILE"

# Validate required environment variables (TF_VAR_* style, matching Terraform input variables)
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
echo "Workspace ONE Profiles - $FRIENDLY_NAME"
echo "=================================================="
echo ""
echo "Environment: $LOADED_ENV_FILE"
echo "Instance: $UEM_INSTANCE_URL"
echo "Org Group ID: ${UEM_ORG_GROUP_ID:-Not Set}"
echo ""

if [ -n "$PLATFORM" ]; then
    echo "Platform Filter: $PLATFORM"
    echo ""
fi

# Build the Go program to list profiles
echo "Building profile lister..."
OUTPUT_DIR="$REPO_ROOT/output"
mkdir -p "$OUTPUT_DIR"
go build -o "$OUTPUT_DIR/list-profiles" "$SCRIPTS_DIR/list-profiles.go" 2>/dev/null || {
    echo "Error: Failed to build list-profiles tool"
    exit 1
}

# Run the profile lister
echo ""
echo "Fetching profiles from Workspace ONE..."
echo ""

if [ -n "$PLATFORM" ]; then
    "$OUTPUT_DIR/list-profiles" --platform "$PLATFORM"
else
    "$OUTPUT_DIR/list-profiles"
fi

echo ""
echo "=================================================="
echo ""
echo "To import a profile into Terraform:"
echo "  1. Note the Profile ID and Platform from the list above."
echo "  2. In the matching module's terraform.tfvars, add an entry to"
echo "     the 'profiles' map with:"
echo "       import_id = \"<PROFILE_ID>:<PLATFORM>\""
echo "     where <PLATFORM> is one of:"
echo "       AppleOsX     (macOS)"
echo "       Apple        (Apple iOS / iPadOS)"
echo "       Android      (Android)"
echo "       WinRT        (Windows 10/11 desktop)"
echo "       Linux        (Linux)"
echo "  3. Run 'terraform apply' from inside the module directory."
echo "     ('terraform init' is not required while the omnissa/uem"
echo "      provider is loaded via dev_overrides from dev.tfrc.)"
echo ""
echo "Example tfvars entry (macOS passcode profile):"
echo "  profiles = {"
echo "    corporate = {"
echo "      name      = \"macOS Passcode Policy\""
echo "      import_id = \"57996:AppleOsX\""
echo "    }"
echo "  }"
echo ""
echo "See the per-module README for the exact tfvars schema."
