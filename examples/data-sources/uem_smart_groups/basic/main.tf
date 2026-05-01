terraform {
  required_providers {
    uem = {
      source = "local/omnissa/uem"
    }
  }
}

provider "uem" {
  # Configuration is loaded from UEM_* environment variables.
}

variable "org_group_id" {
  description = "Workspace ONE UEM organization group ID"
  type        = string
}

variable "name_filter" {
  description = "Optional smart group name filter"
  type        = string
  default     = ""
}

data "uem_smart_groups" "example" {
  organization_group_id = var.org_group_id
  name                  = var.name_filter != "" ? var.name_filter : null
}

output "smart_group_ids" {
  description = "Matching smart group IDs"
  value       = [for group in data.uem_smart_groups.example.smart_groups : group.smart_group_id]
}

output "smart_group_uuids" {
  description = "Matching smart group UUIDs"
  value       = [for group in data.uem_smart_groups.example.smart_groups : group.smart_group_uuid]
}

output "smart_group_names" {
  description = "Matching smart group names"
  value       = [for group in data.uem_smart_groups.example.smart_groups : group.name]
}
