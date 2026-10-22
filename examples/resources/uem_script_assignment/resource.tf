variable "script_uuid" {
  type        = string
  description = "Script UUID (uem_mac_script id) to manage assignments for"
}

variable "smart_group_uuid" {
  type        = string
  description = "Smart group UUID to assign the script to"
}

resource "uem_script_assignment" "example" {
  script_uuid = var.script_uuid

  assignments = [
    {
      name            = "Example Script Assignment"
      priority        = 1
      deployment_mode = "AUTO"
      show_in_catalog = true

      memberships = [
        {
          smart_group_uuid = var.smart_group_uuid
        },
      ]

      script_deployment = {
        trigger_type     = "SCHEDULE_AND_EVENT"
        trigger_events   = ["RUN_IMMEDIATELY", "LOGIN"]
        trigger_schedule = "FOUR_HOURS"
      }
    },
  ]
}

output "script_assignment_id" {
  description = "Script UUID used as the assignment resource identifier"
  value       = uem_script_assignment.example.id
}
