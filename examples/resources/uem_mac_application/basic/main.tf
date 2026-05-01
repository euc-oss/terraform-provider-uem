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
  type        = number
}

variable "dmg_file_path" {
  description = "Path to the macOS application DMG file"
  type        = string
  default     = "./apps/example-app.dmg"
}

variable "plist_file_path" {
  description = "Path to the pkginfo plist for the macOS application"
  type        = string
  default     = "./apps/example-app.plist"
}

variable "app_version" {
  description = "Application version"
  type        = string
  default     = "1.0.0"
}

resource "uem_mac_application" "example" {
  org_group_id    = var.org_group_id
  dmg_file_path   = var.dmg_file_path
  plist_file_path = var.plist_file_path
  app_version     = var.app_version
}

output "application_id" {
  description = "Uploaded application ID"
  value       = uem_mac_application.example.id
}

output "application_uuid" {
  description = "Uploaded application UUID"
  value       = uem_mac_application.example.uuid
}
