#!/usr/bin/env bash
set -euo pipefail

# Import an existing purchased-app assignment rule by application UUID.
# Usage: APPLICATION_UUID=<uuid> ./import.sh

terraform import uem_purchased_application_assignment.bible "${APPLICATION_UUID:?set APPLICATION_UUID}"
