# Create a macOS (AppleOsX) Custom Settings Profile
# This example creates a profile with custom settings payloads for macOS
# devices via the POST /api/mdm/profiles/platforms/appleosx/create endpoint.
# Custom settings allow pushing arbitrary configuration profiles (XML/plist)
# to devices when the built-in payload types don't cover a specific need.

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

# macOS Custom Settings Profile
resource "uem_profile" "macos_custom_settings" {
  name            = "macOS Custom Settings"
  description     = "Deploys custom configuration profiles to macOS devices"
  platform        = "AppleOsX"
  org_group_id    = "12345" # Update with your OG ID from .env
  assignment_type = "Auto"
  profile_scope   = "Production"
  is_active       = true

  custom_settings_list = [
    {
      custom_settings = <<-XML
        <dict>
          <key>PayloadType</key>
          <string>com.apple.screensaver</string>
          <key>PayloadDisplayName</key>
          <string>Screen Saver Settings</string>
          <key>idleTime</key>
          <integer>600</integer>
          <key>askForPassword</key>
          <true/>
          <key>askForPasswordDelay</key>
          <integer>5</integer>
        </dict>
      XML
    },
    {
      custom_settings = <<-XML
        <dict>
          <key>PayloadType</key>
          <string>com.apple.loginwindow</string>
          <key>PayloadDisplayName</key>
          <string>Login Window Settings</string>
          <key>SHOWFULLNAME</key>
          <true/>
          <key>AdminHostInfo</key>
          <string>HostName</string>
        </dict>
      XML
    },
  ]
}

# Outputs
output "profile_id" {
  description = "Created macOS custom settings profile ID"
  value       = uem_profile.macos_custom_settings.id
}

output "profile_uuid" {
  description = "Created macOS custom settings profile UUID"
  value       = uem_profile.macos_custom_settings.uuid
}

output "profile_details" {
  description = "Full profile details"
  value = {
    id              = uem_profile.macos_custom_settings.id
    name            = uem_profile.macos_custom_settings.name
    uuid            = uem_profile.macos_custom_settings.uuid
    platform        = uem_profile.macos_custom_settings.platform
    profile_context = uem_profile.macos_custom_settings.profile_context
  }
}
