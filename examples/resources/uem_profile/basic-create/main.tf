# Quick test configuration for Workspace ONE UEM Terraform Provider
# This file is for local testing only

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

# Provider uses environment variables from .env file
provider "uem" {
  # Configuration loaded from:
  # - UEM_INSTANCE_URL
  # - UEM_TENANT_CODE
  # - UEM_AUTH_METHOD
  # - UEM_USERNAME / UEM_PASSWORD (for basic auth)
  # - UEM_CLIENT_ID / UEM_CLIENT_SECRET / UEM_OAUTH2_TOKEN_URL (for oauth2)
}

# Test Android Profile
resource "uem_profile" "test_android" {
  name            = "Terraform Test Profile"
  description     = "Test profile created by Terraform provider"
  platform        = "Android"
  org_group_id    = "12345" # Update with your ORG_GROUP_ID from .env
  assignment_type = "Optional"
  profile_scope   = "Staging"
  is_active       = false

  lock_screen_message = "Test device - Managed by Terraform"
}

# Outputs
output "profile_id" {
  description = "Created profile ID"
  value       = uem_profile.test_android.id
}

output "profile_uuid" {
  description = "Created profile UUID"
  value       = uem_profile.test_android.uuid
}

output "profile_details" {
  description = "Full profile details"
  value = {
    id              = uem_profile.test_android.id
    name            = uem_profile.test_android.name
    uuid            = uem_profile.test_android.uuid
    platform        = uem_profile.test_android.platform
    profile_context = uem_profile.test_android.profile_context
  }
}

