variable "organization_group_id" {
  type        = string
  description = "Numeric id of the organization group that manages the smart group"
}

# Criteria-based smart group: every corporate-dedicated macOS device on
# a macOS version later than 13.0.
resource "uem_smart_group" "macs" {
  name                    = "terraform_example_macs"
  managed_by_org_group_id = var.organization_group_id
  criteria_type           = "All"

  platforms  = ["AppleOsX"]
  ownerships = ["CorporateDedicated"]

  operating_systems = [
    {
      device_type = "AppleOsX"
      operator    = "GreaterThan"
      value       = "13.0"
    },
  ]
}

# Explicit-membership smart group.
resource "uem_smart_group" "pilot" {
  name                    = "terraform_example_pilot"
  managed_by_org_group_id = var.organization_group_id
  criteria_type           = "UserDevice"

  device_additions  = ["1001", "1002"]
  device_exclusions = ["1003"]
}

output "macs_smart_group_uuid" {
  description = "Smart group UUID assigned by UEM"
  value       = uem_smart_group.macs.uuid
}
