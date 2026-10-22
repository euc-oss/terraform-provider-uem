#!/bin/bash
# Import an existing Workspace ONE macOS internal application into Terraform

# Import by bare application UUID (org_group_id is left unset in state)
# Usage: terraform import uem_mac_application.example "<uuid>"

terraform import uem_mac_application.example "11111111-1111-1111-1111-111111111111"

# Import by UUID plus org_group_id (composite form) -- use this when you want
# org_group_id tracked exactly, since the API does not expose it for import
# Usage: terraform import uem_mac_application.example "<uuid>,<org_group_id>"

terraform import uem_mac_application.example "11111111-1111-1111-1111-111111111111,571"
