package scriptlookup

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
)

const testOG = "666ff6cc-aa5b-3c07-feaa-3a95d3a4bd2c"

type listFunc func(page, size int) (*sdk.ScriptsSearchResultV1, error)

type fakeLister struct {
	fn    listFunc
	calls int
	pages []int
}

func (f *fakeLister) GetScriptsByOrganizationGroupAsync(
	_ context.Context,
	_ string,
	opts *sdk.ScriptsV1GetScriptsByOrganizationGroupAsyncOptions,
) (http.Header, *sdk.ScriptsSearchResultV1, error) {
	f.calls++
	f.pages = append(f.pages, *opts.Page)
	res, err := f.fn(*opts.Page, *opts.PageSize)
	return nil, res, err
}

func result(total int, ids ...string) *sdk.ScriptsSearchResultV1 {
	res := &sdk.ScriptsSearchResultV1{RecordCount: &total, SearchResults: []sdk.ScriptResourceLiteV1{}}
	for _, id := range ids {
		res.SearchResults = append(res.SearchResults, sdk.ScriptResourceLiteV1{ScriptUUID: id})
	}
	return res
}

// fixedPages answers from fixed pages with a fixed RecordCount; any page past
// the end is empty with RecordCount 0, as the live server answers it.
func fixedPages(total int, pages ...[]string) listFunc {
	return func(page, _ int) (*sdk.ScriptsSearchResultV1, error) {
		if page < len(pages) {
			return result(total, pages[page]...), nil
		}
		return result(0), nil
	}
}

func run(t *testing.T, fn listFunc, target string, pageSize, maxPages int) (*fakeLister, bool, error) {
	t.Helper()
	f := &fakeLister{fn: fn}
	absent, err := AbsentFromOrgGroup(t.Context(), f, testOG, target, pageSize, maxPages)
	return f, absent, err
}

func requireErr(t *testing.T, absent bool, err error, want string) {
	t.Helper()
	if absent || err == nil {
		t.Fatalf("got absent=%v err=%v, want false/error", absent, err)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error %q does not contain %q", err, want)
	}
}

func requireResult(t *testing.T, absent bool, err error, want bool) {
	t.Helper()
	if err != nil || absent != want {
		t.Fatalf("got absent=%v err=%v, want %v/nil", absent, err, want)
	}
}

// TestAbsent_PageIgnored_NoProgressErrors is the gate's probe: RecordCount 4,
// page size 2, and every call returns the same [a, b]. The pre-fix code
// counted 2+2 raw items, reached 4, and returned absent=true although "d" was
// never seen.
func TestAbsent_PageIgnored_NoProgressErrors(t *testing.T) {
	same := func(int, int) (*sdk.ScriptsSearchResultV1, error) { return result(4, "a", "b"), nil }
	f, absent, err := run(t, same, "d", 2, DefaultMaxPages)
	requireErr(t, absent, err, "made no progress on page 1")
	if !strings.Contains(err.Error(), "ignoring the page parameter") {
		t.Fatalf("expected a page-ignored error, got %v", err)
	}
	if f.calls != 2 {
		t.Fatalf("expected 2 list calls, got %d", f.calls)
	}
}

// TestAbsent_PageIgnoredShortPage_NoProgressErrors repeats a short page: the
// raw item count would pass RecordCount, but unique progress stalls.
func TestAbsent_PageIgnoredShortPage_NoProgressErrors(t *testing.T) {
	same := func(int, int) (*sdk.ScriptsSearchResultV1, error) { return result(3, "a"), nil }
	_, absent, err := run(t, same, "d", 2, DefaultMaxPages)
	requireErr(t, absent, err, "made no progress")
}

// TestAbsent_DuplicateOverlap covers a list that shifts by one mid-walk:
// page 0 [a, b], page 1 [b, c] (one new), page 2 empty, RecordCount 3. The
// unique count reaches 3 on the full page 1, so page 2 is fetched and, being
// empty, confirms the end.
func TestAbsent_DuplicateOverlap(t *testing.T) {
	pages := fixedPages(3, []string{"a", "b"}, []string{"b", "c"})
	for _, target := range []string{"a", "b", "c", "C"} {
		_, absent, err := run(t, pages, target, 2, DefaultMaxPages)
		requireResult(t, absent, err, false)
	}
	f, absent, err := run(t, pages, "d", 2, DefaultMaxPages)
	requireResult(t, absent, err, true)
	if f.calls != 6 {
		t.Fatalf("expected 6 list calls (3 per walk, two walks), got %d", f.calls)
	}
}

