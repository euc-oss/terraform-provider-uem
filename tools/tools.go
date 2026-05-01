//go:build generate

package tools

// Commented out until provider is registered with HashiCorp Terraform Registry
// import (
// 	_ "github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs"
// )

// Format Terraform code for use in documentation.
// If you do not have Terraform installed, you can remove the formatting command, but it is suggested
// to ensure the documentation is formatted properly.
//go:generate terraform fmt -recursive ../examples/

// Generate documentation.
// Commented out until provider is registered with HashiCorp Terraform Registry
// This requires the provider to be available at registry.terraform.io
//go:generate echo "Documentation generation skipped - provider not yet registered with Terraform Registry"
// Uncomment when ready:
// //go:generate go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs generate --provider-dir .. --provider-name uem
