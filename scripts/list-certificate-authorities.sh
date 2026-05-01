#!/usr/bin/env bash
# List Certificate Authorities (and their templates) in Workspace ONE UEM
# for the configured organization group, using the picklists API.
# Use this to discover the (Authority ID, Template ID) pair needed for a
# credentials_list entry with credential_source = "DefinedCertificateAuthority"
# in the macOS network-wifi module.
#
# Usage: ./list-certificate-authorities.sh [--env ENV_FILE]
#
# Examples:
#   ./list-certificate-authorities.sh                 # Use .env
#   ./list-certificate-authorities.sh --env .env.prod # Use .env.prod

# shellcheck source=scripts/common.sh disable=SC1091
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/common.sh"

ENV_FILE=".env"

while [[ $# -gt 0 ]]; do
    case $1 in
        --env)
            ENV_FILE="$2"
            shift 2
            ;;
        -h|--help)
            echo "Usage: $0 [--env ENV_FILE]"
            echo ""
            echo "Options:"
            echo "  --env FILE  Environment file to use (default: .env)"
            echo "  -h, --help  Show this help message"
            echo ""
            echo "Environment variables required (TF_VAR_* style, matching Terraform input variables):"
            echo "  TF_VAR_uem_instance_url"
            echo "  TF_VAR_uem_tenant_code"
            echo "  TF_VAR_uem_org_group_id  (the picklists API is org-group scoped)"
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
if [ -z "${TF_VAR_uem_org_group_id:-}" ]; then
    echo "Error: TF_VAR_uem_org_group_id must be set in $LOADED_ENV_FILE (the picklists API is org-group scoped)"
    exit 1
fi

# Re-export under the UEM_* names that the Go helper binary consumes.
export UEM_INSTANCE_URL="$TF_VAR_uem_instance_url"
export UEM_TENANT_CODE="$TF_VAR_uem_tenant_code"
export UEM_ORG_GROUP_ID="$TF_VAR_uem_org_group_id"
export UEM_AUTH_METHOD="${TF_VAR_uem_auth_method:-}"
export UEM_USERNAME="${TF_VAR_uem_username:-}"
export UEM_PASSWORD="${TF_VAR_uem_password:-}"
export UEM_CLIENT_ID="${TF_VAR_uem_client_id:-}"
export UEM_CLIENT_SECRET="${TF_VAR_uem_client_secret:-}"
export UEM_OAUTH2_TOKEN_URL="${TF_VAR_uem_oauth2_token_url:-}"

FRIENDLY_NAME="${TF_VAR_uem_friendly_name:-Unknown}"

echo "=================================================="
echo "Workspace ONE Certificate Authorities - $FRIENDLY_NAME"
echo "=================================================="
echo ""
echo "Environment: $LOADED_ENV_FILE"
echo "Instance: $UEM_INSTANCE_URL"
echo "Org Group ID: $UEM_ORG_GROUP_ID"
echo ""

echo "Building certificate-authority lister..."
OUTPUT_DIR="$REPO_ROOT/output"
mkdir -p "$OUTPUT_DIR"
go build -o "$OUTPUT_DIR/list-certificate-authorities" "$SCRIPTS_DIR/list-certificate-authorities.go" 2>/dev/null || {
    echo "Error: Failed to build list-certificate-authorities tool"
    exit 1
}

echo ""
echo "Fetching certificate authorities from Workspace ONE..."
echo ""

"$OUTPUT_DIR/list-certificate-authorities"

echo ""
echo "=================================================="
echo ""
echo "To use a (CA, template) pair in the macOS network-wifi module,"
echo "drop the values into a credentials_list entry in terraform.tfvars:"
echo ""
echo "  credentials_list = ["
echo "    {"
echo "      credential_source     = \"DefinedCertificateAuthority\""
echo "      credential_name       = \"corp-eaptls\"  # logical join key"
echo "      certificate_authority = <AUTHORITY ID from the table>"
echo "      certificate_template  = <TEMPLATE ID from the table>"
echo "    }"
echo "  ]"
echo ""
echo "Then reference it from the matching network_list entry:"
echo "  network_list = [{ ..., identity_certificate = \"corp-eaptls\" }]"
echo ""
