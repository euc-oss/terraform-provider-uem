package datasource

import (
	"context"
	"fmt"
	"strings"

	"github.com/euc-oss/terraform-provider-uem/internal/pagewalk"
	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
)

// applicationSearchPageSize is the page size requested on every call to
// AppsV2.Search. Mirrors internal/profile/data_source.go's
// profileSearchPageSize (500) so all hand-written paginated data sources in
// this codebase request the same page size.
const applicationSearchPageSize = 500

// maxApplicationSearchPages bounds the pagination loop in walkApplications.
// At pageSize=500 this allows walking up to 50,000 applications before
// erroring out -- far beyond any real org group's application count -- and
// exists purely as a termination guard against a pathological/misbehaving
// API response, not as a limit expected to be hit in practice. Mirrors
// internal/profile/data_source.go's maxProfilePages.
const maxApplicationSearchPages = 100

// walkApplications runs the paged AppsV2 search for opts (already populated
// with whatever filters the specific data source applies -- name/platform for
// uem_applications, organization_group_uuid for uem_mac_applications) and
// returns every matched application. It is shared by both hand-written
// data sources in this package (ApplicationsDataSource.Read and
// macApplicationSearch.List) since both search the exact same
// mam/v2/AppsV2Service.Search endpoint with an identical paging shape.
//
// AppsV2.Search paginates via Page/PageSize request fields + Applications/Total
// response fields (no cursor). The page-index BASE is PROBED per call (never
// cached across calls/tenants) and the walk is hardened via internal/pagewalk,
// exactly mirroring internal/profile/data_source.go's
// walkProfiles/probeProfilePageBase (internal-ticket, release blocker B12):
// progress counted in unique (lower-cased) application UUIDs, the reported
// total trusted only from non-empty pages and never allowed to change between
// them, no-progress and page-cap are errors, and a multi-page result is
// walked twice end-to-end as an independent confirmation.
//
// Live-confirmed (internal-ticket): AppsV2.Search is 0-indexed on both tenant
// as<internal-env> (UEM 26.2) and tenant paul-2609 (UEM 26.9), and an out-of-range page
// returns HTTP 204 (a nil result) on both.
//
// MaxPages, when non-zero, overrides maxApplicationSearchPages -- see
// ApplicationsDataSource.maxPages / macApplicationSearch.maxPages.
func walkApplications(ctx context.Context, svc appsV2SearchAPI, opts *sdk.AppsV2SearchOptions, maxPages int, subject string) ([]sdk.ApplicationV2Model, error) {
	fetch := func(ctx context.Context, page, pageSize int) (pagewalk.Page[sdk.ApplicationV2Model], error) {
		pOpts := *opts
		p, sz := page, pageSize
		pOpts.Page = &p
		pOpts.PageSize = &sz
		_, res, err := svc.Search(ctx, &pOpts)
		if err != nil {
			return pagewalk.Page[sdk.ApplicationV2Model]{}, err
		}
		var items []sdk.ApplicationV2Model
		var total *int
		if res != nil {
			items = res.Applications
			total = res.Total
		}
		return pagewalk.Page[sdk.ApplicationV2Model]{Items: items, Total: total}, nil
	}

	key := func(a sdk.ApplicationV2Model) (string, error) {
		if a.UUID == "" {
			return "", fmt.Errorf("application %q came back from search without a uuid; cannot page or dedupe it", a.ApplicationName)
		}
		return strings.ToLower(a.UUID), nil
	}

	if maxPages <= 0 {
		maxPages = maxApplicationSearchPages
	}

	result, err := pagewalk.WalkTwiceVerified(ctx, fetch, key, applicationSearchPageSize, maxPages, subject)
	if err != nil {
		return nil, err
	}
	return result.Items, nil
}
