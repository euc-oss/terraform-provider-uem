# Example Workspace ONE UEM Provider Configuration

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

provider "uem" {
  # All config can be provided via env vars:
  # UEM_INSTANCE_URL, UEM_TENANT_CODE, UEM_AUTH_METHOD,
  # UEM_USERNAME / UEM_PASSWORD (basic), UEM_CLIENT_ID / UEM_CLIENT_SECRET / UEM_OAUTH2_TOKEN_URL (oauth2)
}

# Example using Basic Auth explicitly
# provider "uem" {
#   instance_url = "https://cn1234.awmdm.com"
#   tenant_code  = "YourTenantCode"
#   auth_method  = "basic"
#   username     = "admin@example.com"
#   password     = "your-password"
# }

# Example using OAuth2 explicitly
# provider "uem" {
#   instance_url    = "https://cn1234.awmdm.com"
#   tenant_code     = "YourTenantCode"
#   auth_method     = "oauth2"
#   client_id       = "your-client-id"
#   client_secret   = "your-client-secret"
#   oauth2_token_url = "https://na.uemauth.workspaceone.com/connect/token"
# }
