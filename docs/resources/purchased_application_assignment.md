---
page_title: "uem_purchased_application_assignment Resource - uem"
subcategory: "Application"
description: |-
  Manages assignment rules for a purchased (VPP) Apple application by UUID using the MAM Apps API v2.
---

# uem_purchased_application_assignment (Resource)

Manages assignment rules for a **purchased (VPP) Apple application** in Workspace ONE UEM.
This resource uses `GET/PUT /api/mam/apps/{applicationUuid}/assignment-rules` with
`Accept: application/json;version=2`.

Use `uem_application_assignment` for internal and public applications.
Use this resource when you need VPP license allocation (`vpp_app_details.license_usage`).
Smart groups are assigned only through `license_usage`; `distribution.smart_groups`
is not supported for purchased apps.

## Example Usage

```hcl
variable "application_uuid" {
  type        = string
  description = "UUID of the purchased (VPP) application to manage assignment rules for"
}

variable "smart_group_uuid" {
  type        = string
  description = "Smart group UUID to license the app to"
}

resource "uem_purchased_application_assignment" "example" {
  application_uuid = var.application_uuid

  assignments = [
    {
      priority = 0
      distribution = {
        name                = "All users"
        description         = "VPP assignment"
        app_delivery_method = "AUTO"
        vpp_app_details = {
          license_usage = [
            {
              smart_group_uuid = var.smart_group_uuid
              allocated        = 1
            },
          ]
        }
      }
      restriction = {
        remove_on_unenroll = true
      }
    },
  ]
}

output "purchased_application_assignment_id" {
  description = "Assignment resource ID (mirrors application_uuid)"
  value       = uem_purchased_application_assignment.example.id
}
```

Discover purchased app UUIDs with the
[`uem_purchased_applications`](../data-sources/purchased_applications.md)
data source, or with `ws1-tf list`.

## Schema

### Required

- `application_uuid` (String) UUID of the purchased application.

### Optional

- `assignments` (Attributes List) Assignment rules. Each block requires `priority` and `distribution`.
- `excluded_smart_groups` (List of String) Smart group UUIDs excluded from the assignment. Omitting it from
  configuration resets it to `[]` on apply, the same as its `uem_application_assignment` sibling.

### Read-Only

- `id` (String)

### Nested `assignments`

