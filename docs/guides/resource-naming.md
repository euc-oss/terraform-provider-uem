# Resource Naming Guide

Terraform resource identifiers follow a rule defined by the HCL language
itself, not by this provider:

- A resource name must start with a letter (`a-z`, `A-Z`) or an underscore
  (`_`).
- After that first character, only letters, digits, underscores, and dashes
  are allowed.

This means a name like `123_test` or a name containing spaces or special
characters (`My Profile!`) is not a valid Terraform resource identifier,
even though it may be a perfectly valid name in Workspace ONE UEM.

## When you write your own configuration

A Workspace ONE UEM object's name and its Terraform resource identifier
don't have to match. If the object's name isn't a valid Terraform
identifier, just pick a valid name for the resource block yourself — for
example:

```hcl
resource "uem_profile" "corporate_wifi" {
  name            = "Corporate Wi-Fi" # the UEM-side name can be anything
  platform        = "AppleOsX"
  org_group_id    = "12345"
  assignment_type = "Optional"
  profile_scope   = "Staging"
  is_active       = false
}
```

## When `ws1-tf onboard` generates configuration

The companion `ws1-tf` CLI's `onboard` command scaffolds `.tf` files from
what already exists in your tenant, and generates a resource name for each
object it imports. It sanitizes the source object's name: any character
that isn't a letter, digit, underscore, or dash becomes `_`, and if the
result still starts with a digit, `ws1-tf` prefixes it with a name that
reflects the resource type being generated (for example, a profile becomes
`profile_<name>`, a macOS application becomes `mac_application_<name>`), so
generated names stay distinguishable across resource types.

## Naming is yours to control

This provider itself performs no name sanitization — Terraform resource
addresses in your `.tf` files are always your own responsibility, whether
you write them by hand or review what `ws1-tf onboard` generated.
