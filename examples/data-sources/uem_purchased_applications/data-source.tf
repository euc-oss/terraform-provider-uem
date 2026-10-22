variable "organization_group_uuid" {
  type        = string
  description = "Organization group UUID to search for purchased applications within"
}

data "uem_purchased_applications" "example" {
  organization_group_uuid = var.organization_group_uuid
}

output "example_purchased_application_uuids" {
  value = data.uem_purchased_applications.example.purchased_applications[*].uuid
}