- `priority` (Number, Required) Assignment priority (0 is highest). The priorities of the n assignments must be exactly
  0 to n-1, with no gaps or duplicates: UEM itself rejects anything else. Separately, `terraform plan` also requires the
  assignments to be listed in ascending priority order (0 first) — this ordering requirement is a provider-side
  limitation (UEM's own API accepts an out-of-order list), not a UEM rule.
- `distribution` (Attributes, Required) Distribution settings.
- `restriction` (Attributes, Optional) Restriction flags. Within a configured block,
  `remove_on_unenroll`, `prevent_removal`, and `managed_access` default to `false`.
- `tunnel` (Attributes, Optional) Per-app VPN profile UUIDs.
- `application_configuration` (Attributes List, Optional) App configuration key/value pairs.
- `application_attributes` (Attributes List, Optional) App attribute key/value pairs.
- `is_dynamic_template_saved` (Boolean, Optional) Whether DDUI template state is saved.

### Restriction flags by platform

UEM silently ignores some `restriction` flags depending on the VPP app's platform:

| Flag | iOS | macOS |
|---|---|---|
| `remove_on_unenroll` | applied | applied |
| `prevent_removal` | applied | ignored |
| `prevent_application_backup` | applied | ignored |
| `make_app_mdm_managed` | applied | ignored |
| `managed_access` | applied; forced to `true` when `make_app_mdm_managed` or `prevent_application_backup` is `true` | ignored |
| `desired_state_management` | ignored | ignored |

`is_dynamic_template_saved` is also ignored on both platforms.

`terraform plan` checks these rules before anything is written:

- `desired_state_management = true` or `is_dynamic_template_saved = true` is an error on any platform.
- When an assignment sets `prevent_removal`, `prevent_application_backup`, `make_app_mdm_managed` or `managed_access` to
  `true`, the provider looks up the app's platform by searching the purchased-app catalog (a verified, paginated walk).
  On macOS each of these flags set to `true` is an error. On iOS, when `make_app_mdm_managed` or `prevent_application_backup`
  is `true`, an unset `managed_access` is planned as `true`, and an explicit `managed_access = false` is an error.
- If the platform lookup fails, the plan shows a warning and continues; the apply-time check below still applies.

After each create or update the provider compares the requested flags with what UEM stored. If a flag set to `true` was not
applied, or `managed_access` differs from the configuration, the apply fails with an error naming each flag and the likely
reason, and Terraform state records the values UEM actually stored. On iOS, set `managed_access = true` whenever
`make_app_mdm_managed` or `prevent_application_backup` is `true`.

If this happens on a **create**, Terraform marks the resource as tainted, so the next apply replaces it: the delete step clears
all of the app's assignment rules, then the create runs again. To avoid the replace, correct the configuration to the values UEM
actually applies, then run `terraform untaint <address>` before applying. On an **update**, the resource stays in place.

### Nested `distribution`

- `name` (String, Required) Assignment name.
- `description` (String, Optional) Assignment description.
- `smart_groups` (List of String, Optional) Must be left unset or empty. UEM
  ignores this field for VPP apps, so a non-empty list is rejected at plan
  time; assign smart groups with `vpp_app_details.license_usage` instead.
  Refresh still reports any groups UEM returns here.
- `app_delivery_method` (String, Optional) e.g. `AUTO`, `ON_DEMAND`.
- `effective_date` (String, Optional) Not applicable to purchased (VPP) apps: UEM ignores it, so setting it is rejected at plan time.
- `vpp_app_details` (Attributes, Optional) VPP license targeting.
  - `license_usage` (Attributes List, Optional) Smart groups that receive the app, with the
    VPP licenses allocated to each. This is the only way to assign a smart
    group to a purchased app.
    - `smart_group_uuid` (String, Required)
    - `allocated` (Number, Required)
    - `redeemed` (Number, Computed) Redeemed license count from UEM.

Each assignment must set `distribution.vpp_app_details.license_usage`, with one
entry per smart group. This is enforced at **apply** time (on both create and update),
not at plan time: an assignment missing `license_usage` passes `terraform plan` and only
fails when you run `terraform apply`. Setting `distribution.smart_groups` fails at plan time
with an error telling you to use `vpp_app_details.license_usage`, because UEM
silently drops `smart_groups` for VPP apps.

`license_usage` reads back as `null`, not an empty list, whenever UEM omits it
(the server never returns an empty array for this field — it either sends
populated entries or omits the field entirely).

### Nested `restriction`

- `remove_on_unenroll` (Boolean, Optional)
- `prevent_removal` (Boolean, Optional)
- `prevent_application_backup` (Boolean, Optional)
- `make_app_mdm_managed` (Boolean, Optional)
- `managed_access` (Boolean, Optional) Whether UEM manages this app's access restrictions. Defaults to false when left
  unset. On iOS, UEM forces this on (and rejects an explicit false) whenever `make_app_mdm_managed` or
  `prevent_application_backup` is true.
- `desired_state_management` (Boolean, Optional)

### Nested `tunnel`

- `per_app_vpn_profile_uuid` (String, Optional)
- `afw_per_app_vpn_profile_uuid` (String, Optional)
- `amapi_per_app_vpn_profile_uuid` (String, Optional)

UEM's assignment update is a full replace, not a merge: if a refresh no longer
returns one of these UUIDs, it reads back as `null` — reflecting that UEM
actually cleared it — rather than silently keeping the last-known value.

### Nested `application_configuration` / `application_attributes`

- `key` (String, Required)
- `value` (String, Required)
- `type` (String, Required) One of `STRING`, `INTEGER`, `BOOLEAN`, `CHOICE`, `MULTISELECT`, `HIDDEN`,
  `BUNDLE`, or `BUNDLE_ARRAY` (case-insensitive), or the equivalent numeric code UEM uses on the wire.

## Import

Import by application UUID:

```shell
terraform import uem_purchased_application_assignment.example 00000000-0000-0000-0000-000000000003
```

## API Reference

- [MAM Apps assignment-rules (v2)](https://developer.omnissa.com/ws1-uem/apis/mam/v2/)
