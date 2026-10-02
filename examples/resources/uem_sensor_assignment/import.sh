#!/bin/bash
# Import an existing Workspace ONE device sensor assignment rule into Terraform

# Import by device sensor UUID
# Usage: terraform import uem_sensor_assignment.example "<sensor_uuid>"

terraform import uem_sensor_assignment.example "00000000-0000-0000-0000-000000000001"
