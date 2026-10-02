// Package scriptlookup holds the confirming OG-scoped script list lookup
// shared by the uem_macos_script and uem_script_assignment resources. Both
// run it after an ambiguous HTTP 500 errorCode 1000 to decide whether the
// script is really gone.
package scriptlookup

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
)

// DefaultPageSize is the page size used when paging the OG-scoped script
// list.
const DefaultPageSize = 500

// DefaultMaxPages is a last-resort guard against an endless walk: a server
// that keeps advancing pages and keeps returning new scripts but never runs
// out. It is not the defense against a server that ignores the page
// parameter; that is the per-page progress check in AbsentFromOrgGroup,
// which fails on the second repeated page. Exceeding it is an error, never a
// truncated "absent".
const DefaultMaxPages = 200

// Lister is the one scripts service call the lookup needs.
type Lister interface {
	GetScriptsByOrganizationGroupAsync(
		ctx context.Context,
		OrganizationGroupUUID string,
		opts *sdk.ScriptsV1GetScriptsByOrganizationGroupAsyncOptions,
	) (http.Header, *sdk.ScriptsSearchResultV1, error)
}

// AbsentFromOrgGroup pages through the OG-scoped script list and reports
// whether scriptUUID is absent from it. A match (case-insensitive) returns
// (false, nil) at once. Every other outcome except a confirmed absence is
// (false, err), so a caller that treats only (true, nil) as "gone" can never
// drop state on a partial or inconsistent walk.
//
// Progress is counted in unique ScriptUUIDs (lowercased), never in raw items
// per page, so a server that repeats a page, or a list that shifts under
// the walk, cannot reach RecordCount through duplicates.
//
// RecordCount is the grand total only on pages that hold items. The live
// server answers a page past the end with no items and RecordCount 0, so an
// empty page never overwrites a total learned from an earlier non-empty
// page; an empty page 0 (no earlier total) uses its own RecordCount, which
// is 0 for an organization group with no scripts. Per page, with n items,
// unique the distinct uuids seen so far and total the last trusted
// RecordCount:
//
//   - unique > total: error. More distinct scripts exist than the server
//     claims, so RecordCount cannot be trusted to bound the walk.
//   - unique == total and n < pageSize (a short or empty page): absent. Every
//     script the server counts has been seen, and the server has signalled
//     there is no more data.
//   - unique == total and a full page that reached total: fetch one more page
//     to confirm the end (a total that is an exact multiple of pageSize is
//     legitimate and ends on a full page).
//   - unique == total and a full page when total was already reached before
//     it: error. The server keeps returning full pages past the end.
//   - unique < total and an empty page: error (the list ran out early).
//   - unique < total and a page that added no new uuid: error. The server is
//     repeating pages (for example ignoring the page parameter); continuing
//     would only re-read the same items.
//
// Absent therefore needs both unique == total and a short or empty page.
// Neither half alone is enough: a count reached by duplicates is impossible
// because only distinct uuids count, and a short page with unique < total is
// never read as the end.
//
// A list that changes during the walk shifts the offsets under it, so a
// later page can skip a script that still exists. The lookup guards this in
// two ways. First, once a non-empty page has set the total, every later
// non-empty page must report the same RecordCount; a RecordCount that
// disagrees between two non-empty pages is an error. Second, a walk that
// concludes absent after fetching more than one page is repeated as a second,
// independent walk from page 0, and absent is returned only if that walk also
// concludes absent with the same total; any disagreement, including an error
// in the second walk, is an error. A single-page walk (the common case, any
// organization group under the page size) makes exactly one server call and
// has no window for the list to shift between calls, so it is not repeated.
//
// This does not close every race. A combination of deletes and inserts that
// nets the same total, shifts the offsets so the target is skipped, and is
// reproduced identically on both walks (the same script uuids visible) looks
// exactly like a stable list, and the lookup cannot tell the two apart from
// the list alone. It would also report a false absent if RecordCount itself
// undercounts the scripts the server returns and the server stops early. The
// page size and page cap are parameters so callers' tests can shrink them.
func AbsentFromOrgGroup(
	ctx context.Context,
	svc Lister,
	orgGroupUUID string,
	scriptUUID string,
	pageSize int,
	maxPages int,
) (bool, error) {
	if pageSize <= 0 {
		return false, fmt.Errorf("invalid pageSize %d: must be positive", pageSize)
	}
	absent, total, pages, err := walkOrgGroup(ctx, svc, orgGroupUUID, scriptUUID, pageSize, maxPages)
	if err != nil || !absent || pages == 1 {
		return absent, err
	}
	again, againTotal, _, err := walkOrgGroup(ctx, svc, orgGroupUUID, scriptUUID, pageSize, maxPages)
	switch {
	case err != nil:
		return false, fmt.Errorf("second walk of the script list for organization group %s disagrees with the first, which found script %s absent: %w", orgGroupUUID, scriptUUID, err)
	case !again:
		return false, fmt.Errorf("script list for organization group %s changed between two walks: the first found script %s absent, the second found it present", orgGroupUUID, scriptUUID)
	case againTotal != total:
		return false, fmt.Errorf("script list for organization group %s changed between two walks: RecordCount %d then %d; refusing to treat script %s as absent", orgGroupUUID, total, againTotal, scriptUUID)
	}
	return true, nil
}

