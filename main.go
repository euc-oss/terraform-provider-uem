package main

import (
	"context"
	"flag"
	"log"

	"github.com/euc-oss/terraform-provider-uem/internal/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
)

var (
	// These will be set by the goreleaser configuration
	// to appropriate values for the compiled binary.
	version string = "dev"

	// Goreleaser can pass other information to the main package, such as the specific commit
	// https://goreleaser.com/cookbooks/using-main.version/
)

func main() {
	var debug bool

	flag.BoolVar(&debug, "debug", false, "set to true to run the provider with support for debuggers like delve")
	flag.Parse()

	opts := providerserver.ServeOpts{
		// registry.terraform.io/omnissa/uem — matches the required_providers source in a consuming
		// Terraform configuration and the dev_overrides key in a local CLI config. It resolves against
		// the real Terraform Registry only once the provider is published there.
		Address: "registry.terraform.io/omnissa/uem",
		Debug:   debug,
	}

	err := providerserver.Serve(context.Background(), provider.New(version), opts)

	if err != nil {
		log.Fatal(err.Error())
	}
}
