package scripts

import (
	"context"
	"fmt"
	"strings"

	"github.com/euc-oss/terraform-provider-uem/internal/pagewalk"
	"github.com/euc-oss/terraform-provider-uem/internal/providerdata"
	"github.com/euc-oss/terraform-provider-uem/internal/scripts/scriptlookup"
	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// scriptSearchPageSize is the page size requested on every call to
// GetScriptsByOrganizationGroupAsync. Mirrors internal/profile/data_source.go's
// profileSearchPageSize (500) so all hand-written paginated data sources in
// this codebase request the same page size.
const scriptSearchPageSize = 500

// maxScriptSearchPages bounds the pagination loop in List. At pageSize=500
// this allows walking up to 50,000 scripts before erroring out -- far beyond
// any real org group's script count -- and exists purely as a termination
// guard against a pathological/misbehaving API response, not as a limit
// expected to be hit in practice. Mirrors
// internal/profile/data_source.go's maxProfilePages.
const maxScriptSearchPages = 100

// scriptSearch implements scriptSearchIface against the real SDK. It is the hand-written
// companion the generated scripts_data_source_gen.go depends on via newSearch/List.
//
// Svc is typed as scriptlookup.Lister -- the exact one-method seam already
// declared by internal/scripts/scriptlookup for the same SDK call -- rather
// than a new interface, and rather than the concrete *sdk.ScriptsV1Service
// (which cannot be faked in tests). Reusing it avoids a duplicate interface
// with an identical signature.
type scriptSearch struct {
	svc scriptlookup.Lister

	// maxPages, when non-zero, overrides maxScriptSearchPages for this
	// instance. It exists purely so tests can inject a tiny cap and prove the
	// termination guard fires against a pathological fake that never signals
	// end-of-results, without waiting out the full production cap. Production
	// code always leaves this unset (zero value) and falls back to
	// maxScriptSearchPages.
	maxPages int
}

func newSearch(pd *providerdata.ProviderData) scriptSearchIface {
	return &scriptSearch{svc: sdk.NewScriptsV1Service(pd.Client)}
}

// List returns every script visible to ogUUID (plus f's other filters,
// applied identically on every page). GetScriptsByOrganizationGroupAsync
// paginates via Page/PageSize request fields + SearchResults/RecordCount
// response fields (no cursor). The page-index BASE is PROBED per call (never
// cached across calls/tenants) and the walk is hardened via
// internal/pagewalk, exactly mirroring internal/profile/data_source.go's
// walkProfiles/probeProfilePageBase (internal-ticket, release blocker B12) and
// internal/scripts/scriptlookup.AbsentFromOrgGroup, which pages the same SDK
// call for a different purpose (confirming a single script's absence):
// progress counted in unique (lower-cased) ScriptUUIDs, RecordCount trusted
// only from non-empty pages and never allowed to change between them, no-
// progress and page-cap are errors, and a multi-page result is walked twice
// end-to-end as an independent confirmation.
//
// Live-confirmed (internal-ticket): GetScriptsByOrganizationGroupAsync is
// 0-indexed on both tenant as<internal-env> (UEM 26.2) and tenant paul-2609 (UEM 26.9),
// and an out-of-range page returns HTTP 200 with an empty SearchResults and
// RecordCount 0 (not a 204) on both -- still handled by the same probe/walk
// since an empty page's reported total is never trusted over an earlier
// non-empty page's.
func (s *scriptSearch) List(ctx context.Context, f scriptFilters) ([]ScriptSummary, error) {
	// organization_group_uuid is a Required schema attribute, but "Required" only rules
	// out null/unknown at the framework level -- an empty string still satisfies it. The
	// SDK call takes the OG-UUID as a positional path parameter, so a blank value here
	// would mean a garbage request; guard defensively rather than let it through.
	ogUUID := f.OrganizationGroupUuid.ValueString()
	if ogUUID == "" {
		return nil, fmt.Errorf("organization_group_uuid is required and must not be empty")
	}

	// The SDK hard-requires a non-nil opts struct (it errors internally otherwise), so
	// always pass one even when no optional filters are set.
	baseOpts := sdk.ScriptsV1GetScriptsByOrganizationGroupAsyncOptions{}
	if !f.Name.IsNull() && !f.Name.IsUnknown() {
		v := f.Name.ValueString()
		baseOpts.Name = &v
	}

	fetch := func(ctx context.Context, page, pageSize int) (pagewalk.Page[sdk.ScriptResourceLiteV1], error) {
		opts := baseOpts
		p, sz := page, pageSize
		opts.Page = &p
		opts.PageSize = &sz
		_, res, err := s.svc.GetScriptsByOrganizationGroupAsync(ctx, ogUUID, &opts)
		if err != nil {
			return pagewalk.Page[sdk.ScriptResourceLiteV1]{}, err
		}
		var items []sdk.ScriptResourceLiteV1
		var total *int
		if res != nil {
			items = res.SearchResults
			total = res.RecordCount
		}
		return pagewalk.Page[sdk.ScriptResourceLiteV1]{Items: items, Total: total}, nil
	}

	key := func(item sdk.ScriptResourceLiteV1) (string, error) {
		if item.ScriptUUID == "" {
			return "", fmt.Errorf("script %q came back from search without a script_uuid; cannot page or dedupe it", item.Name)
		}
		return strings.ToLower(item.ScriptUUID), nil
	}

	maxPages := maxScriptSearchPages
	if s.maxPages > 0 {
		maxPages = s.maxPages
	}

	result, err := pagewalk.WalkTwiceVerified(ctx, fetch, key, scriptSearchPageSize, maxPages, "script search")
	if err != nil {
		return nil, err
	}

	out := make([]ScriptSummary, 0, len(result.Items))
	for _, item := range result.Items {
		out = append(out, ScriptSummary{
			Name:                  types.StringValue(item.Name),
			OrganizationGroupUuid: types.StringValue(item.OrganizationGroupUUID),
			Platform:              types.StringValue(item.Platform),
			ScriptType:            types.StringValue(item.ScriptType),
			ScriptUuid:            types.StringValue(item.ScriptUUID),
			Version:               types.StringValue(item.Version),
		})
	}
	return out, nil
}
