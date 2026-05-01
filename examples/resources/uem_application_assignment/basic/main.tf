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

variable "application_uuid" {
  description = "Workspace ONE UEM application UUID to assign"
  type        = string
}

variable "smart_group_uuids" {
  description = "Smart group UUIDs that receive the application assignment"
  type        = list(string)
}

variable "excluded_smart_groups" {
  description = "Smart group UUIDs to exclude from the assignment"
  type        = list(string)
  default     = []
}

resource "uem_application_assignment" "example" {
  application_uuid      = var.application_uuid
  excluded_smart_groups = var.excluded_smart_groups

  assignments = [
    {
      priority = 0
      distribution = {
        name                = "Terraform Example Auto Install"
        description         = "Deploy this application to managed devices"
        smart_groups        = var.smart_group_uuids
        app_delivery_method = "AUTO"
      }
      restriction = {
        remove_on_unenroll = true
      }
    }
  ]
}

output "assignment_id" {
  description = "Application assignment ID"
  value       = uem_application_assignment.example.id
}
