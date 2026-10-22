variable "application_uuid" {
  type        = string
  description = "UUID of the purchased (VPP) application to manage assignment rules for"
}

variable "smart_group_uuid" {
  type        = string
  description = "Smart group UUID to license the app to"
}

resource "uem_purchased_application_assignment" "example" {
  application_uuid = var.application_uuid

  assignments = [
    {
      priority = 0
      distribution = {
        name                = "All users"
        description         = "VPP assignment"
        app_delivery_method = "AUTO"
        vpp_app_details = {
          license_usage = [
            {
              smart_group_uuid = var.smart_group_uuid
              allocated        = 1
            },
          ]
        }
      }
      restriction = {
        remove_on_unenroll = true
      }
    },
  ]
}

output "purchased_application_assignment_id" {
  description = "Assignment resource ID (mirrors application_uuid)"
  value       = uem_purchased_application_assignment.example.id
}
