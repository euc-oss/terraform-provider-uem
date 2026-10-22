variable "organization_group_uuid" {
  type        = string
  description = "Organization group UUID that owns the sensor"
}

resource "uem_mac_sensor" "example" {
  organization_group_uuid = var.organization_group_uuid
  name                    = "terraform_example_sensor"
  description             = "Sample macOS sensor managed by Terraform"
  language                = "BASH"
  execution_context       = "SYSTEM"
  response_data_type      = "STRING"

  # code must be base64-encoded. Use filebase64() to load a script from disk.
  code = base64encode(<<-EOT
    #!/bin/bash
    echo "OK"
  EOT
  )
}

output "mac_sensor_id" {
  description = "Device sensor UUID assigned by UEM"
  value       = uem_mac_sensor.example.id
}