// TestAbsent_DuplicateOverlapShortOfTotal: with RecordCount 4 the same shifted
// list leaves one script unseen, so the empty page 2 is an error, not absent.
func TestAbsent_DuplicateOverlapShortOfTotal(t *testing.T) {
	_, absent, err := run(t, fixedPages(4, []string{"a", "b"}, []string{"b", "c"}), "d", 2, DefaultMaxPages)
	requireErr(t, absent, err, "empty page 2 after 3 of 4 records")
}

// TestAbsent_ExactMultipleConfirmsWithTrailingPage: a total that is an exact
// multiple of the page size ends on a full page, so one more page is fetched.
func TestAbsent_ExactMultipleConfirmsWithTrailingPage(t *testing.T) {
	f, absent, err := run(t, fixedPages(4, []string{"a", "b"}, []string{"c", "d"}), "z", 2, DefaultMaxPages)
	requireResult(t, absent, err, true)
	if f.calls != 6 {
		t.Fatalf("expected 6 list calls (3 per walk, two walks), got %d", f.calls)
	}
}

// TestAbsent_FullPageAfterTotalErrors: once every counted script is seen, a
// further full page means the end was never signalled.
func TestAbsent_FullPageAfterTotalErrors(t *testing.T) {
	same := func(int, int) (*sdk.ScriptsSearchResultV1, error) { return result(2, "a", "b"), nil }
	_, absent, err := run(t, same, "z", 2, DefaultMaxPages)
	requireErr(t, absent, err, "full page 1 after all 2 records")
}

// TestAbsent_MoreDistinctThanRecordCountErrors: RecordCount undercounting the
// scripts the server returns is never trusted to end the walk.
func TestAbsent_MoreDistinctThanRecordCountErrors(t *testing.T) {
	_, absent, err := run(t, fixedPages(2, []string{"a", "b", "c"}), "z", 5, DefaultMaxPages)
	requireErr(t, absent, err, "3 distinct scripts by page 0 but RecordCount is 2")
}

// TestAbsent_PageCapExceeded: a server that advances pages and keeps
// returning new scripts under an inflated RecordCount hits the cap.
func TestAbsent_PageCapExceeded(t *testing.T) {
	endless := func(page, _ int) (*sdk.ScriptsSearchResultV1, error) {
		return result(1_000_000, fmt.Sprintf("p%d-a", page), fmt.Sprintf("p%d-b", page)), nil
	}
	f, absent, err := run(t, endless, "z", 2, 7)
	requireErr(t, absent, err, "exceeded 7 pages")
	if f.calls != 7 {
		t.Fatalf("expected 7 list calls, got %d", f.calls)
	}
}

func TestAbsent_DistinctPages(t *testing.T) {
	pages := fixedPages(5, []string{"a", "b"}, []string{"c", "d"}, []string{"e"})
	_, absent, err := run(t, pages, "E", 2, DefaultMaxPages)
	requireResult(t, absent, err, false)
	f, absent, err := run(t, pages, "z", 2, DefaultMaxPages)
	requireResult(t, absent, err, true)
	if f.calls != 6 {
		t.Fatalf("expected 6 list calls (3 per walk, two walks), got %d", f.calls)
	}
}

// TestAbsent_ServerCapsPageSize: the server returns 2 items although 5 were
// asked for; the short pages below RecordCount must not end the walk.
func TestAbsent_ServerCapsPageSize(t *testing.T) {
	ids := []string{"a", "b", "c", "d", "e"}
	capped := func(page, _ int) (*sdk.ScriptsSearchResultV1, error) {
		var out []string
		for i := page * 2; i < len(ids) && i < (page+1)*2; i++ {
			out = append(out, ids[i])
		}
		if len(out) == 0 {
			return result(0), nil
		}
		return result(len(ids), out...), nil
	}
	_, absent, err := run(t, capped, "e", 5, DefaultMaxPages)
	requireResult(t, absent, err, false)
	f, absent, err := run(t, capped, "z", 5, DefaultMaxPages)
	requireResult(t, absent, err, true)
	if f.calls != 6 {
		t.Fatalf("expected 6 list calls (3 per walk, two walks), got %d", f.calls)
	}
}

