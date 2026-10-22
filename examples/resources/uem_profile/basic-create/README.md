# Basic Profile Creation Example

This example demonstrates creating a new Workspace ONE profile from scratch
using Terraform.

## What This Example Creates

- A basic Android profile with lock screen message
- Assigned to your organization group
- Optional assignment (devices can choose to install)
- Test scope (safe for testing)
- Initially inactive (won't deploy until activated)

## Prerequisites

1. **Workspace ONE UEM environment** with API access
2. **API credentials** configured in `.env` file in repository root
3. **Terraform** installed (>= 1.0)
4. **Provider installed** - see the project README's Installation section

## Configuration

The `.env` file in the repository root should contain:

```bash
# Workspace ONE Instance
UEM_INSTANCE_URL=https://your-instance.awmdm.com
UEM_TENANT_CODE=your-tenant-code

# Authentication (choose one method)
UEM_AUTH_METHOD=basic  # or oauth2

# Basic Auth
UEM_USERNAME=your-api-username
UEM_PASSWORD=your-api-password

# OAuth2 (if using UEM_AUTH_METHOD=oauth2)
UEM_CLIENT_ID=your-client-id
UEM_CLIENT_SECRET=your-client-secret
UEM_OAUTH2_TOKEN_URL=https://your-instance.awmdm.com/api/oauth/token

# Organization Group
UEM_ORG_GROUP_ID=12345  # Your organization group ID
```

## Usage

### Step 1: Review Configuration

Open `main.tf` and review the profile configuration:

```hcl
resource "uem_profile" "test_android" {
  name             = "Terraform Test Profile"
  description      = "Test profile created by Terraform provider"
  platform         = "Android"
  org_group_id     = "12345"  # Update with your ORG_GROUP_ID
  assignment_type  = "Optional"
  profile_scope    = "Staging"
  is_active        = false

  lock_screen_message = "Test device - Managed by Terraform"
}
```

**Update the `org_group_id`** to match your environment (from `.env` file).

### Step 2: Initialize Terraform

```bash
terraform init
```

This downloads the Workspace ONE provider and prepares the working directory.

### Step 3: Preview Changes

```bash
terraform plan
```

Review the planned changes. You should see:

- 1 resource to create (`uem_profile.test_android`)
- Profile details matching your configuration

### Step 4: Create the Profile

```bash
terraform apply
```

Type `yes` when prompted to confirm.

### Step 5: Verify in Workspace ONE

1. Log into your Workspace ONE UEM console
2. Navigate to **Resources > Profiles & Baselines > Profiles**
3. Search for "Terraform Test Profile"
4. Verify the profile was created with correct settings:
   - Platform: Android
   - Assignment: Optional
   - Scope: Staging
   - Status: Inactive

### Step 6: View Outputs

After creation, Terraform displays the profile details:

```bash
Outputs:

profile_id = "12345"
profile_uuid = "abc-123-def-456"
profile_details = {
  id = "12345"
  name = "Terraform Test Profile"
  uuid = "abc-123-def-456"
  platform = "Android"
  profile_context = "Test"
}
```

### Step 7: Make Changes

Edit `main.tf` to change the profile (e.g., activate it):

```hcl
resource "uem_profile" "test_android" {
  # ... other settings ...
  is_active = true  # Changed from false
}
```

Then apply the changes:

```bash
terraform plan   # Review changes
terraform apply  # Apply changes
```

### Step 8: Clean Up

When done testing, destroy the profile:

```bash
terraform destroy
```

Type `yes` when prompted to confirm.

## Customization

### Different Platforms

Change the `platform` attribute:

```hcl
platform = "Apple iOS"      # iOS devices
platform = "Windows 10"     # Windows 10 devices
platform = "AppleOsX"       # macOS devices
platform = "Linux"          # Linux devices
```

### Assignment Types

```hcl
assignment_type = "Auto"      # Automatically assigned to all devices
assignment_type = "Optional"  # Users can choose to install
```

### Profile Scopes

```hcl
profile_scope = "Staging"     # Staging environment
profile_scope = "Production"  # Production environment
profile_scope = "Both"        # Both staging and production
```

## Troubleshooting

### Error: "Provider not found"

See the project README's Installation section to install the provider.

### Error: "Authentication failed"

Check your `.env` file credentials and ensure they're correct.

### Error: "Organization group not found"

Verify the `org_group_id` matches your environment.

## Next Steps

- **Import existing profiles**: See `../import-existing/` example
- **Create multiple profiles**: Add more `resource` blocks
- **Use variables**: Extract common values to `variables.tf`
- **Add outputs**: Export profile information for other modules

## Related Documentation

- [Provider Configuration](../../../provider/provider.tf)
- [Import Existing Profiles](../import-existing/README.md)
- [Workspace ONE Profile Resource](../../../../docs/resources/profile.md)
