resource "uem_update_deployment" "docs_example" {
  update_uuid             = var.update_uuid
  organization_group_uuid = var.organization_group_uuid
  name                    = var.name
  deployment_type         = var.deployment_type
  deployment_start_time   = var.deployment_start_time
  smart_group_uuids       = var.smart_group_uuids

  notifications = [
    {
      action  = "INSTALL_SUCCESS"
      message = "Update installed successfully"
    }
  ]
}

output "update_deployment_docs_example_id" {
  description = "Deployment UUID assigned by UEM"
  value       = uem_update_deployment.docs_example.id
}
