package sensors

import (
	"context"
	"fmt"
	"strings"

	"github.com/euc-oss/terraform-provider-uem/internal/providerdata"
	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// sensorSearch implements sensorSearchIface against the real SDK. It is the hand-written
// companion the generated sensors_data_source_gen.go depends on via newSearch/List.
type sensorSearch struct{ svc *sdk.DeviceSensorsV2Service }

func newSearch(pd *providerdata.ProviderData) sensorSearchIface {
	return &sensorSearch{svc: sdk.NewDeviceSensorsV2Service(pd.Client)}
}

// sensorListPageSize is the page size used when paging
// GET /api/mdm/devicesensors/list/{organizationGroupUuid}. 500 is the
// endpoint's own documented default page size (see the PageSize doc comment
// on sdk.DeviceSensorsV2GetDeviceSensorsAsyncOptions). A variable so tests
// can shrink it.
var sensorListPageSize = 500

// sensorListMaxPages is a last-resort guard against an endless walk: a
// server that keeps advancing pages and keeps returning new sensors but
// never runs out. It is not the defense against a server that ignores the
// page parameter; that is the per-page progress check in walkSensors.
// Exceeding it is an error, never a truncated list. A variable so tests can
// shrink it.
var sensorListMaxPages = 200

// List pages through GET /api/mdm/devicesensors/list/{organizationGroupUuid}
// and returns every sensor visible to f's filters. Before this change, List
// made exactly one call with no Page/PageSize set at all (the SDK options
// support both, default page_size 500) -- any organization group with more
// than 500 sensors got a silently truncated list. On success len(out) ==
// total always; every outcome other than a complete, consistent walk is an
// error, never a partial list.
//
// This reapplies, for sensors, the same defenses already proven for org
// group search (cli/internal/uemapi/orggroups.go, mb1) and the script
// not-found confirmation (internal/scripts/scriptlookup, 7fx's sibling
// concern): progress counted in unique sensor UUIDs (lower-cased, matching
// scriptlookup's script-UUID convention) rather than raw items per page;
// total_results trusted only from pages that hold items (a page past the
// end has been seen, on this endpoint family, to report 0); a Total that
// changes between two non-empty pages is an error, not silently accepted; a
// page cap that errors rather than truncates; and a walk that took more
// than one page is repeated as a second, independent walk, with the result
// returned only if both walks agree on the total AND the same set of UUIDs
// -- a single-page walk (the common case for any organization group under
// the page size) is not repeated, since it has no window for the list to
// shift between calls. See walkSensors for the exact per-page state
// machine (deliberately NOT a `len(out) >= total` early exit -- that
// accepts a page that duplicates existing entries as if it were progress).
func (s *sensorSearch) List(ctx context.Context, f sensorFilters) ([]SensorSummary, error) {
	if f.OrganizationGroupUuid.IsNull() || f.OrganizationGroupUuid.IsUnknown() {
		return nil, fmt.Errorf("organization_group_uuid is required")
	}
	ogUUID := f.OrganizationGroupUuid.ValueString()
	if ogUUID == "" {
		return nil, fmt.Errorf("organization_group_uuid is required")
	}

	var name *string
	if !f.Name.IsNull() && !f.Name.IsUnknown() {
		v := f.Name.ValueString()
		name = &v
	}

	pageSize, maxPages := sensorListPageSize, sensorListMaxPages
	if pageSize <= 0 {
		return nil, fmt.Errorf("invalid sensor list page size %d: must be positive", pageSize)
	}
	items, total, pages, err := s.walkSensors(ctx, ogUUID, name, pageSize, maxPages)
	if err != nil || pages == 1 {
		return items, err
	}
	again, againTotal, _, err := s.walkSensors(ctx, ogUUID, name, pageSize, maxPages)
	if err != nil {
		return nil, fmt.Errorf("second walk of the device sensor list for organization group %s disagrees with the first, which completed with %d sensors: %w", ogUUID, total, err)
	}
	if againTotal != total || !sameSensorUUIDs(items, again) {
		return nil, fmt.Errorf("device sensor list for organization group %s changed between two walks (total_results %d then %d); retry", ogUUID, total, againTotal)
	}
	return items, nil
}

// sameSensorUUIDs reports whether a and b hold the same set of sensor UUIDs
// (lower-cased). Both come from walkSensors, so neither holds a duplicate.
func sameSensorUUIDs(a, b []SensorSummary) bool {
	if len(a) != len(b) {
		return false
	}
	uuids := make(map[string]struct{}, len(a))
	for _, s := range a {
		uuids[strings.ToLower(s.Uuid.ValueString())] = struct{}{}
	}
	for _, s := range b {
		if _, ok := uuids[strings.ToLower(s.Uuid.ValueString())]; !ok {
			return false
		}
	}
	return true
}

// walkSensors is one independent walk of the device sensor list from page
// 0, under the rules documented on List. It returns the distinct sensors in
// first-seen order, the trusted total, and how many pages it fetched.
func (s *sensorSearch) walkSensors(ctx context.Context, ogUUID string, name *string, pageSize, maxPages int) ([]SensorSummary, int, int, error) {
	seen := make(map[string]struct{})
	var out []SensorSummary
	reached := false
	total, haveTotal, totalPage := 0, false, -1
	for page := 0; page < maxPages; page++ {
		fetched := page + 1
		p, size := page, pageSize
		opts := &sdk.DeviceSensorsV2GetDeviceSensorsAsyncOptions{Page: &p, PageSize: &size}
		if name != nil {
			opts.Name = name
		}
		_, res, err := s.svc.GetDeviceSensorsAsync(ctx, ogUUID, opts)
		if err != nil {
			return nil, 0, fetched, fmt.Errorf("unable to list device sensors for organization group %s (page %d): %w", ogUUID, page, err)
		}
		if res == nil || res.TotalResults == nil {
			return nil, 0, fetched, fmt.Errorf("device sensor list response for organization group %s (page %d) is missing total_results", ogUUID, page)
		}
		n := len(res.ResultSet)
		if n > 0 && totalPage >= 0 && *res.TotalResults != total {
			return nil, 0, fetched, fmt.Errorf("device sensor list for organization group %s: total_results changed from %d to %d between page %d and page %d; the list changed during the walk, refusing to return a possibly-incomplete result", ogUUID, total, *res.TotalResults, totalPage, page)
		}
		if n > 0 || !haveTotal {
			total, haveTotal = *res.TotalResults, true
			if n > 0 && totalPage < 0 {
				totalPage = page
			}
		}
		added := 0
		for _, item := range res.ResultSet {
			key := strings.ToLower(item.UUID)
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			summary := SensorSummary{
				Name:                  types.StringValue(item.Name),
				Uuid:                  types.StringValue(item.UUID),
				OrganizationGroupUuid: types.StringValue(item.OrganizationGroupUUID),
				Platform:              types.StringValue(item.Platform),
			}
			if item.ID != nil {
				summary.Id = types.Int64Value(int64(*item.ID))
			}
			out = append(out, summary)
			added++
		}
		unique := len(seen)
		switch {
		case unique > total:
			return nil, 0, fetched, fmt.Errorf("device sensor list for organization group %s returned %d distinct sensors by page %d but total_results is %d", ogUUID, unique, page, total)
		case unique == total && n < pageSize:
			if out == nil {
				out = []SensorSummary{}
			}
			return out, total, fetched, nil
		case unique == total && reached:
			return nil, 0, fetched, fmt.Errorf("device sensor list for organization group %s returned a full page %d after all %d sensors were seen", ogUUID, page, total)
		case unique == total:
			reached = true
		case n == 0:
			return nil, 0, fetched, fmt.Errorf("device sensor list for organization group %s returned an empty page %d after %d of %d sensors", ogUUID, page, unique, total)
		case added == 0:
			return nil, 0, fetched, fmt.Errorf("device sensor list for organization group %s made no progress on page %d (size %d): the server may be ignoring the page parameter", ogUUID, page, pageSize)
		}
	}
	return nil, 0, maxPages, fmt.Errorf("device sensor list for organization group %s exceeded %d pages of %d; refusing to return a truncated list", ogUUID, maxPages, pageSize)
}
