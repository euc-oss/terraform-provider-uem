module github.com/euc-oss/terraform-provider-uem/scripts

go 1.25.0

// TODO: Update to use main branch once feature/UEMSDK-1-initial-code-migration is merged
require github.com/euc-oss/terraform-sdk-uem v0.0.0-20260423171023-da392942f39f

require (
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-retryablehttp v0.7.8 // indirect
	golang.org/x/sys v0.27.0 // indirect
	golang.org/x/time v0.14.0 // indirect
)
