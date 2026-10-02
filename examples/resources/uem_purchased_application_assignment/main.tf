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

resource "uem_purchased_application_assignment" "bible" {
  application_uuid = "00000000-0000-0000-0000-000000000005"

  assignments = [{
    priority = 0
    distribution = {
      name                = "All users"
      description         = "VPP assignment"
      app_delivery_method = "AUTO"
      vpp_app_details = {
        license_usage = [
          {
            smart_group_uuid = "00000000-0000-0000-0000-000000000002"
            allocated        = 2
          },
        ]
      }
    }
    restriction = {
      remove_on_unenroll = true
      prevent_removal    = true
      managed_access     = true
    }
  }]
}
