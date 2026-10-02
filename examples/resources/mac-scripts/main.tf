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

variable "organization_group_uuid" {
  description = "Organization group UUID that owns the script"
  type        = string
}

variable "smart_group_uuid" {
  description = "Smart group UUID to assign the script to"
  type        = string
}

resource "uem_mac_script" "hello" {
  organization_group_uuid = var.organization_group_uuid
  name                    = "Terraform Hello Script v11"
  description             = "Sample bash script created by the UEM Terraform provider v2"
  platform                = "APPLE_OSX"
  script_type             = "BASH"
  execution_context       = "SYSTEM"
  platform_architecture   = "UNKNOWN"

  # script_data must be base64-encoded. Use filebase64() for scripts on disk.
  script_data = filebase64("${path.module}/scripts/hello.sh")

  timeout = 30

  script_variables = [
    {
      name  = "GREETING"
      value = "Hello, device"
    },
  ]

  allowed_in_catalog = true
  user_interaction   = false

  catalog_display = {
    display_name     = "Hello Script"
    display_desc     = "Runs a simple greeting script on the Mac"
    pre_action_text  = "This script will run on your Mac. Continue?"
    post_action_text = "The script has finished running."
    action_type      = "INSTALL"
    catalog_icon_url = "https://example.com/icons/script.png"
    categories       = ["1"]
  }
}

resource "uem_script_assignment" "hello" {
  script_uuid = uem_mac_script.hello.id

  assignments = [
    {
      name            = "Hello Script Assignment2"
      priority        = 1
      deployment_mode = "AUTO"
      show_in_catalog = true

      memberships = [
        {
          smart_group_uuid = var.smart_group_uuid
        },
      ]

      script_deployment = {
        trigger_type     = "SCHEDULE_AND_EVENT"
        trigger_events   = ["RUN_IMMEDIATELY", "LOGIN", "LOGOUT", "STARTUP", "NETWORK_CHANGE"]
        trigger_schedule = "FOUR_HOURS"
      }
    },
  ]
}

output "script_id" {
  description = "Script UUID assigned by UEM"
  value       = uem_mac_script.hello.id
}

output "script_name" {
  description = "Script display name in UEM"
  value       = uem_mac_script.hello.name
}

output "script_assignment_id" {
  description = "Script UUID used as the assignment resource identifier"
  value       = uem_script_assignment.hello.id
}
