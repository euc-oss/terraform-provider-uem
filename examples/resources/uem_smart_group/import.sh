#!/bin/bash
# Import an existing Workspace ONE smart group into Terraform

# Import by numeric smart group id or by smart group UUID
# Usage: terraform import uem_smart_group.example "<id>"
# Usage: terraform import uem_smart_group.example "<uuid>"

terraform import uem_smart_group.example "12345"
terraform import uem_smart_group.example "00000000-0000-0000-0000-000000000001"
