variable "sensor_uuid" {
  type        = string
  description = "Device sensor UUID (uem_mac_sensor id) to manage assignments for"
}

variable "smart_group_uuids" {
  type        = list(string)
  description = "Smart group UUIDs to assign the sensor to"
}

resource "uem_sensor_assignment" "example" {
  sensor_uuid = var.sensor_uuid

  assignments = [
    {
      name              = "All Devices"
      ranking           = 1
      smart_group_uuids = var.smart_group_uuids
      trigger_type      = "SCHEDULEANDEVENT"
      event_triggers    = ["LOGIN"]
    },
  ]
}

output "sensor_assignment_id" {
  description = "Sensor UUID used as the assignment resource identifier"
  value       = uem_sensor_assignment.example.id
}
