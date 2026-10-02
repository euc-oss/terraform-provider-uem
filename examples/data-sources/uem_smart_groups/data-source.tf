variable "organization_group_id" {
  type        = string
  description = "Organization group ID to filter smart groups by"
}

data "uem_smart_groups" "example" {
  organization_group_id = var.organization_group_id
  name                  = "Example Group"
}

output "example_smart_group_ids" {
  value = data.uem_smart_groups.example.smart_groups[*].smart_group_id
}
