variable "organization_group_uuid" {
  type        = string
  description = "Organization group UUID to search for sensors within"
}

data "uem_sensors" "example" {
  organization_group_uuid = var.organization_group_uuid
  name                    = "example-sensor"
}

output "example_sensor_uuids" {
  value = data.uem_sensors.example.sensors[*].uuid
}