// walkOrgGroup is one independent walk of the OG-scoped script list, from
// page 0, under the rules documented on AbsentFromOrgGroup. It returns
// whether scriptUUID is absent, the trusted total, and how many pages it
// fetched. A match returns (false, 0, pages, nil).
func walkOrgGroup(
	ctx context.Context,
	svc Lister,
	orgGroupUUID string,
	scriptUUID string,
	pageSize int,
	maxPages int,
) (bool, int, int, error) {
	seen := make(map[string]struct{})
	reached := false
	total, haveTotal, totalPage := 0, false, -1
	for page := 0; page < maxPages; page++ {
		fetched := page + 1
		p, size := page, pageSize
		_, result, err := svc.GetScriptsByOrganizationGroupAsync(ctx, orgGroupUUID, &sdk.ScriptsV1GetScriptsByOrganizationGroupAsyncOptions{
			Page:     &p,
			PageSize: &size,
		})
		if err != nil {
			return false, 0, fetched, fmt.Errorf("unable to list scripts for organization group %s (page %d): %w", orgGroupUUID, page, err)
		}
		if result == nil || result.RecordCount == nil {
			return false, 0, fetched, fmt.Errorf("script list response for organization group %s (page %d) is missing RecordCount", orgGroupUUID, page)
		}
		n := len(result.SearchResults)
		if n > 0 && totalPage >= 0 && *result.RecordCount != total {
			return false, 0, fetched, fmt.Errorf("script list for organization group %s: RecordCount changed from %d to %d between page %d and page %d; the list changed during the walk, refusing to treat script %s as absent", orgGroupUUID, total, *result.RecordCount, totalPage, page, scriptUUID)
		}
		if n > 0 || !haveTotal {
			total, haveTotal = *result.RecordCount, true
			if n > 0 && totalPage < 0 {
				totalPage = page
			}
		}
		added := 0
		for _, s := range result.SearchResults {
			if strings.EqualFold(s.ScriptUUID, scriptUUID) {
				return false, 0, fetched, nil
			}
			key := strings.ToLower(s.ScriptUUID)
			if _, dup := seen[key]; !dup {
				seen[key] = struct{}{}
				added++
			}
		}
		unique := len(seen)
		switch {
		case unique > total:
			return false, 0, fetched, fmt.Errorf("script list for organization group %s returned %d distinct scripts by page %d but RecordCount is %d; refusing to treat script %s as absent", orgGroupUUID, unique, page, total, scriptUUID)
		case unique == total && n < pageSize:
			return true, total, fetched, nil
		case unique == total && reached:
			return false, 0, fetched, fmt.Errorf("script list for organization group %s returned a full page %d after all %d records were seen; refusing to treat script %s as absent", orgGroupUUID, page, total, scriptUUID)
		case unique == total:
			reached = true
		case n == 0:
			return false, 0, fetched, fmt.Errorf("script list for organization group %s returned an empty page %d after %d of %d records", orgGroupUUID, page, unique, total)
		case added == 0:
			return false, 0, fetched, fmt.Errorf("script list for organization group %s made no progress on page %d (page %d, size %d): the server may be ignoring the page parameter", orgGroupUUID, page, page, pageSize)
		}
	}
	return false, 0, maxPages, fmt.Errorf("script list for organization group %s exceeded %d pages; refusing to treat script %s as absent", orgGroupUUID, maxPages, scriptUUID)
}
