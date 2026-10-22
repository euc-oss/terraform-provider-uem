# Example Workspace ONE UEM Profile Resource

# Basic Android Profile with Lock Screen Message
resource "uem_profile" "android_basic" {
  name            = "Android Basic Profile"
  description     = "Basic Android profile created by Terraform"
  platform        = "Android"
  org_group_id    = "12345" # Replace with your Organization Group ID
  assignment_type = "Auto"
  profile_scope   = "Production"
  is_active       = true

  lock_screen_message = "Welcome to Workspace ONE UEM"
}

# iOS Profile
resource "uem_profile" "ios_basic" {
  name            = "iOS Basic Profile"
  description     = "Basic iOS profile created by Terraform"
  platform        = "Apple iOS"
  org_group_id    = "12345" # Replace with your Organization Group ID
  assignment_type = "Auto"
  profile_scope   = "Production"
  is_active       = true
}

# Windows 10 Profile
resource "uem_profile" "windows_basic" {
  name            = "Windows 10 Basic Profile"
  description     = "Basic Windows 10 profile created by Terraform"
  platform        = "Windows 10"
  org_group_id    = "12345" # Replace with your Organization Group ID
  assignment_type = "Optional"
  profile_scope   = "Staging"
  is_active       = false
}

# macOS Profile
resource "uem_profile" "macos_basic" {
  name            = "macOS Basic Profile"
  description     = "Basic macOS profile created by Terraform"
  platform        = "AppleOsX"
  org_group_id    = "12345" # Replace with your Organization Group ID
  assignment_type = "Auto"
  profile_scope   = "Production"
  is_active       = true
}

# Output the created profile IDs
output "android_profile_id" {
  description = "The ID of the Android profile"
  value       = uem_profile.android_basic.id
}

output "android_profile_uuid" {
  description = "The UUID of the Android profile"
  value       = uem_profile.android_basic.uuid
}

output "ios_profile_id" {
  description = "The ID of the iOS profile"
  value       = uem_profile.ios_basic.id
}

