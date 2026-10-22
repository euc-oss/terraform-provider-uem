variable "organization_group_uuid" {
  type        = string
  description = "Organization group UUID to search for update deployments within"
}

data "uem_update_deployments" "example" {
  organization_group_uuid = var.organization_group_uuid
  platform                = "Apple"
}

output "example_deployment_uuids" {
  value = data.uem_update_deployments.example.update_deployments[*].uuid
}
