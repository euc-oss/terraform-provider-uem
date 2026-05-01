# Usage

These examples use the local development provider source `local/omnissa/uem`.
Build and install the provider before running them:

```shell
make install
```

## Configure Credentials

Use environment variables so secrets are not committed:

```shell
export UEM_INSTANCE_URL="https://your-instance.awmdm.com"
export UEM_TENANT_CODE="your-tenant-code"
export UEM_AUTH_METHOD="basic"
export UEM_USERNAME="admin@example.com"
export UEM_PASSWORD="your-password"
export UEM_ORG_GROUP_ID="14165"
```

## Initialize And Plan

Run commands from the example directory you want to test:

```shell
cd examples/resources/uem_profile/basic-create
terraform init
terraform plan -var="org_group_id=${UEM_ORG_GROUP_ID}"
```

## Import Existing Profiles

Use the helper script from the repository root:

```shell
./scripts/import-profiles.sh ios_profile 54321 "Apple iOS"
```

The import ID format for `uem_profile` is:

```text
<profile_id>:<platform>
```

Supported platform values include `Android`, `Apple iOS`, `AppleOsX`,
`Windows 10`, `Windows_Rugged`, and `Linux`.

## Notes

- Replace sample organization group IDs, smart group UUIDs, application UUIDs,
  DMG paths, and plist paths before applying.
- Do not commit `.tfvars`, state files, `.terraform/`, or lock files produced
  while running examples locally.
- Run `terraform fmt -recursive examples/` after changing examples.
