# Import Existing Profile Example
# This example shows how to import an existing Workspace ONE UEM profile into Terraform

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

# Provider configuration - uses environment variables from .env
provider "uem" {
  # Configuration is loaded from environment variables:
  # UEM_INSTANCE_URL, UEM_TENANT_CODE, UEM_AUTH_METHOD, UEM_USERNAME, UEM_PASSWORD
  # or UEM_CLIENT_ID, UEM_CLIENT_SECRET, UEM_OAUTH2_TOKEN_URL for OAuth2
}

# Import profiles with: terraform import uem_profile.<resource_name> <profile_id>:<platform>
# Or let `ws1-tf onboard` generate the resource blocks and run the imports for you.

