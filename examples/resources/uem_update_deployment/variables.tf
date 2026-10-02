variable "update_uuid" {
  type        = string
  description = "Device update UUID to deploy (see the uem_update_deployments data source, or ws1-tf list)"
}

variable "organization_group_uuid" {
  type        = string
  description = "Organization group UUID where the deployment is created"
}

variable "smart_group_uuids" {
  type        = list(string)
  description = "Smart group UUIDs targeted by the deployment"
}

variable "name" {
  type        = string
  description = "Deployment name"
  default     = "Terraform example deployment"
}

variable "deployment_type" {
  type        = string
  description = "One of DOWNLOAD_AND_INSTALL, DOWNLOAD_ONLY, INSTALL_ONLY"
  default     = "DOWNLOAD_AND_INSTALL"
}

variable "deployment_start_time" {
  type        = string
  description = "UEM datetime when the deployment starts (e.g. 2026-07-28T16:00:00.000Z)"
  default     = "2026-07-28T16:00:00.000Z"
}
