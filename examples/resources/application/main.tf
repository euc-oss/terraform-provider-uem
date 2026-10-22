# This provider is not yet published to the Terraform Registry; install it
# from the pre-release mirror zip (see the project README) before running
# `terraform init`. For local development, see "Developing the provider
# locally" in the README for the dev_overrides workflow.
terraform {
  required_providers {
    uem = {
      source = "registry.terraform.io/omnissa/uem"
    }
  }
}

# macOS app uploads require Basic Auth (blob uploads do not support OAuth2).
# Configure via environment variables:
#   UEM_INSTANCE_URL, UEM_TENANT_CODE, UEM_AUTH_METHOD=basic,
#   UEM_USERNAME, UEM_PASSWORD
# Or set them explicitly below.
provider "uem" {}
variable "org_group_id" {
  description = "Organization Group ID for the target OG"
  type        = number
}

variable "dmg_file_path" {
  description = "Path to the DMG file on disk"
  type        = string
}

variable "plist_file_path" {
  description = "Path to the plist (pkginfo) metadata file"
  type        = string
}

resource "uem_mac_application" "cursor" {
  org_group_id    = var.org_group_id
  dmg_file_path   = var.dmg_file_path
  plist_file_path = var.plist_file_path
  app_version     = "2.6.22"
}

resource "uem_application_assignment" "a1" {
  application_uuid = uem_mac_application.cursor.uuid
  # "00000000-0000-0000-0000-000000000006"
  #"00000000-0000-0000-0000-000000000007" ##uem_macos_app.cursor.app_uuid #"00000000-0000-0000-0000-000000000008"

  excluded_smart_groups = []

  assignments = [
    {
      priority = 0

      distribution = {
        name                = "a2"
        description         = ""
        smart_groups        = ["00000000-0000-0000-0000-000000000009"]
        app_delivery_method = "AUTO",
      }

      restriction = {
        remove_on_unenroll = true
      }
    }
  ]
}
