# Importing Profiles Guide

## What a Terraform provider actually exposes

A Terraform provider's real surface is its **resources** (with Create,
Read, Update, Delete, and `ImportState` operations) and its **data
sources**. There are no CLI subcommands in the Terraform plugin
protocol — every interaction happens through the `terraform` CLI acting
on resource blocks in HCL.

This guide covers the sanctioned mechanism — `terraform import` against
the `uem_profile` resource's `ImportState` support — and then points you at
`ws1-tf onboard`, which generates the configuration and runs the imports
for you.

## The canonical mechanism: `terraform import`

Importing an existing Workspace ONE UEM object into Terraform state is a
standard `terraform import` invocation against a resource address:

```bash
terraform import uem_application_assignment.example 00000000-0000-0000-0000-000000000001
```

(This is the real, proven example from this repo's README for
`uem_application_assignment`, whose import ID is the application UUID.)

For `uem_profile`, the import ID format is `<profile_id>:<platform>`, for
example:

```bash
terraform import uem_profile.android_corporate 12345:Android
```

This is a real, correct use of Terraform's own `ImportState` mechanism —
nothing about the `terraform import` call itself is a workaround. After
importing, run `terraform plan` and adjust your resource configuration
until Terraform reports no drift.

### Resource name sanitization caveat

Terraform resource names cannot start with a digit or contain special
characters. If the identifier you'd naturally use (e.g., a numeric ID)
isn't a valid Terraform identifier, it must be sanitized first (for
example, `123_test` becomes `profile_123_test`). See the
[Resource Naming Guide](resource-naming.md) for the full detail on that
sanitization behavior.

## Let `ws1-tf onboard` do this for you

Writing `resource "uem_profile" "..." { ... }` blocks and import calls by
hand, one profile at a time, works but doesn't scale past a handful of
objects. The companion `ws1-tf` CLI's `onboard` command reads existing
profiles (and applications, scripts, sensors, smart groups, and update
deployments) from your tenant and generates ready-to-review `.tf` files for
them — including the `terraform import` step — along with the data-source
lookups (or documented `locals`) needed to resolve everything they
reference. Run `ws1-tf list` first to see what's importable before
committing to onboarding anything.

## Summary

- Sanctioned: `terraform import <resource_address> <import_id>` against
  any resource that implements `ImportState` — that's the whole contract.
- Maintained path for onboarding more than a handful of resources:
  `ws1-tf onboard`, which generates the config and runs the imports for
  you.
- Naming: see the [Resource Naming Guide](resource-naming.md) for how
  non-identifier names get sanitized into valid Terraform resource names.
