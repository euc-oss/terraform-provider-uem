# Examples

This directory contains example Terraform configurations for the Omnissa
Workspace ONE UEM provider. The layout follows the common Terraform provider
examples structure used by provider documentation tooling:

- `provider/` contains provider configuration examples.
- `resources/` contains resource examples grouped by Terraform resource type.
- `data-sources/` contains data source examples grouped by Terraform data
  source type.

## Authentication

The examples expect provider configuration to be supplied through environment
variables:

```shell
export UEM_INSTANCE_URL="https://your-instance.awmdm.com"
export UEM_TENANT_CODE="your-tenant-code"
export UEM_AUTH_METHOD="basic"
export UEM_USERNAME="admin@example.com"
export UEM_PASSWORD="your-password"
```

For OAuth2 authentication, use:

```shell
export UEM_AUTH_METHOD="oauth2"
export UEM_CLIENT_ID="your-client-id"
export UEM_CLIENT_SECRET="your-client-secret"
export UEM_OAUTH2_TOKEN_URL="https://na.uemauth.workspaceone.com/connect/token"
```

## Included Examples

- `provider/` shows the minimal provider block.
- `resources/uem_profile/basic-create/` creates a simple profile.
- `resources/uem_profile/import-existing/` supports importing existing
  Workspace ONE UEM profiles.
- `resources/uem_mac_application/basic/` uploads a macOS application.
- `resources/uem_application_assignment/basic/` manages application
  assignments.
- `data-sources/uem_smart_groups/basic/` looks up smart groups.

## Running An Example

Install the local provider first:

```shell
make install
```

Then run Terraform from an example directory:

```shell
cd examples/resources/uem_profile/basic-create
terraform init
terraform plan -var="org_group_id=14165"
```

Review each example before applying it. The resource examples create or update
objects in Workspace ONE UEM.
