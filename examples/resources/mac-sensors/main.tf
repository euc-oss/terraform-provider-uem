# This provider is not yet published to the Terraform Registry; install it
# from the pre-release mirror zip (see the project README) before running
# `terraform init`. For local development, see "Developing the provider
# locally" in the README for the dev_overrides workflow.
terraform {
  required_version = ">= 1.0"

  required_providers {
    uem = {
      source = "registry.terraform.io/omnissa/uem"
    }
  }
}

# Configure via environment variables:
#   UEM_INSTANCE_URL, UEM_TENANT_CODE, UEM_AUTH_METHOD,
#   UEM_USERNAME, UEM_PASSWORD (basic) or UEM_CLIENT_ID / UEM_CLIENT_SECRET (oauth2)
provider "uem" {}

variable "organization_group_uuid" {
  description = "Organization group UUID that owns the sensor"
  type        = string
}

variable "smart_group_uuids" {
  description = "Smart group UUIDs to assign the sensor to"
  type        = list(string)
}

# Sensor definition. Smart group assignments and triggers are managed separately by
# uem_sensor_assignment, mirroring uem_mac_script + uem_script_assignment.
resource "uem_mac_sensor" "disk_free" {
  organization_group_uuid = var.organization_group_uuid
  name                    = "terraform_disk_free_sensor"
  description             = "Sample macOS disk free sensor created by Terraform"
  language                = "BASH"
  execution_context       = "SYSTEM"
  response_data_type      = "STRING"

  # code must be base64-encoded. Use filebase64() for scripts on disk.
  code = filebase64("${path.module}/sensors/disk_free.sh")
}

resource "uem_sensor_assignment" "disk_free" {
  sensor_uuid = uem_mac_sensor.disk_free.id

  assignments = [
    {
      name              = "All Devices"
      ranking           = 1
      smart_group_uuids = var.smart_group_uuids
      trigger_type      = "SCHEDULEANDEVENT"
      event_triggers    = ["LOGIN"]
    },
  ]
}

output "sensor_id" {
  description = "Device sensor UUID assigned by UEM"
  value       = uem_mac_sensor.disk_free.id
}

output "sensor_name" {
  description = "Device sensor display name in UEM"
  value       = uem_mac_sensor.disk_free.name
}

output "sensor_assignment_id" {
  description = "Sensor UUID used as the assignment resource identifier"
  value       = uem_sensor_assignment.disk_free.id
}

output "assigned_smart_group_uuids" {
  description = "Smart group UUIDs assigned to the sensor"
  value = flatten([
    for assignment in uem_sensor_assignment.disk_free.assignments : assignment.smart_group_uuids
  ])
}
