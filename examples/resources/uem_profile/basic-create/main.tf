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

variable "assigned_smart_groups" {
  description = "Smart group IDs to assign to the profile"
  type        = list(string)
  default     = []
}

resource "uem_profile" "macos_gatekeeper" {
  name                  = "Terraform Example - macOS Gatekeeper"
  description           = "Example macOS profile managed by Terraform"
  platform              = "AppleOsX"
  org_group_id          = var.org_group_id
  assignment_type       = "Auto"
  profile_scope         = "Test"
  profile_context       = "Device"
  is_active             = true
  assigned_smart_groups = var.assigned_smart_groups

  gatekeeper = {
    allow_auto_unlock                  = true
    allow_fingerprint_for_unlock       = true
    allow_handoff                      = true
    allow_screen_capture               = true
    enable_software_update_delay       = true
    enforced_software_update_delay     = 30
    enable_app_software_update_delay   = true
    enforced_app_software_update_delay = 30
  }
}

output "profile_id" {
  description = "Created profile ID"
  value       = uem_profile.macos_gatekeeper.id
}

output "profile_uuid" {
  description = "Created profile UUID"
  value       = uem_profile.macos_gatekeeper.uuid
}
