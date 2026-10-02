# Create a macOS (AppleOsX) Network Profile with certificate-based EAP auth
#
# This example demonstrates the two-step credential flow:
#   1. credentials_list — uploads a client certificate (.pfx) to UEM and stores
#      the returned CertificateID in state (credential_source = "Upload").
#   2. network_list     — references that credential by name via identity_certificate.
#
# The provider calls POST /api/mdm/profiles/uploadcertificate (MDM API V1)
# automatically during apply and wires the CertificateID into the profile payload.
#
# Supply sensitive values at runtime — never hardcode them in .tf files:
#
#   Option 1 — environment variables (recommended for CI):
#     export TF_VAR_wifi_password="GuestPassword123"
#     export TF_VAR_cert_password="TestP@ssword123"   # password used when generating client.pfx
#     terraform apply
#
#   Option 2 — .tfvars file (already in .gitignore):
#     wifi_password = "GuestPassword123"
#     cert_password = "TestP@ssword123"
#     terraform apply -var-file="secret.tfvars"
#
# The self-signed test certificate (client.pfx) was generated with:
#   openssl req -x509 -newkey rsa:2048 -sha256 -days 365 -nodes \
#     -keyout client.key -out client.crt \
#     -subj "/CN=EAP-TLS Client Cert/O=Omnissa/OU=IaC-Test/C=US" \
#     -addext "keyUsage=digitalSignature,keyEncipherment" \
#     -addext "extendedKeyUsage=clientAuth"
#   openssl pkcs12 -export -in client.crt -inkey client.key \
#     -out client.pfx -name "EAP-TLS Client Cert" -passout pass:TestP@ssword123
#
# client.key, client.crt, and client.pfx are in .gitignore.

# This provider is not yet published to the Terraform Registry; install it
# from the pre-release mirror zip (see the project README) before running
# `terraform init`. For local development, see "Developing the provider
# locally" in the README for the dev_overrides workflow.
terraform {
  required_version = ">= 1.0"

  required_providers {
    uem = {
      source = "registry.terraform.io/omnissa/uem"
    }
  }
}

provider "uem" {}

variable "wifi_password" {
  description = "Pre-shared key for the Guest WEP network"
  type        = string
  sensitive   = true
}

variable "cert_password" {
  description = "Password for the client certificate .pfx file"
  type        = string
  sensitive   = true
  default     = ""
}

resource "uem_profile" "macos_network" {
  name            = "macOS Wi-Fi Network"
  description     = "Configures Wi-Fi networks on macOS devices"
  platform        = "AppleOsX"
  org_group_id    = "12345" # Update with your OG ID from .env
  assignment_type = "Auto"
  profile_scope   = "Production"
  is_active       = true

  # ── Step 1: credentials_list ────────────────────────────────────────────────
  # Upload a client certificate that the WPA2 Enterprise network will use for
  # EAP-TLS authentication.  The provider calls:
  #   POST /api/mdm/profiles/uploadcertificate
  # and stores the returned CertificateID in state (certificate_id = <computed>).
  #
  # Alternatively, to reference a pre-configured Certificate Authority instead:
  #   credential_source     = "DefinedCertificateAuthority"
  #   certificate_authority = <CA numeric ID from UEM>
  #   certificate_template  = <template numeric ID>
  credentials_list = [
    {
      credential_source    = "Upload"
      credential_name      = "EAP-TLS Client Cert" # referenced by identity_certificate below
      certificate_payload  = filebase64("${path.module}/client.pfx")
      certificate_password = var.cert_password

      # certificate_id is computed after upload — do not set manually
      allow_access_to_all_applications = false
      key_is_extractable               = true # Allow export of private key from Keychain (macOS 10.15+)
    }
  ]

  # ── Step 2: network_list ────────────────────────────────────────────────────
  network_list = [
    # Simple WEP / PSK network
    {
      network_interface                     = "BuiltInWireless"
      service_set_identifier                = "Guest-WiFi"
      hidden_network                        = false
      auto_join                             = false
      disable_association_mac_randomization = false
      security_type                         = "WEP"
      password                              = var.wifi_password

      proxy_type = "None"
    },

    # WPA2 Enterprise with EAP-TLS + the uploaded client certificate
    {
      network_interface                     = "BuiltInWireless"
      service_set_identifier                = "Corp-WiFi"
      hidden_network                        = false
      auto_join                             = true
      disable_association_mac_randomization = false

      security_type       = "WPAEnterprise"
      tls                 = true # enable EAP-TLS
      tls_minimum_version = "1.2"
      tls_maximum_version = "1.2"
      user_name           = "{DeviceUid}" # UEM variable substitution

      # Must match credential_name in credentials_list above
      identity_certificate = "EAP-TLS Client Cert"

      allow_trust_exceptions = true

      proxy_type = "None"
    },
  ]
}

output "profile_id" {
  description = "Created macOS network profile ID"
  value       = uem_profile.macos_network.id
}

output "profile_uuid" {
  description = "Created macOS network profile UUID"
  value       = uem_profile.macos_network.uuid
}

output "certificate_id" {
  description = "UEM certificate ID assigned after upload (use in CredentialsList references)"
  value       = length(uem_profile.macos_network.credentials_list) > 0 ? uem_profile.macos_network.credentials_list[0].certificate_id : null
}

output "profile_details" {
  description = "Full profile details"
  value = {
    id              = uem_profile.macos_network.id
    name            = uem_profile.macos_network.name
    uuid            = uem_profile.macos_network.uuid
    platform        = uem_profile.macos_network.platform
    profile_context = uem_profile.macos_network.profile_context
  }
}
