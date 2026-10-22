variable "organization_group_uuid" {
  type        = string
  description = "Organization group UUID to search for macOS applications within"
}

data "uem_mac_applications" "example" {
  organization_group_uuid = var.organization_group_uuid
}

output "example_mac_application_uuids" {
  value = data.uem_mac_applications.example.mac_applications[*].uuid
}
