package datasource

import (
	"context"

	"github.com/euc-oss/terraform-provider-uem/internal/providerdata"
	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// macApplicationSearch implements macApplicationSearchIface (declared in
// mac_applications_data_source_gen.go) against the real AppsV2 SDK service.
//
// Its svc field is typed as appsV2SearchAPI, the narrow Search seam already declared in
// data_source.go for the hand-written uem_applications data source, rather than a new
// interface, since both concepts search the exact same mam/v2/AppsV2 SDK service with an
// identical method signature; reusing it avoids a duplicate one-method interface.
type macApplicationSearch struct {
	svc appsV2SearchAPI

	// maxPages, when non-zero, overrides maxApplicationSearchPages for this
	// instance. It exists purely so tests can inject a tiny cap and prove the
	// termination guard fires against a pathological fake that never signals
	// end-of-results, without waiting out the full production cap. Production
	// code always leaves this unset (zero value) and falls back to
	// maxApplicationSearchPages.
	maxPages int
}

func newSearch(pd *providerdata.ProviderData) macApplicationSearchIface {
	return &macApplicationSearch{svc: sdk.NewAppsV2Service(pd.Client)}
}

// List implements macApplicationSearchIface by calling AppsV2.Search with NO app-family
// filter. KNOWN LIMITATION: this data source cannot currently scope results to "mac" or
// "internal" applications specifically -- it returns the full AppsV2 result set (optionally
// scoped by org group, see below), identical in app-family scope to the existing
// hand-written `application` data source (internal/application/datasource/data_source.go),
// which has the same characteristic. The composition-map's `filters` array now carries a
// single OPTIONAL entry, organization_group_uuid (see composition-map.json) -- it is the
// only user-facing Terraform filter this concept exposes.
//
// Why: AppsV2SearchOptions.ApplicationType and .ApplicationSource are undocumented *string
// fields with no discoverable enum/accepted-values anywhere in the pinned SDK source, its
// vendored resource-metadata.json, or its test fixtures (exhaustively searched). The one
// unverified lead found -- response fixtures showing application_type: "Internal" -- does
// not resolve the gap even if correct, since apps on OTHER platforms (e.g. Windows) also
// carry application_type "Internal" in the SDK's own test data: an ApplicationType-only
// filter would not scope to macOS specifically even if the value were confirmed. Resolving
// this requires either a live UEM tenant to empirically determine the correct
// ApplicationType/ApplicationSource (and possibly also a Platform) filter combination, or
// SDK-team documentation neither this codebase nor its vendored artifact currently provides.
// See plan Deviation D4
// (internal-design-doc).
func (s *macApplicationSearch) List(ctx context.Context, f macApplicationFilters) ([]MacApplicationSummary, error) {
	opts := &sdk.AppsV2SearchOptions{}

	// organization_group_uuid is OPTIONAL here (unlike scripts/sensors, which require
	// it): existing callers of this data source predate the filter and must see zero
	// behavior change when it's left unset, so opts stays exactly as empty as it was
	// before this field existed unless the caller explicitly set it.
	//
	// A set value -- including an explicit empty string -- is sent as-is: the provider
	// does not second-guess what the caller configured (B16 (b)-row #111 removal; the
	// prior empty-string guard silently rewrote a set value to unset with no UEM basis).
	//
	// Descendant/ancestor OG inclusion when this is omitted follows the UEM API's own
	// default behavior for an omitted organization_group_uuid scope -- that default is
	// undocumented in this SDK repo (swagger overlay gap-fill, no description/default
	// captured) and unverified against a live tenant as of this change. Whether to ever
	// set is_include_apps_from_child_ogs/_parent_ogs explicitly is a separate, unresolved
	// product question and is deliberately not addressed here.
	if !f.OrganizationGroupUuid.IsNull() && !f.OrganizationGroupUuid.IsUnknown() {
		v := f.OrganizationGroupUuid.ValueString()
		opts.OrganizationGroupUUID = &v
	}

	apps, err := walkApplications(ctx, s.svc, opts, s.maxPages, "mac application search")
	if err != nil {
		return nil, err
	}

	out := make([]MacApplicationSummary, 0, len(apps))
	for _, a := range apps {
		item := MacApplicationSummary{
			Name:                  types.StringValue(a.ApplicationName),
			Type:                  types.StringValue(a.ApplicationType),
			BundleId:              types.StringValue(a.BundleID),
			Uuid:                  types.StringValue(a.UUID),
			OrganizationGroupUuid: types.StringValue(a.OrganizationGroupUUID),
		}
		if a.ID != nil {
			item.Id = types.Int64Value(int64(*a.ID))
		}
		out = append(out, item)
	}
	return out, nil
}
