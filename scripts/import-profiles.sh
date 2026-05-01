#!/usr/bin/env bash
# Import existing Workspace ONE profiles into Terraform state
#
# Usage: ./import-profiles.sh [--env ENV_FILE] <resource_name> <profile_id> <platform>
#
# Examples:
#   ./import-profiles.sh android_corporate 12345 Android
#   ./import-profiles.sh --env .env.prod android_corporate 12345 Android
#   ./import-profiles.sh ios_profile 58228 "Apple iOS"

# shellcheck source=scripts/common.sh disable=SC1091
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/common.sh"

# Terraform working directory for imports
TF_DIR="$REPO_ROOT/examples/resources/uem_profile/import-existing"

# Default values
ENV_FILE=".env"

# Parse command line arguments
if [ "${1:-}" = "--env" ]; then
    ENV_FILE="$2"
    shift 2
fi

if [ $# -ne 3 ]; then
    echo "Usage: $0 [--env ENV_FILE] <resource_name> <profile_id> <platform>"
    echo ""
    echo "Options:"
    echo "  --env FILE        Environment file to use (default: .env)"
    echo ""
    echo "Platforms:"
    echo "  Android, Apple iOS, AppleOsX, Windows 10, Windows_Rugged, Linux"
    echo ""
    echo "Examples:"
    echo "  $0 android_corporate 12345 Android"
    echo "  $0 --env .env.prod android_corporate 12345 Android"
    echo "  $0 ios_profile 54321 \"Apple iOS\""
    echo ""
    echo "This will import the profile as uem_profile.<resource_name>"
    exit 1
fi

ORIGINAL_NAME=$1
PROFILE_ID=$2
PLATFORM=$3

# Sanitize the resource name for Terraform (via common.sh)
RESOURCE_NAME=$(sanitize_tf_name "$ORIGINAL_NAME")

# Remove stale temp file if the name was sanitized (from a previous failed run)
if [[ "$RESOURCE_NAME" != "$ORIGINAL_NAME" ]]; then
    rm -f "$TF_DIR/import-temp-${ORIGINAL_NAME}.tf"
fi

# Load environment
load_env "$ENV_FILE"

# Get UEM_FRIENDLY_NAME from env file
UEM_FRIENDLY_NAME="${UEM_FRIENDLY_NAME:-Unknown}"

echo "=================================================="
echo "Importing Profile - $UEM_FRIENDLY_NAME"
echo "=================================================="
echo ""
echo "Environment: $LOADED_ENV_FILE"
echo "Resource Name: uem_profile.$RESOURCE_NAME"
echo "Profile ID: $PROFILE_ID"
echo "Platform: $PLATFORM"
echo "Terraform Dir: $TF_DIR"
echo ""

# Ensure a resource block exists for the import target
IMPORT_TF="$TF_DIR/import-temp-$RESOURCE_NAME.tf"
CREATED_TF=false
if ! grep -rq "\"uem_profile\" \"$RESOURCE_NAME\"" "$TF_DIR"/*.tf 2>/dev/null; then
    echo "Creating temporary resource block for uem_profile.$RESOURCE_NAME..."
    cat > "$IMPORT_TF" <<EOF
resource "uem_profile" "$RESOURCE_NAME" {
  name         = "imported"
  platform     = "$PLATFORM"
  org_group_id = "${UEM_ORG_GROUP_ID:-0}"
}
EOF
    CREATED_TF=true
fi

# Initialize Terraform. If already initialized, remove .terraform and lock file
# so a rebuilt local provider gets a fresh copy and checksum (avoids "does not match checksums" error).
if [ -d "$TF_DIR/.terraform" ]; then
    rm -rf "$TF_DIR/.terraform" "$TF_DIR/.terraform.lock.hcl"
fi
echo "Initializing Terraform..."
terraform -chdir="$TF_DIR" init
echo ""

# Remove existing state entry if present (allows re-importing)
terraform -chdir="$TF_DIR" state rm "uem_profile.$RESOURCE_NAME" 2>/dev/null || true

# Run the import
terraform -chdir="$TF_DIR" import "uem_profile.$RESOURCE_NAME" "$PROFILE_ID:$PLATFORM"

echo ""
echo "Import successful! Generating complete resource configuration..."
echo ""

# ---------------------------------------------------------------------------
# Generate a complete .tf file from the imported state so brownfield customers
# have all settings in their config and can modify them after import.
# ---------------------------------------------------------------------------
generate_tf_from_state() {
    local resource_name="$1"
    local output_file="$2"

    if ! command -v jq &>/dev/null; then
        echo "WARNING: jq is not installed. Skipping auto-generation of complete .tf file."
        echo "         Install jq and re-run, or manually run 'terraform show' to see imported values."
        return 1
    fi

    local json
    json=$(terraform -chdir="$TF_DIR" show -json 2>/dev/null) || {
        echo "WARNING: Failed to read Terraform state as JSON. Skipping .tf generation."
        return 1
    }

    local values
    values=$(echo "$json" | jq -r --arg addr "uem_profile.$resource_name" '
        .values.root_module.resources[]
        | select(.address == $addr)
        | .values
    ' 2>/dev/null) || {
        echo "WARNING: Could not find resource uem_profile.$resource_name in state."
        return 1
    }

    if [ -z "$values" ] || [ "$values" = "null" ]; then
        echo "WARNING: Resource uem_profile.$resource_name has no values in state."
        return 1
    fi

    # Helper: check if a key exists and is not null in the values JSON
    key_exists() {
        local json="$1" attr="$2"
        echo "$json" | jq -e --arg a "$attr" 'has($a) and .[$a] != null' >/dev/null 2>&1
    }

    # Helper: emit a string attribute line (includes empty strings to avoid drift)
    emit_string() {
        local attr="$1"
        if key_exists "$values" "$attr"; then
            local val
            val=$(echo "$values" | jq -r --arg a "$attr" '.[$a]')
            printf '  %-21s = %s\n' "$attr" "$(printf '"%s"' "$val")"
        fi
    }

    # Helper: emit a bool attribute line (handles false correctly)
    emit_bool() {
        local attr="$1"
        if key_exists "$values" "$attr"; then
            local val
            val=$(echo "$values" | jq -r --arg a "$attr" '.[$a]')
            printf '  %-21s = %s\n' "$attr" "$val"
        fi
    }

    # Helper: emit a string list attribute
    emit_string_list() {
        local attr="$1"
        if ! key_exists "$values" "$attr"; then
            return
        fi
        local arr
        arr=$(echo "$values" | jq -c --arg a "$attr" '.[$a]')
        if [ "$arr" != "[]" ]; then
            local items
            items=$(echo "$arr" | jq -r '.[] | "    \"" + . + "\","')
            printf '  %s = [\n%s\n  ]\n' "$attr" "$items"
        fi
    }

    # Helper: emit a nested block with bool/string/int64 attributes
    emit_nested_block() {
        local block_name="$1"
        shift
        local block_json
        if ! key_exists "$values" "$block_name"; then
            return
        fi
        block_json=$(echo "$values" | jq -c --arg b "$block_name" '.[$b]')

        printf '\n  %s = {\n' "$block_name"

        for attr_spec in "$@"; do
            local attr_name="${attr_spec%%:*}"
            local attr_type="${attr_spec##*:}"
            if ! key_exists "$block_json" "$attr_name"; then
                continue
            fi
            local val
            val=$(echo "$block_json" | jq -r --arg a "$attr_name" '.[$a]')
            case "$attr_type" in
                string)
                    printf '    %-43s = %s\n' "$attr_name" "$(printf '"%s"' "$val")"
                    ;;
                sensitive)
                    printf '    %-43s = %s\n' "$attr_name" "$(printf '"%s"' "$val")"
                    ;;
                bool)
                    printf '    %-43s = %s\n' "$attr_name" "$val"
                    ;;
                int)
                    printf '    %-43s = %s\n' "$attr_name" "$val"
                    ;;
                string_list)
                    local items
                    items=$(echo "$block_json" | jq -c --arg a "$attr_name" '.[$a]')
                    if [ "$items" != "[]" ]; then
                        local list_items
                        list_items=$(echo "$items" | jq -r '.[] | "      \"" + . + "\","')
                        printf '    %s = [\n%s\n    ]\n' "$attr_name" "$list_items"
                    fi
                    ;;
            esac
        done

        printf '  }\n'
    }

    {
        printf '# Imported from profile ID %s (%s)\n' "$PROFILE_ID" "$PLATFORM"
        printf '# Generated by import-profiles.sh on %s\n' "$(date '+%Y-%m-%d %H:%M:%S')"
        printf 'resource "uem_profile" "%s" {\n' "$resource_name"

        # Required attributes
        emit_string "name"
        emit_string "platform"
        emit_string "org_group_id"

        # Optional general attributes
        emit_string "description"
        emit_string "assignment_type"
        emit_string "profile_scope"
        emit_bool   "is_active"
        emit_string "profile_context"

        # Android-specific
        emit_string "lock_screen_message"

        # Smart groups
        emit_string_list "assigned_smart_groups"

        # Passcode block (macOS / iOS)
        emit_nested_block "passcode" \
            "require_passcode_on_device:bool" \
            "allow_simple_value:bool" \
            "require_alphanumeric_value:bool" \
            "minimum_passcode_length:int" \
            "minimum_number_of_complex_characters:string" \
            "maximum_passcode_age:string" \
            "auto_lock:string" \
            "grace_period:int" \
            "max_failed_attempts:int" \
            "pin_history:int" \
            "minutes_until_failed_login_reset:int"

        # Network block (AppleOsX only)
        emit_nested_block "network" \
            "network_interface:string" \
            "service_set_identifier:string" \
            "hidden_network:bool" \
            "auto_join:bool" \
            "security_type:string" \
            "password:sensitive" \
            "use_as_login_window_configuration:bool" \
            "use_directory_authentication:bool" \
            "disable_association_mac_randomization:bool" \
            "tls:bool" \
            "ttls:bool" \
            "leap:bool" \
            "peap:bool" \
            "eap_fast:bool" \
            "eap_sim:bool" \
            "eap_aka:bool" \
            "tls_minimum_version:string" \
            "tls_maximum_version:string" \
            "user_name:string" \
            "user_password:sensitive" \
            "identity_certificate:string" \
            "inner_identity:string" \
            "outer_identity:string" \
            "use_pac:bool" \
            "allow_two_rands:bool" \
            "trusted_certificates:string_list" \
            "allow_trust_exceptions:bool" \
            "proxy_type:string" \
            "proxy_server:string" \
            "proxy_server_port:int" \
            "proxy_username:string" \
            "proxy_password:sensitive" \
            "proxy_url:string" \
            "pac_fallback:bool"

        printf '}\n'
    } > "$output_file"

    return 0
}

if generate_tf_from_state "$RESOURCE_NAME" "$IMPORT_TF"; then
    echo "Generated complete resource configuration: $IMPORT_TF"
    echo ""
    cat "$IMPORT_TF"
    echo ""
    echo "=================================================="
    echo "Import complete!"
    echo "=================================================="
    echo ""
    echo "Next steps:"
    echo "1. Review the generated file: $IMPORT_TF"
    echo "2. Run 'set -a && source $ENV_FILE && set +a && terraform -chdir=$TF_DIR plan' to verify no drift"
    echo "3. Modify settings in the .tf file as needed, then run 'terraform apply' to push changes"
else
    echo ""
    echo "=================================================="
    echo "Import complete (manual configuration needed)"
    echo "=================================================="
    echo ""
    echo "Next steps:"
    echo "1. Run 'terraform -chdir=$TF_DIR show' to see the imported profile's actual values"
    echo "2. Update the resource block in $IMPORT_TF to match the actual profile configuration"
    echo "3. Run 'set -a && source $ENV_FILE && set +a && terraform -chdir=$TF_DIR plan' to verify"
fi
