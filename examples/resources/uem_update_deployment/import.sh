#!/usr/bin/env bash
set -euo pipefail

# Import an existing device update deployment by deployment UUID.
# Usage: DEPLOYMENT_UUID=<uuid> ./import.sh
#
# Discover UUIDs with the uem_update_deployments data source, or ws1-tf list.

terraform import uem_update_deployment.example "${DEPLOYMENT_UUID:?set DEPLOYMENT_UUID}"
