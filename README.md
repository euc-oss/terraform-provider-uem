# Terraform Provider for Omnissa Workspace ONE UEM

[License](LICENSE)
[Tests](https://github.com/euc-oss/terraform-provider-uem/actions)
Status: Tech Preview

> [!WARNING] This provider is in tech preview. Please ensure flows are thoroughly test before using against a production environment.

---

Manage Omnissa Workspace ONE UEM profiles, macOS applications, and application assignments using Terraform.

This provider is built on the [Terraform Plugin Framework](https://github.com/hashicorp/terraform-plugin-framework) and uses the [terraform-sdk-uem](https://github.com/euc-oss/terraform-sdk-uem) SDK for API interactions.

## Overview

The Workspace ONE Terraform Provider enables Infrastructure as Code (IaC)
management of Omnissa Workspace ONE UEM resources. Automate profile creation,
updates, and lifecycle management for certain macOS Profiles.

### Key Features

- ✅ **Profile Management**: Full CRUD operations for Workspace ONE profiles
- ✅ **Application Management**: Upload macOS internal applications with Terraform
- ✅ **Assignment Management**: Manage application assignment rules by Smart Group
- ✅ **Import Existing Resources**: Bring existing profiles under Terraform
management
- ✅ **Dual Authentication**: Basic Auth and OAuth2 (client credentials flow)
- ✅ **Type-Safe Implementation**: Built with Go and Terraform Plugin Framework
- ✅ **Helper Scripts**: CLI tools for profile and application discovery/import
- ✅ **Comprehensive Documentation**: Guides, examples, and API reference

### Supported Resources


| Resource                     | Description                                        | Status      |
| ---------------------------- | -------------------------------------------------- | ----------- |
| `uem_profile`                | Manage device profiles across all platforms        | ✅ Available |
| `uem_mac_application`        | Upload and manage macOS internal applications      | ✅ Available |
| `uem_application_assignment` | Manage assignment rules for an application by UUID | ✅ Available |


## Documentation

### Quick Links

- **[Provider Documentation](docs/index.md)** - Complete provider reference
- **[Examples](examples/resources/uem_profile/)** - Usage examples
  - [Basic Profile Creation](examples/resources/uem_profile/basic-create/)
  - [Import Existing Profiles](examples/resources/uem_profile/import-existing/)
- **[Application Example](examples/resources/uem_mac_application/main.tf)** - macOS app + assignment example

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.0
- [Go](https://golang.org/doc/install) >= 1.24

## Quick Start

### 1. Install the Provider

```bash
make install
```

### 2. Configure Environment

Create a `.env` file in the repository root:

```bash
# Workspace ONE Instance
UEM_INSTANCE_URL=https://your-instance.awmdm.com
UEM_TENANT_CODE=your-tenant-code

# Authentication
UEM_AUTH_METHOD=basic
UEM_USERNAME=your-api-username
UEM_PASSWORD=your-api-password

# Organization Group
UEM_ORG_GROUP_ID=12345
```

### 3. Create a Profile

See the [basic-create example](examples/resources/uem_profile/basic-create/)
for a complete working example.

```hcl
terraform {
  required_providers {
    uem = {
      source = "local/omnissa/uem"
    }
  }
}

provider "uem" {
  # Configuration loaded from .env file
}

resource "uem_profile" "example" {
  name             = "Example Profile"
  platform         = "AppleOSx"
  org_group_id     = "12345"
  assignment_type  = "Optional"
  profile_scope    = "Test"
  is_active        = false
}
```

### 4. Apply Configuration

```bash
cd examples/resources/uem_profile/basic-create/
terraform init
terraform apply
```

### 5. Create a macOS Application + Assignment

See the [application example](examples/resources/application/main.tf) for a
working end-to-end sample.

```hcl
resource "uem_mac_application" "example" {
  org_group_id    = 12345
  dmg_file_path   = "./apps/Ws1IntelligentHub-24.01.dmg"
  plist_file_path = "./apps/Ws1IntelligentHub-24.01.plist"
  app_version     = "24.01"
}

resource "uem_application_assignment" "example" {
  application_uuid      = uem_mac_application.example.uuid
  excluded_smart_groups = []

  assignments = [
    {
      priority = 0
      distribution = {
        name                = "Ws1IntelligentHub Auto Install"
        description         = "Deploy to managed macOS devices"
        smart_groups        = ["b012031c-c54e-485c-86e8-5126ed0d1480"]
        app_delivery_method = "AUTO"
      }
      restriction = {
        remove_on_unenroll = true
      }
    }
  ]
}
```

## Examples

All examples are located in the `examples/` directory:

- **[Basic Profile Creation](examples/resources/uem_profile/basic-create/)** -
Create new profiles from scratch
- **[Import Existing Profiles](examples/resources/uem_profile/import-existing/)** -
Import and manage existing profiles
- **[Application + Assignment](examples/resources/application/main.tf)** -
Upload a macOS app and manage assignment rules

Each example includes:

- Complete Terraform configuration
- Detailed README with step-by-step instructions
- Usage examples for different platforms


### Local Development Installation

For local development and testing:

```bash
# Clone the repository
git clone https://github.com/euc-oss/terraform-provider-uem.git
cd terraform-provider-uem

# Build and install
make install
```

The provider will be installed to your local Terraform plugins directory.

## Developing the Provider

If you wish to work on the provider, you'll first need
[Go](http://www.golang.org) installed on your machine (see
[Requirements](#requirements) above).

### Development Workflow

```bash
# Clone the repository
git clone https://github.com/euc-oss/terraform-provider-uem.git
cd terraform-provider-uem

# Install dependencies
go mod download

# Build the provider
go build -v .

# Install locally for testing
make install

# Run tests
make test

# Run linter
make lint

# Generate documentation
make generate

# Format code
make fmt
```

### Running Tests

```bash
# Run unit tests
go test ./...

# Run with coverage
go test -cover ./...

# Run acceptance tests (requires Workspace ONE instance)
TF_ACC=1 make testacc
```

*Note:* Acceptance tests create real resources in your Workspace ONE
environment.

### Project Structure

```text
terraform-provider-uem/
├── docs/                       # Provider documentation
│   ├── index.md               # Provider configuration
│   ├── data-sources/          # Data source documentation
│   └── resources/             # Resource documentation
├── examples/                   # Example Terraform configurations
│   ├── data-sources/          # Data source examples
│   ├── provider/              # Provider configuration example
│   ├── resources/             # Resource examples
│   ├── README.md              # Examples overview
│   └── USAGE.md               # Example usage guide
├── internal/                   # Provider implementation
│   ├── application/           # Application and assignment resources
│   ├── common/                # Shared provider helpers
│   ├── provider/              # Provider wiring
│   ├── profile/               # Profile resource implementation
│   └── smartgroup/            # Smart group data source implementation
├── scripts/                    # Helper scripts
│   ├── list-profiles.sh       # List profiles CLI
│   ├── import-profiles.sh     # Import helper CLI
│   ├── list-application.sh    # List applications helper script
│   └── quick-check.sh         # Local quick validation script
├── tools/                      # Code generation tooling
├── go.mod                      # Go module definition
├── main.go                     # Provider entry point
└── Makefile                    # Build automation
```

## Contributing

Contributions are welcome! Please follow these guidelines:

### Reporting Issues

- Check existing issues before creating a new one
- Provide detailed information about the problem
- Include Terraform version, provider version, and Go version
- Share relevant configuration and error messages

## Support

### Community Support

- **GitHub Issues**:
[Report bugs or request features](https://github.com/euc-oss/terraform-provider-uem/issues)
- **Discussions**:
[Ask questions and share ideas](https://github.com/euc-oss/terraform-provider-uem/discussions)
- **Documentation**: [Read the docs](docs/)

## Acknowledgments

- Built with
[Terraform Plugin Framework](https://github.com/hashicorp/terraform-plugin-framework)
- Uses [terraform-sdk-uem](https://github.com/euc-oss/terraform-sdk-uem) SDK
- Based on
[HashiCorp's Provider Scaffolding](https://github.com/hashicorp/terraform-provider-scaffolding-framework)

---

