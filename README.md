# Terraform Provider for Omnissa Workspace ONE UEM

[![Lint Status](https://github.com/euc-oss/terraform-provider-uem/workflows/Linter/badge.svg)](https://github.com/euc-oss/terraform-provider-uem/actions)
![Status: Pre-Release](https://img.shields.io/badge/status-pre--release-0078D4)

> [!WARNING] This provider is in tech preview. Please ensure flows are thoroughly test before using against a production environment.

---

Manage Omnissa Workspace ONE UEM with Terraform: macOS applications, scripts,
sensors and their assignments, update deployments, smart groups, and device
profiles across platforms. The companion `ws1-tf` CLI scaffolds Terraform
configuration from what's already in your tenant.

This provider is built on the [Terraform Plugin Framework](https://github.com/hashicorp/terraform-plugin-framework)
and uses the [terraform-sdk-uem](https://github.com/euc-oss/terraform-sdk-uem) SDK
for API interactions.

## What it manages

10 resources and 9 data sources. macOS is the primary, most exercised
surface (application upload, scripts, sensors, their assignments, update
deployments, and smart groups); `uem_profile` additionally covers Apple iOS,
Android, Windows 10, Windows_Rugged, and Linux profile payloads. Don't
assume a platform is supported by a resource unless its own doc page lists
it.

### Resources

| Resource | Purpose | Docs |
| --- | --- | --- |
| `uem_profile` | Manages a device profile (macOS, Apple iOS, Android, Windows 10, Windows_Rugged, or Linux) | [docs/resources/profile.md](docs/resources/profile.md) |
| `uem_mac_application` | Uploads a macOS application (DMG/PKG + plist) to Workspace ONE UEM | [docs/resources/mac_application.md](docs/resources/mac_application.md) |
| `uem_application_assignment` | Manages assignment rules for an application by UUID | [docs/resources/application_assignment.md](docs/resources/application_assignment.md) |
| `uem_purchased_application_assignment` | Manages assignment rules for a purchased (VPP) Apple application by UUID | [docs/resources/purchased_application_assignment.md](docs/resources/purchased_application_assignment.md) |
| `uem_mac_script` | Manages a macOS script | [docs/resources/mac_script.md](docs/resources/mac_script.md) |
| `uem_script_assignment` | Manages assignments for a macOS script | [docs/resources/script_assignment.md](docs/resources/script_assignment.md) |
| `uem_mac_sensor` | Manages a macOS device sensor (MDM API V2) | [docs/resources/mac_sensor.md](docs/resources/mac_sensor.md) |
| `uem_sensor_assignment` | Manages assignments for a macOS device sensor (MDM API V2) | [docs/resources/sensor_assignment.md](docs/resources/sensor_assignment.md) |
| `uem_update_deployment` | Manages a device update deployment (MDM Updates API v1) | [docs/resources/update_deployment.md](docs/resources/update_deployment.md) |
| `uem_smart_group` | Manages a smart group (MDM API v1) | [docs/resources/smart_group.md](docs/resources/smart_group.md) |

### Data Sources

| Data Source | Purpose | Docs |
| --- | --- | --- |
| `uem_profiles` | Searches for profiles by platform and organization group | [docs/data-sources/profiles.md](docs/data-sources/profiles.md) |
| `uem_applications` | Searches for applications; looks up IDs, types, and bundle identifiers | [docs/data-sources/applications.md](docs/data-sources/applications.md) |
| `uem_mac_applications` | Searches for applications (see known limitation on its page) | [docs/data-sources/mac_applications.md](docs/data-sources/mac_applications.md) |
| `uem_purchased_applications` | Enumerates purchased (VPP) applications visible to an organization group | [docs/data-sources/purchased_applications.md](docs/data-sources/purchased_applications.md) |
| `uem_scripts` | Searches for macOS scripts within an organization group | [docs/data-sources/scripts.md](docs/data-sources/scripts.md) |
| `uem_sensors` | Searches for macOS device sensors within an organization group | [docs/data-sources/sensors.md](docs/data-sources/sensors.md) |
| `uem_update_deployments` | Enumerates device update deployments across the update catalog | [docs/data-sources/update_deployments.md](docs/data-sources/update_deployments.md) |
| `uem_smart_groups` | Looks up smart groups by id, UUID, name, or organization group | [docs/data-sources/smart_groups.md](docs/data-sources/smart_groups.md) |
| `uem_organization_groups` | Looks up organization groups by search, id, parent, or ancestry | [docs/data-sources/organization_groups.md](docs/data-sources/organization_groups.md) |

Full provider reference, including the schema for every attribute above: [docs/index.md](docs/index.md).

## The `ws1-tf` CLI

`ws1-tf` is a companion CLI, distributed as its own pre-release zip (see
Installation below), that scaffolds Terraform configuration from what
already exists in your Workspace ONE UEM tenant instead of you hand-writing
it:

- `ws1-tf init` — sets up credentials and a working directory.
- `ws1-tf list` — lists what's importable (profiles, macOS apps, scripts,
  sensors, smart groups, update deployments, and their assignments) before
  you commit to onboarding anything.
- `ws1-tf onboard` — reads existing objects from your tenant and generates
  ready-to-review `.tf` files for them, along with the data-source lookups
  (or documented `locals`) needed to resolve everything they reference.

`ws1-tf` also detects and installs the `uem` provider for you the first time
you run it, from a pre-release zip it finds in `~/Downloads`.

## Installation (pre-release)

This provider isn't on the Terraform Registry yet. Pre-release installation
is via a downloadable mirror zip:

1. Download `terraform-provider-uem-pre-release-<version>.zip` and unzip it:
   ```sh
   unzip terraform-provider-uem-pre-release-<version>.zip -d uem-provider && cd uem-provider
   ```
2. Run the installer. This copies the provider binaries into
   `~/.terraform.d/plugins` and adds a `provider_installation` /
   `filesystem_mirror` block to `~/.terraformrc`:
   ```sh
   bash install.sh
   ```
3. `terraform init` in your configuration directory — no registry lookup is
   needed, since the mirror block takes precedence for `omnissa/uem`.

Supported platforms: macOS (`darwin_arm64`, `darwin_amd64`) and Linux
(`linux_amd64`, `linux_arm64`; WSL users run the `linux_amd64` binary).
Native Windows is not in this bundle yet.

To install the `ws1-tf` CLI instead (or as well), download `ws1-tf-<version>.zip`,
unzip it, and run its `install.sh`; it puts `ws1-tf` on your `PATH` with no
`sudo` required.

## Developing the provider locally: dev_overrides

The mirror-zip install above is the normal way to install and run this
provider. If you're instead building the provider from source and want
Terraform to use your local build without publishing anything, Terraform's
`dev_overrides` mechanism points `terraform` straight at a binary on disk
and skips `terraform init` entirely for `omnissa/uem`:

```hcl
# dev.tfrc
provider_installation {
  dev_overrides {
    "registry.terraform.io/omnissa/uem" = "/path/to/your/local/terraform-provider-uem/build/dir"
  }
  direct {}
}
```

Point Terraform at it with `TF_CLI_CONFIG_FILE=/path/to/dev.tfrc terraform ...`.
Terraform prints a "development overrides are in effect" warning when the
override is active — that's expected, not an error.

## Authentication

Configure the provider with either Basic Auth or OAuth2 (client
credentials). Both may be set via environment variables:

| Variable | Purpose |
| --- | --- |
| `UEM_INSTANCE_URL` | Workspace ONE UEM instance URL (e.g. `https://your-instance.awmdm.com`) |
| `UEM_TENANT_CODE` | Workspace ONE UEM tenant code (API key) |
| `UEM_AUTH_METHOD` | `basic` or `oauth2`; optional — inferred from whichever credentials are configured when unset |
| `UEM_USERNAME` / `UEM_PASSWORD` | Basic Auth credentials |
| `UEM_CLIENT_ID` / `UEM_CLIENT_SECRET` | OAuth2 client credentials |
| `UEM_OAUTH2_TOKEN_URL` | OAuth2 token URL (defaults to `https://na.uemauth.workspaceone.com/connect/token`) |

See [docs/index.md](docs/index.md) for the full provider schema, including
the equivalent `provider "uem" { ... }` block attributes and the operational
`UEM_HTTP_HEADER_TIMEOUT` / `UEM_HTTP_STALL_TIMEOUT` environment variables.

## Quick Start

```hcl
terraform {
  required_providers {
    uem = {
      source = "omnissa/uem"
    }
  }
}

provider "uem" {
  # All configuration can also come from environment variables — see Authentication above.
}

resource "uem_profile" "example" {
  name            = "Example Profile"
  platform        = "AppleOsX"
  org_group_id    = "12345"
  assignment_type = "Optional"
  profile_scope   = "Staging"
  is_active       = false
}

output "profile_id" {
  value = uem_profile.example.id
}
```

More complete, runnable examples are under [examples/](examples/), including
[basic profile creation](examples/resources/uem_profile/basic-create/),
[importing an existing profile](examples/resources/uem_profile/import-existing/),
and [uploading a macOS application with an assignment](examples/resources/application/main.tf).

## Known issues

- `uem_profile` `scep_list.identity_preference.names`: not yet proven live
  when omitted on create. Set it explicitly (e.g. to `[]`) as a workaround.
- `uem_profile` `network_list` EAP `trusted_certificates`: same as above —
  set it explicitly on create.
- Creating a macOS Custom Attributes payload (`uem_profile`
  `custom_attributes`) isn't supported in 26.2; `ws1-tf onboard` refuses
  profiles that carry a non-empty attribute script.

## Support

This provider is not officially supported by Omnissa. It is a community
project maintained by independent developers. Use at your own risk and
always test thoroughly in non-production environments before deploying to
production.

- **GitHub Issues**: [Report bugs or request features](https://github.com/euc-oss/terraform-provider-uem/issues)
- **Discussions**: [Ask questions and share ideas](https://github.com/euc-oss/terraform-provider-uem/discussions)
- **Documentation**: [Read the docs](docs/)

## Acknowledgments

- Built with [Terraform Plugin Framework](https://github.com/hashicorp/terraform-plugin-framework)
- Uses [terraform-sdk-uem](https://github.com/euc-oss/terraform-sdk-uem) SDK
- Based on [HashiCorp's Provider Scaffolding](https://github.com/hashicorp/terraform-provider-scaffolding-framework)
