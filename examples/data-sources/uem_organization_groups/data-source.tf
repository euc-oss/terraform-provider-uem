variable "organization_group_name_filter" {
  type        = string
  description = "Optional substring filter on organization group name; leave null to list every organization group visible to the caller"
  default     = null
}

data "uem_organization_groups" "example" {
  name = var.organization_group_name_filter
}

output "example_organization_group_uuids" {
  value = data.uem_organization_groups.example.organization_groups[*].uuid
}
