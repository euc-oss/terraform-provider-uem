// Package providerdata defines the shared holder type passed from the
// provider's Configure to every resource's and data source's Configure via
// ResourceData/DataSourceData.
package providerdata

import sdkclient "github.com/euc-oss/terraform-sdk-uem/v26/client"

// ProviderData is the configured value passed to every resource and data
// source via ResourceData/DataSourceData. It replaces the bare *sdk.Client
// so provider-level config (e.g. app_binary_storage_path) can reach resources
// without a global.
type ProviderData struct {
	Client               *sdkclient.Client
	AppBinaryStoragePath string // set by provider.Configure from app_binary_storage_path (or its default)

	// SecondaryClient holds the client built from the non-primary credential
	// set when both basic and oauth2 credentials are complete (nil
	// otherwise). It is inert today — no resource or data source consumes
	// it — and exists so a future SDK-marked, per-endpoint auth routing
	// change has a client to route to without another Configure-time
	// plumbing change.
	SecondaryClient *sdkclient.Client
}
