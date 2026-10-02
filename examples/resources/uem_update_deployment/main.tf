# This provider is not yet published to the Terraform Registry; install it
# from the pre-release mirror zip (see the project README) before running
# `terraform init`. For local development, see "Developing the provider
# locally" in the README for the dev_overrides workflow.
terraform {
  required_version = ">= 1.0"

  required_providers {
    uem = {
      source = "registry.terraform.io/omnissa/uem"
    }
  }
}

# Configure via environment variables:
#   UEM_INSTANCE_URL, UEM_TENANT_CODE, UEM_AUTH_METHOD,
#   UEM_USERNAME, UEM_PASSWORD (basic) or UEM_CLIENT_ID / UEM_CLIENT_SECRET (oauth2)
provider "uem" {}

resource "uem_update_deployment" "example" {
  update_uuid             = var.update_uuid
  organization_group_uuid = var.organization_group_uuid
  name                    = var.name
  deployment_type         = var.deployment_type
  deployment_start_time   = var.deployment_start_time
  smart_group_uuids       = var.smart_group_uuids

  notifications = [
    {
      action  = "INSTALL_SUCCESS"
      message = "OS update installed v2"
    }
  ]
}

output "deployment_id" {
  value = uem_update_deployment.example.id
}
