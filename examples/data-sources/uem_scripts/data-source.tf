variable "organization_group_uuid" {
  type        = string
  description = "Organization group UUID to search for scripts within"
}

data "uem_scripts" "example" {
  organization_group_uuid = var.organization_group_uuid
  name                    = "example-script"
}

output "example_script_uuids" {
  value = data.uem_scripts.example.scripts[*].script_uuid
}
