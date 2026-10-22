#!/bin/bash
# Import an existing Workspace ONE macOS device sensor into Terraform

# Import by device sensor UUID
# Usage: terraform import uem_mac_sensor.example "<uuid>"

terraform import uem_mac_sensor.example "00000000-0000-0000-0000-000000000001"
