# Import Existing Profiles Example

This example demonstrates importing existing Workspace ONE profiles into
Terraform for management.

## What This Example Does

- Looks up existing profiles in your Workspace ONE environment
- Imports profiles into Terraform state
- Allows you to manage existing profiles with Terraform going forward

## Why Import?

If you have existing profiles in Workspace ONE that were created manually or
through other tools, you can import them into Terraform to:

1. **Manage with Infrastructure as Code** - Track changes in version control
2. **Prevent Drift** - Ensure profiles match your desired configuration
3. **Automate Updates** - Use Terraform to update profiles consistently
4. **Document Configuration** - Profile settings are now in code

## Prerequisites

1. **Workspace ONE UEM environment** with existing profiles
2. **API credentials** configured in `.env` file in repository root
3. **Terraform** installed (>= 1.0)
4. **Provider installed** - see the project README's Installation section

## Step-by-Step Import Process

### Step 1: List Available Profiles

Use the [`uem_profiles`](../../../../docs/data-sources/profiles.md) data
source, or `ws1-tf list`, to see profiles in your environment and their IDs
and platforms.

### Step 2: Map API Platform to Terraform Platform

The API returns different platform names than Terraform expects:

| API Platform | Terraform Platform |
| ------------ | ------------------ |
| Android      | Android            |
| Apple        | Apple iOS          |
| WinRT        | Windows 10         |
| AppleOsX     | AppleOsX           |
| Linux        | Linux              |

### Step 3: Create Resource Block

Create or edit `main.tf` with a resource block for the profile:

```hcl
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

provider "uem" {
  # Configuration loaded from .env file
}

# Import existing Android profile
resource "uem_profile" "custom_android" {
  name                = "Custom Message Android"
  platform            = "Android"  # Use Terraform platform name
  org_group_id        = "12345"
  assignment_type     = "Auto"
  profile_scope       = "Production"
  is_active           = true
  lock_screen_message = "Hey There!!"
}
```

**Important:** The resource block should match the profile's current
configuration as closely as possible.

### Step 4: Initialize Terraform

```bash
terraform init
```

### Step 5: Import the Profile

```bash
terraform import uem_profile.custom_android "12345:Android"
```

**Import format:** `<profile_id>:<terraform_platform>`

Or let `ws1-tf onboard` generate the resource block and run the import for
you.

### Step 6: Verify Import

Check that Terraform recognizes the profile:

```bash
terraform plan
```

**Expected output:**

```text
No changes. Your infrastructure matches the configuration.
```

If you see changes, update your resource block to match the actual profile
configuration.

### Step 7: Adjust Configuration

If `terraform plan` shows differences, update `main.tf` to match:

```bash
terraform show
```

This displays the imported profile's actual values. Update your resource block
accordingly.

### Step 8: Apply Any Changes

Once the configuration matches, you can make changes:

```hcl
resource "uem_profile" "custom_android" {
  # ... existing config ...
  lock_screen_message = "Updated message!"  # Change something
}
```

Then apply:

```bash
terraform apply
```

## Importing Multiple Profiles

Use the `imported-profiles.tf.example` file as a template:

### Step 1: Copy the Example

```bash
cp imported-profiles.tf.example imported-profiles.tf
```

### Step 2: Update Resource Blocks

Edit `imported-profiles.tf` and update each resource block with actual profile
details from your environment.

### Step 3: Import Each Profile

```bash
# Android profiles
terraform import uem_profile.android_corporate "12345:Android"
terraform import uem_profile.android_byod "12348:Android"

# iOS profiles
terraform import uem_profile.ios_corporate "12349:Apple iOS"
terraform import uem_profile.ios_byod "12350:Apple iOS"

# Windows profiles
terraform import uem_profile.windows_corporate "12351:Windows 10"

# macOS profiles
terraform import uem_profile.macos_corporate "12352:AppleOsX"
```

Or let `ws1-tf onboard` do all of this for you in one pass.

### Step 4: Verify All Imports

```bash
terraform plan
```

Should show "No changes" if all resource blocks match the actual profiles.

## Example: Complete Import Workflow

```bash
# 1. Look up profiles (uem_profiles data source, or ws1-tf list)

# 2. Create resource block in main.tf
cat > main.tf << 'EOF'
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

provider "uem" {}

resource "uem_profile" "prod_android" {
  name             = "Production Android"
  platform         = "Android"
  org_group_id     = "12345"
  assignment_type  = "Auto"
  profile_scope    = "Production"
  is_active        = true
}
EOF

# 3. Initialize
terraform init

# 4. Import
terraform import uem_profile.prod_android "12345:Android"

# 5. Verify
terraform plan

# 6. Make changes if needed
# Edit main.tf...

# 7. Apply changes
terraform apply
```

## Troubleshooting

### Error: "Resource already exists in state"

The profile was already imported. Use `terraform state rm` to remove it first:

```bash
terraform state rm uem_profile.custom_android
```

Then re-import.

### Error: "Invalid import ID format"

Ensure you're using the format `<profile_id>:<platform>`:

```bash
terraform import uem_profile.custom_android "12345:Android"
```

### Error: "Profile not found"

Verify the profile ID is correct using the `uem_profiles` data source, or
`ws1-tf list`.

### Plan shows many changes after import

Your resource block doesn't match the actual profile. Run `terraform show` to
see the imported values and update your resource block.

## Next Steps

- **Manage imported profiles**: Make changes and apply with Terraform
- **Create new profiles**: See `../basic-create/` example
- **Organize profiles**: Use modules for better organization
- **Version control**: Commit your `.tf` files to git

## Related Documentation

- [Provider Configuration](../../../provider/provider.tf)
- [Create New Profiles](../basic-create/README.md)
- [Workspace ONE Profile Resource](../../../../docs/resources/profile.md)
- [Profiles Data Source](../../../../docs/data-sources/profiles.md)
- [Importing Profiles Guide](../../../../docs/guides/importing-profiles.md)
