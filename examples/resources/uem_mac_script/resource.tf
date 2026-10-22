variable "organization_group_uuid" {
  type        = string
  description = "Organization group UUID that owns the script"
}

resource "uem_mac_script" "example" {
  organization_group_uuid = var.organization_group_uuid
  name                    = "Example Script"
  description             = "Sample bash script managed by Terraform"
  platform                = "APPLE_OSX"
  script_type             = "BASH"
  execution_context       = "SYSTEM"
  platform_architecture   = "UNKNOWN"

  # script_data must be base64-encoded. Use filebase64() to load a script from disk.
  script_data = base64encode(<<-EOT
    #!/bin/bash
    echo "hello from terraform"
  EOT
  )

  timeout = 30

  script_variables = [
    {
      name  = "GREETING"
      value = "hello"
    },
  ]

  allowed_in_catalog = true
  user_interaction   = false

  catalog_display = {
    display_name     = "Example Script"
    display_desc     = "Runs a simple greeting script"
    pre_action_text  = "This script will run on your Mac. Continue?"
    post_action_text = "The script has finished running."
    action_type      = "INSTALL"
    catalog_icon_url = "https://example.com/icons/script.png"
    categories       = ["1"]
  }
}

output "mac_script_id" {
  description = "Script UUID assigned by UEM"
  value       = uem_mac_script.example.id
}