func TestAbsent_EdgeCases(t *testing.T) {
	t.Run("empty list with RecordCount 0 is absent", func(t *testing.T) {
		_, absent, err := run(t, fixedPages(0), "z", 2, DefaultMaxPages)
		requireResult(t, absent, err, true)
	})
	t.Run("list error", func(t *testing.T) {
		_, absent, err := run(t, func(int, int) (*sdk.ScriptsSearchResultV1, error) {
			return nil, errors.New("list failed")
		}, "z", 2, DefaultMaxPages)
		requireErr(t, absent, err, "list failed")
	})
	t.Run("missing RecordCount", func(t *testing.T) {
		_, absent, err := run(t, func(int, int) (*sdk.ScriptsSearchResultV1, error) {
			return &sdk.ScriptsSearchResultV1{}, nil
		}, "z", 2, DefaultMaxPages)
		requireErr(t, absent, err, "missing RecordCount")
	})
	t.Run("nil result", func(t *testing.T) {
		_, absent, err := run(t, func(int, int) (*sdk.ScriptsSearchResultV1, error) { return nil, nil }, "z", 2, DefaultMaxPages)
		requireErr(t, absent, err, "missing RecordCount")
	})
	t.Run("duplicate uuids differing in case count once", func(t *testing.T) {
		_, absent, err := run(t, fixedPages(2, []string{"a", "A"}), "z", 2, DefaultMaxPages)
		requireErr(t, absent, err, "empty page 1 after 1 of 2 records")
	})
	t.Run("pages start at 0 and advance by 1", func(t *testing.T) {
		f, _, _ := run(t, fixedPages(3, []string{"a", "b"}, []string{"c"}), "z", 2, DefaultMaxPages)
		// A multi-page absent is walked twice, each walk from page 0.
		if fmt.Sprint(f.pages) != "[0 1 0 1]" {
			t.Fatalf("got pages %v", f.pages)
		}
	})
}

// TestAbsent_LiveShapedTrailingPageRecordCountZero mirrors the live server:
// a page past the end is empty with RecordCount 0, not the grand total. A
// total that is an exact multiple of the page size ends on a full page, so
// that empty trailing page confirms the end and must yield absent.
func TestAbsent_LiveShapedTrailingPageRecordCountZero(t *testing.T) {
	live := func(page, _ int) (*sdk.ScriptsSearchResultV1, error) {
		switch page {
		case 0:
			return result(4, "a", "b"), nil
		case 1:
			return result(4, "c", "d"), nil
		default:
			return result(0), nil
		}
	}
	f, absent, err := run(t, live, "z", 2, DefaultMaxPages)
	requireResult(t, absent, err, true)
	if f.calls != 6 {
		t.Fatalf("expected 6 list calls (3 per walk, two walks), got %d", f.calls)
	}
	_, absent, err = run(t, live, "D", 2, DefaultMaxPages)
	requireResult(t, absent, err, false)
}

// TestAbsent_EmptyOrgGroupFirstPage: an organization group with no scripts
// answers page 0 empty with RecordCount 0, which is absent at once.
func TestAbsent_EmptyOrgGroupFirstPage(t *testing.T) {
	f, absent, err := run(t, func(int, int) (*sdk.ScriptsSearchResultV1, error) { return result(0), nil }, "z", 2, DefaultMaxPages)
	requireResult(t, absent, err, true)
	if f.calls != 1 {
		t.Fatalf("expected 1 list call, got %d", f.calls)
	}
}

// TestAbsent_EmptyFirstPageWithNonzeroCountErrors: an empty page 0 that still
// claims scripts exist is an early run-out, never absent.
func TestAbsent_EmptyFirstPageWithNonzeroCountErrors(t *testing.T) {
	_, absent, err := run(t, func(int, int) (*sdk.ScriptsSearchResultV1, error) { return result(3), nil }, "z", 2, DefaultMaxPages)
	requireErr(t, absent, err, "empty page 0 after 0 of 3 records")
}

// TestAbsent_RecordCountShrinksMidWalkErrors is the gate's shrink case: the
// list is [a, b, T, c], "a" is deleted after page 0, so page 1 shifts to [c]
// with RecordCount 3. The pre-fix code overwrote the total with 3, counted
// unique {a, b, c} == 3 and returned absent for T, which still exists.
func TestAbsent_RecordCountShrinksMidWalkErrors(t *testing.T) {
	shrink := func(page, _ int) (*sdk.ScriptsSearchResultV1, error) {
		switch page {
		case 0:
			return result(4, "a", "b"), nil
		case 1:
			return result(3, "c"), nil
		default:
			return result(0), nil
		}
	}
	f, absent, err := run(t, shrink, "T", 2, DefaultMaxPages)
	requireErr(t, absent, err, "RecordCount changed from 4 to 3 between page 0 and page 1")
	if f.calls != 2 {
		t.Fatalf("expected 2 list calls, got %d", f.calls)
	}
}

