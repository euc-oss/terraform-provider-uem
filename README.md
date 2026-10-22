# Terraform Provider for Omnissa Workspace ONE UEM

![Status: Beta](https://img.shields.io/badge/status-beta-0078D4)

Manage Omnissa Workspace ONE UEM with Terraform: macOS applications, scripts, sensors and their assignments, update deployments, smart groups, and device profiles across platforms.

> [!NOTE]
> **26.4 beta is available.** The current release is **v26.4.0-beta.1**, for UEM 26.4. Its source is on the [`release/26.4.0-beta1`](https://github.com/euc-oss/terraform-provider-uem/tree/release/26.4.0-beta1) branch. The 26.2 beta (**v26.2.0-beta.1**, branch [`release/26.2.0-beta1`](https://github.com/euc-oss/terraform-provider-uem/tree/release/26.2.0-beta1)) remains available for UEM 26.2. `main` is not updated for beta releases.

> [!WARNING]
> This provider is a beta. Test every flow thoroughly in a non-production environment before you use it against production.

## Downloads

Both are on the [Releases](https://github.com/euc-oss/terraform-provider-uem/releases) page:

| Release | What it is |
| --- | --- |
| [**v26.4.0-beta.1**](https://github.com/euc-oss/terraform-provider-uem/releases/tag/v26.4.0-beta.1) | The `omnissa/uem` Terraform provider for UEM 26.4, as a mirror zip with an installer (macOS and Linux) |
| [**ws1-tf v26.4.0-beta.1**](https://github.com/euc-oss/terraform-provider-uem/releases/tag/ws1-tf-v26.4.0-beta.1) | The `ws1-tf` CLI, which generates Terraform configuration from what already exists in your tenant and installs the provider for you |

For UEM 26.2, use [v26.2.0-beta.1](https://github.com/euc-oss/terraform-provider-uem/releases/tag/v26.2.0-beta.1) and [ws1-tf v26.2.0-beta.1](https://github.com/euc-oss/terraform-provider-uem/releases/tag/ws1-tf-v26.2.0-beta.1) instead. The 26.4 `ws1-tf` also works with the 26.2.0 provider (see the [ws1-tf guide](docs/cli/getting-started.md#41-picking-the-provider-version-the-compatibility-table)).

The provider is not on the Terraform Registry yet. The release notes describe how to install each one. In short:

1. Download `terraform-provider-uem-pre-release-26.4.0.zip` and unzip it:
   ```sh
   unzip terraform-provider-uem-pre-release-26.4.0.zip -d uem-provider && cd uem-provider
   ```
2. Run the installer. It copies the provider binaries into `~/.terraform.d/plugins` and adds a `provider_installation` / `filesystem_mirror` block to `~/.terraformrc`:
   ```sh
   bash install.sh
   ```
3. Run `terraform init` in your configuration directory. No registry lookup is needed, because the mirror block takes precedence for `omnissa/uem`.

Supported platforms: macOS (`darwin_arm64`, `darwin_amd64`) and Linux (`linux_amd64`, `linux_arm64`; WSL users run the `linux_amd64` binary). Native Windows is not in this bundle yet.

To install the `ws1-tf` CLI, download `ws1-tf-26.4.0.zip`, unzip it, and run its `install.sh`. It puts `ws1-tf` on your `PATH` with no `sudo` required.

## What the provider manages

10 resources and 9 data sources. macOS is the primary and most-tested area. `uem_profile` also covers Apple iOS, Android, Windows 10, Windows Rugged and Linux profiles. Do not assume a platform is supported by a resource unless its own doc page lists it.

- **Resources:** `uem_profile`, `uem_mac_application`, `uem_application_assignment`, `uem_purchased_application_assignment`, `uem_mac_script`, `uem_script_assignment`, `uem_mac_sensor`, `uem_sensor_assignment`, `uem_update_deployment`, `uem_smart_group`
- **Data sources:** `uem_profiles`, `uem_applications`, `uem_mac_applications`, `uem_purchased_applications`, `uem_scripts`, `uem_sensors`, `uem_update_deployments`, `uem_smart_groups`, `uem_organization_groups`

The full reference is in [`docs/`](https://github.com/euc-oss/terraform-provider-uem/tree/release/26.4.0-beta1/docs) on the release branch.

## What's new in 26.4

The set of resources and data sources is the same as in 26.2.

**Provider**

- `tenant_code` (or `UEM_TENANT_CODE`) is required only for Basic authentication. OAuth2 works without it.
- Every UEM request sends a `User-Agent` that identifies the provider, and errors from UEM calls end in `(correlation id: <id>)`. Include that id when you report a failed request.
- `uem_profile`: creating a macOS profile no longer fails with "Provider produced inconsistent result after apply" when UEM fills values the configuration left out. An update that leaves `is_active` unset now keeps the profile's current value instead of activating it. An update that adds a `restrictions` setting without its parent flag set to `true` is rejected at `terraform plan`.
- `uem_smart_groups`: looking up a smart group that does not exist, or that your credentials cannot see, returns an empty list instead of failing.

**`ws1-tf`**

- Defaults to provider 26.4.0 for UEM 26.4 consoles. A 26.2 console keeps provider 26.2.0, and a repo pinned to 26.2.0 keeps its pin.
- OAuth2 without a tenant code works for `init` and `verify`. Basic authentication still needs one.
- The action log records the `User-Agent` and the UEM `X-Correlation-ID` of every call, and errors end in `(correlation id: <id>)`.
- `--provider-zip <path>` (or `WS1TF_PROVIDER_ZIP`) installs the provider without prompting.

## The `ws1-tf` CLI

`ws1-tf` reads objects that already exist in your Workspace ONE UEM tenant, generates ready-to-review `.tf` files for them, and imports them into Terraform state:

- `ws1-tf init`: sets up a working directory and credentials for a tenant.
- `ws1-tf verify`: checks connectivity to the tenant.
- `ws1-tf list`: shows what can be onboarded.
- `ws1-tf onboard`: generates configuration for the objects you pick, imports them, and confirms a clean `terraform plan`.
- `ws1-tf refresh-local`: brings your local `.tf` files back in line after changes made in the console. It never writes to UEM.

See the [ws1-tf guide](docs/cli/README.md) for details.

## Authentication

Configure the provider with either Basic Auth or OAuth2 (client credentials). Both may be set with environment variables:

| Variable | Purpose |
| --- | --- |
| `UEM_INSTANCE_URL` | Workspace ONE UEM instance URL (for example `https://your-instance.awmdm.com`) |
| `UEM_TENANT_CODE` | Workspace ONE UEM tenant code (API key); required for Basic Auth, optional for OAuth2 |
| `UEM_AUTH_METHOD` | `basic` or `oauth2`; optional, and inferred from whichever credentials are configured when unset |
| `UEM_USERNAME` / `UEM_PASSWORD` | Basic Auth credentials |
| `UEM_CLIENT_ID` / `UEM_CLIENT_SECRET` | OAuth2 client credentials |
| `UEM_OAUTH2_TOKEN_URL` | OAuth2 token URL (defaults to `https://na.uemauth.workspaceone.com/connect/token`) |

See [docs/index.md](docs/index.md) for the full provider schema, including the equivalent `provider "uem" { ... }` block attributes and the operational `UEM_HTTP_HEADER_TIMEOUT` / `UEM_HTTP_STALL_TIMEOUT` environment variables.

## Quick start

```hcl
terraform {
  required_providers {
    uem = {
      source = "omnissa/uem"
    }
  }
}

provider "uem" {
  # All configuration can also come from environment variables. See Authentication above.
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

More complete, runnable examples are under [examples/](examples/), including [basic profile creation](examples/resources/uem_profile/basic-create/), [importing an existing profile](examples/resources/uem_profile/import-existing/), and [uploading a macOS application with an assignment](examples/resources/application/main.tf).

## Known issues

- `uem_profile` `scep_list.identity_preference.names`: not yet proven live when omitted on create. Set it explicitly (for example to `[]`) as a workaround.
- `uem_profile` `network_list` EAP `trusted_certificates`: same as above. Set it explicitly on create.
- Creating a macOS Custom Attributes payload (`uem_profile` `custom_attributes`) is not supported. `ws1-tf onboard` refuses profiles that carry a non-empty attribute script.

## Support

This provider is not officially supported by Omnissa. It is a community project maintained by independent developers. Use it at your own risk, and test thoroughly in non-production environments before you deploy to production.

- **GitHub Issues**: [Report bugs or request features](https://github.com/euc-oss/terraform-provider-uem/issues)
- **Discussions**: [Ask questions and share ideas](https://github.com/euc-oss/terraform-provider-uem/discussions)
- **Documentation**: [Read the docs](docs/)

## License

[Apache License 2.0](LICENSE)

## Acknowledgments

- Built with [Terraform Plugin Framework](https://github.com/hashicorp/terraform-plugin-framework)
- Uses the [terraform-sdk-uem](https://github.com/euc-oss/terraform-sdk-uem) SDK
- Based on [HashiCorp's Provider Scaffolding](https://github.com/hashicorp/terraform-provider-scaffolding-framework)
