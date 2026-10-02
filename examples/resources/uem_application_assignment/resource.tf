variable "application_uuid" {
  type        = string
  description = "UUID of the application to manage assignment rules for"
}

variable "smart_group_uuid" {
  type        = string
  description = "Smart group UUID to assign the application to"
}

resource "uem_application_assignment" "example" {
  application_uuid = var.application_uuid

  excluded_smart_groups = []

  assignments = [
    {
      priority = 0

      distribution = {
        name                = "All users"
        description         = "Terraform-managed assignment"
        smart_groups        = [var.smart_group_uuid]
        app_delivery_method = "AUTO"
      }

      restriction = {
        remove_on_unenroll = true
      }
    },
  ]
}

output "application_assignment_id" {
  description = "Assignment resource ID (mirrors application_uuid)"
  value       = uem_application_assignment.example.id
}