// TestAbsent_RecordCountChangesMidWalkErrors: RecordCount 5 on page 0 and 4
// on page 1, then an empty trailing page. The pre-fix code reached "absent".
func TestAbsent_RecordCountChangesMidWalkErrors(t *testing.T) {
	changed := func(page, _ int) (*sdk.ScriptsSearchResultV1, error) {
		switch page {
		case 0:
			return result(5, "a", "b"), nil
		case 1:
			return result(4, "c", "d"), nil
		default:
			return result(0), nil
		}
	}
	_, absent, err := run(t, changed, "z", 2, DefaultMaxPages)
	requireErr(t, absent, err, "RecordCount changed from 5 to 4 between page 0 and page 1")
}

// TestAbsent_SecondWalkFindsTargetErrors: the first walk is internally
// consistent and concludes absent over two pages; the second, independent
// walk sees the target. Either walk alone would be trusted, so the
// disagreement must be an error. This proves the second walk runs.
func TestAbsent_SecondWalkFindsTargetErrors(t *testing.T) {
	calls := 0
	shifting := func(page, _ int) (*sdk.ScriptsSearchResultV1, error) {
		calls++
		first := calls <= 2
		switch {
		case page == 0 && first:
			return result(3, "a", "b"), nil
		case page == 1 && first:
			return result(3, "c"), nil
		case page == 0:
			return result(3, "a", "T"), nil
		case page == 1:
			return result(3, "c"), nil
		default:
			return result(0), nil
		}
	}
	f, absent, err := run(t, shifting, "T", 2, DefaultMaxPages)
	requireErr(t, absent, err, "the second found it present")
	if f.calls != 3 {
		t.Fatalf("expected 3 list calls (2 + the second walk's page 0), got %d", f.calls)
	}
}

// TestAbsent_SecondWalkTotalDiffersErrors: both walks conclude absent, but
// with different totals, so the list changed between them.
func TestAbsent_SecondWalkTotalDiffersErrors(t *testing.T) {
	calls := 0
	shifting := func(page, _ int) (*sdk.ScriptsSearchResultV1, error) {
		calls++
		if calls <= 2 {
			return fixedPages(3, []string{"a", "b"}, []string{"c"})(page, 0)
		}
		return fixedPages(4, []string{"a", "b"}, []string{"c", "d"})(page, 0)
	}
	_, absent, err := run(t, shifting, "z", 2, DefaultMaxPages)
	requireErr(t, absent, err, "RecordCount 3 then 4")
}

// TestAbsent_SinglePageAbsentMakesOneCall: an organization group under the
// page size is answered by one page, so absent costs exactly one list call.
func TestAbsent_SinglePageAbsentMakesOneCall(t *testing.T) {
	f, absent, err := run(t, fixedPages(3, []string{"a", "b", "c"}), "z", DefaultPageSize, DefaultMaxPages)
	requireResult(t, absent, err, true)
	if f.calls != 1 {
		t.Fatalf("expected 1 list call, got %d", f.calls)
	}
}

// TestAbsent_MultiPageAbsentAgreesMakesTwoWalks: a stable multi-page list is
// walked twice, both walks agree, and the result is absent.
func TestAbsent_MultiPageAbsentAgreesMakesTwoWalks(t *testing.T) {
	f, absent, err := run(t, fixedPages(3, []string{"a", "b"}, []string{"c"}), "z", 2, DefaultMaxPages)
	requireResult(t, absent, err, true)
	if f.calls != 4 || fmt.Sprint(f.pages) != "[0 1 0 1]" {
		t.Fatalf("expected 4 list calls on pages [0 1 0 1], got %d on %v", f.calls, f.pages)
	}
}

// TestAbsent_NonPositivePageSizeErrors: a page size of 0 or less fails
// before any list call.
func TestAbsent_NonPositivePageSizeErrors(t *testing.T) {
	for _, size := range []int{0, -1} {
		f, absent, err := run(t, fixedPages(0), "z", size, DefaultMaxPages)
		requireErr(t, absent, err, fmt.Sprintf("invalid pageSize %d: must be positive", size))
		if f.calls != 0 {
			t.Fatalf("pageSize %d: expected 0 list calls, got %d", size, f.calls)
		}
	}
}
