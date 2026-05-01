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
  description = "Workspace ONE UEM organization group ID for imported profiles"
  type        = string
}

# Use scripts/import-profiles.sh to import profiles into this directory.
# The script creates temporary import-temp-*.tf files when needed.
resource "uem_profile" "imported_example" {
  name         = "Imported Profile Placeholder"
  platform     = "AppleOsX"
  org_group_id = var.org_group_id
}

output "imported_profile_id" {
  description = "Imported profile ID"
  value       = uem_profile.imported_example.id
}

output "imported_profile_uuid" {
  description = "Imported profile UUID"
  value       = uem_profile.imported_example.uuid
}
