package smartgroup

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/euc-oss/terraform-provider-uem/internal/pagewalk"
	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
)

// The hardened smart group search walk, shared by the uem_smart_groups data
// source (search and smart_group_uuid modes) and the uem_smart_group
// resource (import by UUID). Keeping one walk means both callers get the same
// paging, dedup, and double-walk guarantees.

// searchAllSmartGroups walks every page of the smart group search for the
// given filters and returns every matched smart group, in first-seen order,
// via internal/pagewalk.WalkTwiceVerified -- the same hardened,
// page-index-base-agnostic walk shared by uem_scripts and
// uem_applications/uem_mac_applications (internal-ticket, release blocker B12).
//
// The page-index BASE is PROBED per call (page 0 first, falling back to page
// 1 only when page 0 comes back empty/204; never cached across calls or
// tenants -- see internal/pagewalk's probeBase). Progress is counted in
// unique numeric SmartGroupIDs, never in raw items per page, so a server that
// repeats a page, or a list that shifts under the walk, cannot reach Total
// through duplicates. A smart group with no numeric id is an error at once:
// without an id it can neither be deduplicated nor later matched by uuid with
// any confidence of stability across pages. Total is trusted only from
// non-empty pages and must not change between them; a multi-page result is
// walked twice end-to-end (including its own base probe) and the two walks'
// id sets, totals and bases must agree exactly. See internal/pagewalk's doc
// comments for the full per-page decision table and the
// maxSmartGroupSearchPages cap's semantics (a last-resort guard against an
// endless walk; exceeding it is an error, never a truncated result).
func searchAllSmartGroups(ctx context.Context, search smartGroupSearchAPI, name *string, orgGroupID *int, pageSize int) ([]sdk.SmartGroupSearchModelV1, error) {
	if pageSize <= 0 {
		pageSize = defaultSmartGroupSearchPageSize
	}

	fetch := func(ctx context.Context, page, ps int) (pagewalk.Page[sdk.SmartGroupSearchModelV1], error) {
		p, sz := page, ps
		_, res, err := search.SearchAsync(ctx, &sdk.SmartGroupsSearchAsyncOptions{
			Name:                name,
			OrganizationGroupID: orgGroupID,
			Page:                &p,
			PageSize:            &sz,
		})
		if err != nil {
			return pagewalk.Page[sdk.SmartGroupSearchModelV1]{}, err
		}
		var items []sdk.SmartGroupSearchModelV1
		var total *int
		if res != nil {
			items = res.SmartGroups
			total = res.Total
		}
		return pagewalk.Page[sdk.SmartGroupSearchModelV1]{Items: items, Total: total}, nil
	}

	key := func(g sdk.SmartGroupSearchModelV1) (string, error) {
		id, ok := smartGroupID(g)
		if !ok {
			return "", fmt.Errorf("smart group %q came back from search without a numeric id; cannot page or match it", g.Name)
		}
		return strconv.FormatInt(id, 10), nil
	}

	result, err := pagewalk.WalkTwiceVerified(ctx, fetch, key, pageSize, maxSmartGroupSearchPages, "smart group search")
	if err != nil {
		return nil, err
	}
	return result.Items, nil
}

// smartGroupID extracts the numeric id from a SmartGroupSearchModelV1,
// reporting false when the SDK did not populate one.
func smartGroupID(sg sdk.SmartGroupSearchModelV1) (int64, bool) {
	if sg.SmartGroupID != nil {
		return int64(*sg.SmartGroupID), true
	}
	return 0, false
}

// findSmartGroupsByUUID walks the full, unfiltered hardened search and keeps
// only exact, case-insensitive uuid matches, in first-seen order. The search
// API has no uuid parameter, so this is the only way to resolve a uuid. There
// is normally at most one match (UUIDs are unique), but every match is
// returned rather than assuming that, since nothing here enforces server-side
// uniqueness; callers decide what more than one match means.
//
// B16 decision table row #220 (KEEP+cite). UEM source:
// ProviderImpl/src/ProviderImplSln/WanderingWiFi.AirWatch.ProviderImpl/SmartGroups/SmartGroupDataHandler.cs:8498
// (canonical Q33): there is no server-side lower/upper normalization; the
// UUID comes from the DB and is exposed as a Guid on the model with no
// stable-case guarantee beyond default .NET/JSON Guid formatting. A
// case-insensitive comparison is the correct defensive choice precisely
// because no stable-case guarantee exists.
func findSmartGroupsByUUID(ctx context.Context, search smartGroupSearchAPI, uuid string, pageSize int) ([]sdk.SmartGroupSearchModelV1, error) {
	raw, err := searchAllSmartGroups(ctx, search, nil, nil, pageSize)
	if err != nil {
		return nil, err
	}
	var out []sdk.SmartGroupSearchModelV1
	for _, sg := range raw {
		if strings.EqualFold(sg.SmartGroupUUID, uuid) {
			out = append(out, sg)
		}
	}
	return out, nil
}
