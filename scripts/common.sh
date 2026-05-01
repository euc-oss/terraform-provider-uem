#!/usr/bin/env bash
# common.sh - Shared preamble for all project scripts
#
# Source this at the top of any script:
#   source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/common.sh"
#
# Provides:
#   REPO_ROOT     - Absolute path to repository root (via git rev-parse)
#   SCRIPTS_DIR   - Absolute path to scripts/ directory
#   PLATFORM      - One of: darwin, linux, wsl, windows
#   Color vars    - RED, GREEN, YELLOW, BLUE, CYAN, NC (empty when not a tty)
#   load_env()    - Load .env file from repo root (accepts optional path argument)
#   print_status  - Print pass/fail status line
#   print_section - Print a section header
#   require_cmd   - Check that a command exists or exit with message

set -euo pipefail

# ---------------------------------------------------------------------------
# Repository root — the single source of truth for all path resolution
# ---------------------------------------------------------------------------
REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null)" || {
    echo "Error: not inside a git repository" >&2
    exit 1
}
export REPO_ROOT

SCRIPTS_DIR="$REPO_ROOT/scripts"
export SCRIPTS_DIR

# ---------------------------------------------------------------------------
# Platform detection: darwin | linux | wsl
# ---------------------------------------------------------------------------
_detect_platform() {
    local uname_s
    uname_s="$(uname -s)"
    case "$uname_s" in
        Darwin)  echo "darwin" ;;
        Linux)
            # WSL exposes Microsoft in /proc/version
            if grep -qi 'microsoft\|wsl' /proc/version 2>/dev/null; then
                echo "wsl"
            else
                echo "linux"
            fi
            ;;
        MINGW*|MSYS*|CYGWIN*)
            echo "windows"
            ;;
        *)
            echo "linux"  # safe fallback
            ;;
    esac
}
PLATFORM="$(_detect_platform)"
export PLATFORM

# ---------------------------------------------------------------------------
# Colors — disabled when output is not a terminal or NO_COLOR is set
# See https://no-color.org/
# ---------------------------------------------------------------------------
if [[ -t 1 ]] && [[ -z "${NO_COLOR:-}" ]]; then
    RED='\033[0;31m'
    GREEN='\033[0;32m'
    YELLOW='\033[1;33m'
    BLUE='\033[0;34m'
    CYAN='\033[0;36m'
    NC='\033[0m'
else
    RED=''
    GREEN=''
    YELLOW=''
    BLUE=''
    CYAN=''
    NC=''
fi
export RED GREEN YELLOW BLUE CYAN NC

# ---------------------------------------------------------------------------
# Helper: load_env [FILE]
#
# Loads an .env file. If FILE is not given, loads $REPO_ROOT/.env.
# Relative paths are resolved against CWD first, then REPO_ROOT.
#
# Skip behaviour: if TF_VAR_uem_instance_url is already exported in the
# environment, the env file is NOT sourced — the caller (CI, direnv, the
# parent shell) has already supplied configuration and we must not
# overwrite it with potentially stale values from .env. The env-file
# argument is then ignored, even if the file is missing.
# ---------------------------------------------------------------------------
load_env() {
    local env_file="${1:-.env}"

    # If the caller already set TF_VAR_uem_instance_url, treat the
    # current process environment as authoritative and skip the file.
    if [[ -n "${TF_VAR_uem_instance_url:-}" ]]; then
        LOADED_ENV_FILE="<process environment>"
        export LOADED_ENV_FILE
        return 0
    fi

    # Resolve relative paths: try CWD, then REPO_ROOT
    if [[ ! -f "$env_file" ]]; then
        if [[ -f "$REPO_ROOT/$env_file" ]]; then
            env_file="$REPO_ROOT/$env_file"
        else
            echo -e "${RED}Error: Environment file '$env_file' not found${NC}" >&2
            return 1
        fi
    fi

    set -a
    # shellcheck disable=SC1090
    source "$env_file"
    set +a

    # Export the resolved path so callers can reference it
    LOADED_ENV_FILE="$env_file"
    export LOADED_ENV_FILE
}

# ---------------------------------------------------------------------------
# Helper: print_status EXIT_CODE LABEL
#
# Prints a colour-coded pass/fail line.
# ---------------------------------------------------------------------------
print_status() {
    if [[ $1 -eq 0 ]]; then
        echo -e "${GREEN}✓${NC} $2"
    else
        echo -e "${RED}✗${NC} $2"
    fi
}

# ---------------------------------------------------------------------------
# Helper: print_section TITLE
#
# Prints a section header.
# ---------------------------------------------------------------------------
print_section() {
    echo ""
    echo -e "${CYAN}========================================${NC}"
    echo -e "${CYAN}$1${NC}"
    echo -e "${CYAN}========================================${NC}"
}

# ---------------------------------------------------------------------------
# Helper: sanitize_tf_name NAME
#
# Makes a string safe for use as a Terraform resource name.
# Terraform names must start with a letter or underscore and may only contain
# letters, digits, underscores, and dashes.
#
# - Replaces invalid characters with underscores
# - Prefixes with "profile_" if the name starts with a digit
# - Prints a warning to stderr if the name was changed
#
# Usage:  RESOURCE_NAME=$(sanitize_tf_name "$RESOURCE_NAME")
# ---------------------------------------------------------------------------
sanitize_tf_name() {
    local name="$1"
    local original="$name"

    # Replace characters that aren't letters, digits, underscores, or dashes
    name=$(echo "$name" | sed 's/[^a-zA-Z0-9_-]/_/g')

    # Prefix if it starts with a digit
    if [[ "$name" =~ ^[0-9] ]]; then
        name="profile_${name}"
    fi

    if [[ "$name" != "$original" ]]; then
        echo -e "${YELLOW}Note: Resource name sanitized from '${original}' to '${name}'${NC}" >&2
    fi

    echo "$name"
}

# ---------------------------------------------------------------------------
# Helper: require_cmd COMMAND [INSTALL_HINT]
#
# Exits with an error if COMMAND is not found.
# ---------------------------------------------------------------------------
require_cmd() {
    local cmd="$1"
    local hint="${2:-}"
    if ! command -v "$cmd" >/dev/null 2>&1; then
        echo -e "${RED}Error: '$cmd' is not installed${NC}" >&2
        if [[ -n "$hint" ]]; then
            echo -e "${YELLOW}Install with: $hint${NC}" >&2
        fi
        return 1
    fi
}
