variable "organization_group_id" {
  type        = string
  description = "Organization group ID to filter profiles by"
}

data "uem_profiles" "example" {
  name                  = "Example Profile"
  organization_group_id = var.organization_group_id
}

output "example_profile_ids" {
  value = data.uem_profiles.example.profiles[*].profile_id
}
