variable "org_group_id" {
  type        = number
  description = "Organization Group ID for the target OG"
}

variable "dmg_file_path" {
  type        = string
  description = "Path to the DMG file on disk"
}

variable "plist_file_path" {
  type        = string
  description = "Path to the plist (pkginfo) metadata file"
}

resource "uem_mac_application" "example" {
  org_group_id    = var.org_group_id
  dmg_file_path   = var.dmg_file_path
  plist_file_path = var.plist_file_path
  app_version     = "1.0.0"
}

output "mac_application_uuid" {
  description = "UUID assigned by UEM after creation"
  value       = uem_mac_application.example.uuid
}
