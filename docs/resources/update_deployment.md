---
page_title: "uem_update_deployment Resource - uem"
subcategory: "Updates"
description: |-
  Manages a device update deployment in Workspace ONE UEM using the MDM Updates API v1.
---

# uem_update_deployment (Resource)

Manages a **device update deployment** in Workspace ONE UEM.

Uses:

- `POST /api/mdm/updates/{updateUuid}/groups/{organizationGroupUuid}/deployment`
- `GET` / `PUT` / `DELETE /api/mdm/updates/deployments/{uuid}`

## Example Usage

```hcl
resource "uem_update_deployment" "docs_example" {
  update_uuid             = var.update_uuid
  organization_group_uuid = var.organization_group_uuid
  name                    = var.name
  deployment_type         = var.deployment_type
  deployment_start_time   = var.deployment_start_time
  smart_group_uuids       = var.smart_group_uuids

  notifications = [
    {
      action  = "INSTALL_SUCCESS"
      message = "Update installed successfully"
    }
  ]
}

output "update_deployment_docs_example_id" {
  description = "Deployment UUID assigned by UEM"
  value       = uem_update_deployment.docs_example.id
}
```

Discover update UUIDs (and deployment UUIDs) with the
[`uem_update_deployments`](../data-sources/update_deployments.md) data
source, or with `ws1-tf list`.

## Schema

### Required

- `update_uuid` (String) Device update UUID. Forces replacement.
- `organization_group_uuid` (String) Organization group UUID used on create. Forces replacement.
- `name` (String) Deployment name.
- `deployment_type` (String) One of `DOWNLOAD_AND_INSTALL`, `DOWNLOAD_ONLY`, `INSTALL_ONLY` (case-insensitive; normalised to uppercase).
- `deployment_start_time` (String) UEM datetime for when the deployment starts.
- `smart_group_uuids` (List of String) Smart groups that receive the deployment; must contain at least one UUID. Read back exactly as UEM returns it: a server-omitted list reads as null, not `[]`.

### Optional

- `notifications` (Attributes List) Up to two notification preferences. Read back exactly as UEM returns it: a server-omitted list reads as null, not `[]`.
  - `action` (String) `DOWNLOAD_SUCCESS` or `INSTALL_SUCCESS`, sent to and read from UEM exactly as configured (no uppercasing).
  - `message` (String) Push notification message (max 4000 characters).
  - `message_template_id` (Number) Email template ID (mutually exclusive with `message`).

### Read-Only

- `id` (String) Deployment UUID assigned by UEM.

## Import

Import by deployment UUID:

```shell
terraform import uem_update_deployment.example 00000000-0000-0000-0000-000000000004
```

## API Reference

- [MDM Updates V1](https://developer.omnissa.com/ws1-uem/apis/mdm/v1/)
