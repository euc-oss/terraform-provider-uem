# Terraform Provider for Omnissa Workspace ONE UEM

![Status: Beta](https://img.shields.io/badge/status-beta-0078D4)

Manage Omnissa Workspace ONE UEM with Terraform: macOS applications, scripts, sensors and their assignments, update deployments, smart groups, and device profiles across platforms.

> [!NOTE]
> **26.2 beta is available.** The current release is **v26.2.0-beta.1**, for UEM 26.2. Its source is on the [`release/26.2.0-beta1`](https://github.com/euc-oss/terraform-provider-uem/tree/release/26.2.0-beta1) branch. `main` is not updated for beta releases.

## Downloads

Both are on the [Releases](https://github.com/euc-oss/terraform-provider-uem/releases) page:

| Release | What it is |
| --- | --- |
| [**v26.2.0-beta.1**](https://github.com/euc-oss/terraform-provider-uem/releases/tag/v26.2.0-beta.1) | The `omnissa/uem` Terraform provider, as a mirror zip with an installer (macOS and Linux) |
| [**ws1-tf v26.2.0-beta.1**](https://github.com/euc-oss/terraform-provider-uem/releases/tag/ws1-tf-v26.2.0-beta.1) | The `ws1-tf` CLI, which generates Terraform configuration from what already exists in your tenant and installs the provider for you |

The provider is not on the Terraform Registry yet. The release notes describe how to install each one.

## What the provider manages

10 resources and 9 data sources. macOS is the primary and most-tested area. `uem_profile` also covers Apple iOS, Android, Windows 10, Windows Rugged and Linux profiles.

- **Resources:** `uem_profile`, `uem_mac_application`, `uem_application_assignment`, `uem_purchased_application_assignment`, `uem_mac_script`, `uem_script_assignment`, `uem_mac_sensor`, `uem_sensor_assignment`, `uem_update_deployment`, `uem_smart_group`
- **Data sources:** `uem_profiles`, `uem_applications`, `uem_mac_applications`, `uem_purchased_applications`, `uem_scripts`, `uem_sensors`, `uem_update_deployments`, `uem_smart_groups`, `uem_organization_groups`

The full reference is in [`docs/`](https://github.com/euc-oss/terraform-provider-uem/tree/release/26.2.0-beta1/docs) on the release branch.

## The `ws1-tf` CLI

`ws1-tf` reads objects that already exist in your Workspace ONE UEM tenant, generates ready-to-review `.tf` files for them, and imports them into Terraform state:

- `ws1-tf init`: sets up a working directory and credentials for a tenant.
- `ws1-tf verify`: checks connectivity to the tenant.
- `ws1-tf list`: shows what can be onboarded.
- `ws1-tf onboard`: generates configuration for the objects you pick, imports them, and confirms a clean `terraform plan`.
- `ws1-tf refresh-local`: brings your local `.tf` files back in line after changes made in the console. It never writes to UEM.

See the [ws1-tf guide](docs/cli/README.md) for details.

## Support

Report issues at https://github.com/euc-oss/terraform-provider-uem/issues.

## License

[Apache License 2.0](LICENSE)
