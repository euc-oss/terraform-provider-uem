package sensors

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/euc-oss/terraform-sdk-uem/v26/client"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// createTestClient creates a *client.Client backed by the given httptest.Server, mirroring
// the pattern already used in internal/smartgroup/data_source_test.go for exercising a
// hand-written companion against the concrete (non-interface-seamed) SDK service.
func createTestClient(t *testing.T, handler http.HandlerFunc) (*client.Client, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)

	cfg := &client.Config{
		InstanceURL: server.URL,
		TenantCode:  "test-tenant",
		AuthMethod:  "basic",
		Username:    "test-user",
		Password:    "test-pass",
	}

	c, err := client.NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create test client: %v", err)
	}
	return c, server
}

// TestSensorSearchList_MapsOrganizationGroupUuid proves that sensorSearch.List (the
// hand-written companion in sensors_data_source_hooks.go) maps the SDK response item's
// OrganizationGroupUUID onto SensorSummary.OrganizationGroupUuid, mirroring how it already
// maps Uuid/Name.
func TestSensorSearchList_MapsOrganizationGroupUuid(t *testing.T) {
	t.Parallel()

	const wantOGUUID = "fc1bf13c-f814-d160-6b79-0bd6696dd98b"

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"result_set": []map[string]any{
				{
					"id":                      1,
					"name":                    "Battery Health",
					"uuid":                    "sensor-uuid-1",
					"organization_group_uuid": wantOGUUID,
				},
			},
			"total_results": 1,
		})
	}

	c, server := createTestClient(t, handler)
	defer server.Close()

	s := &sensorSearch{svc: sdk.NewDeviceSensorsV2Service(c)}

	out, err := s.List(context.Background(), sensorFilters{
		OrganizationGroupUuid: types.StringValue("og-uuid-123"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 sensor, got %d", len(out))
	}
	if out[0].OrganizationGroupUuid.ValueString() != wantOGUUID {
		t.Errorf("expected OrganizationGroupUuid %q, got %q", wantOGUUID, out[0].OrganizationGroupUuid.ValueString())
	}
	if out[0].Uuid.ValueString() != "sensor-uuid-1" {
		t.Errorf("expected Uuid %q, got %q", "sensor-uuid-1", out[0].Uuid.ValueString())
	}
}

// TestSensorSearchList_MapsPlatform proves sensorSearch.List maps each SDK list item's
// Platform onto SensorSummary.Platform, so consumers (e.g. ws1-tf onboard) can tell an
// APPLE_OSX sensor from a WIN_RT one.
func TestSensorSearchList_MapsPlatform(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"result_set": []map[string]any{
				{"id": 1, "name": "mac", "uuid": "u-mac", "platform": "APPLE_OSX"},
				{"id": 2, "name": "win", "uuid": "u-win", "platform": "WIN_RT"},
			},
			"total_results": 2,
		})
	}

	c, server := createTestClient(t, handler)
	defer server.Close()

	s := &sensorSearch{svc: sdk.NewDeviceSensorsV2Service(c)}

	out, err := s.List(context.Background(), sensorFilters{
		OrganizationGroupUuid: types.StringValue("og-uuid-123"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 sensors, got %d", len(out))
	}
	if got := out[0].Platform.ValueString(); got != "APPLE_OSX" {
		t.Errorf("sensor 0 platform = %q, want APPLE_OSX", got)
	}
	if got := out[1].Platform.ValueString(); got != "WIN_RT" {
		t.Errorf("sensor 1 platform = %q, want WIN_RT", got)
	}
}

// withShrunkSensorPaging shrinks sensorListPageSize/sensorListMaxPages for
// the duration of one test, restoring both via t.Cleanup, matching the
// pattern cli/internal/uemapi/orggroups_test.go already uses for
// OrgGroupPageSize/OrgGroupMaxPages. Callers of this helper must not call
// t.Parallel(): the mutation is only race-free because it starts and fully
// unwinds within this test's own serial execution window, before any
// t.Parallel()-marked test in this package/file enters its concurrent
// phase.
func withShrunkSensorPaging(t *testing.T, pageSize, maxPages int) {
	t.Helper()
	oldSize, oldMax := sensorListPageSize, sensorListMaxPages
	sensorListPageSize, sensorListMaxPages = pageSize, maxPages
	t.Cleanup(func() { sensorListPageSize, sensorListMaxPages = oldSize, oldMax })
}

// pagedSensorHandler serves canned JSON pages, keyed by 0-based page
// number, for GET requests against the sensor list endpoint, all sharing
// the same total_results value. RequestedPages, if non-nil, records every
// requested page number in call order (used to assert the walk fetched
// exactly the pages expected, and no more).
func pagedSensorHandler(pages map[int][]map[string]any, total int, requestedPages *[]int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if requestedPages != nil {
			*requestedPages = append(*requestedPages, page)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"result_set":    pages[page],
			"total_results": total,
		})
	}
}

// TestSensorSearchList_WalksMultiplePages proves the sensors-ds-paging fix
// (bugfix/sensors-ds-paging): before this change, List made exactly one
// call with no Page/PageSize set at all, so a 3rd sensor living on a 2nd
// page was silently dropped. With sensorListPageSize shrunk to 2, this
// forces a 2-page walk for 3 total sensors and asserts all 3 come back.
func TestSensorSearchList_WalksMultiplePages(t *testing.T) {
	withShrunkSensorPaging(t, 2, 50)

	var requestedPages []int
	pages := map[int][]map[string]any{
		0: {
			{"id": 1, "name": "s1", "uuid": "uuid-1", "platform": "APPLE_OSX"},
			{"id": 2, "name": "s2", "uuid": "uuid-2", "platform": "APPLE_OSX"},
		},
		1: {
			{"id": 3, "name": "s3", "uuid": "uuid-3", "platform": "APPLE_OSX"},
		},
	}
	c, server := createTestClient(t, pagedSensorHandler(pages, 3, &requestedPages))
	defer server.Close()

	s := &sensorSearch{svc: sdk.NewDeviceSensorsV2Service(c)}
	out, err := s.List(context.Background(), sensorFilters{OrganizationGroupUuid: types.StringValue("og-uuid-123")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 3 {
		t.Fatalf("expected 3 sensors across 2 pages, got %d: %+v", len(out), out)
	}
	want := map[string]bool{"uuid-1": true, "uuid-2": true, "uuid-3": true}
	for _, item := range out {
		if !want[item.Uuid.ValueString()] {
			t.Fatalf("unexpected sensor uuid %q in result", item.Uuid.ValueString())
		}
		delete(want, item.Uuid.ValueString())
	}
	if len(want) != 0 {
		t.Fatalf("missing expected sensor uuids: %v", want)
	}
	// 3 total pages fetched per walk (2 pages the real walk needs to see
	// unique==total on a short page 1, doubled because a >1-page walk is
	// repeated as a second, independent walk) -- proves the double-walk
	// defense actually ran, not just the first walk.
	if len(requestedPages) != 4 {
		t.Fatalf("expected 4 page requests (2-page walk x2, the double-walk defense), got %d: %v", len(requestedPages), requestedPages)
	}
}

// TestSensorSearchList_TotalChangedBetweenPages_Errors proves the
// mb1/scriptlookup "Total changed mid-walk" defense: if total_results
// disagrees between two non-empty pages, the walk must error rather than
// silently trust whichever value it saw last -- a shrinking or growing list
// mid-walk means the accumulated result may have skipped or duplicated an
// item, and List must refuse to return that as if it were complete.
func TestSensorSearchList_TotalChangedBetweenPages_Errors(t *testing.T) {
	withShrunkSensorPaging(t, 1, 50)

	handler := func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		w.Header().Set("Content-Type", "application/json")
		switch page {
		case 0:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result_set":    []map[string]any{{"id": 1, "name": "s1", "uuid": "uuid-1"}},
				"total_results": 2,
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result_set":    []map[string]any{{"id": 2, "name": "s2", "uuid": "uuid-2"}},
				"total_results": 3,
			})
		}
	}
	c, server := createTestClient(t, handler)
	defer server.Close()

	s := &sensorSearch{svc: sdk.NewDeviceSensorsV2Service(c)}
	_, err := s.List(context.Background(), sensorFilters{OrganizationGroupUuid: types.StringValue("og-uuid-123")})
	if err == nil {
		t.Fatal("expected an error when total_results changes mid-walk, got nil")
	}
}

// TestSensorSearchList_ServerIgnoresPageParam_Errors proves the
// no-progress defense: if the server returns the SAME page (ignoring the
// page query parameter) for every request, unique never reaches total and
// the walk must error, not loop forever or return a truncated/duplicated
// result.
func TestSensorSearchList_ServerIgnoresPageParam_Errors(t *testing.T) {
	withShrunkSensorPaging(t, 1, 10)

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"result_set":    []map[string]any{{"id": 1, "name": "s1", "uuid": "uuid-1"}},
			"total_results": 2,
		})
	}
	c, server := createTestClient(t, handler)
	defer server.Close()

	s := &sensorSearch{svc: sdk.NewDeviceSensorsV2Service(c)}
	_, err := s.List(context.Background(), sensorFilters{OrganizationGroupUuid: types.StringValue("og-uuid-123")})
	if err == nil {
		t.Fatal("expected an error when the server ignores the page parameter (no progress made), got nil")
	}
}

// TestSensorSearchList_PageCapExceeded_Errors proves the last-resort page
// cap: a server that always returns a full, non-repeating page and never
// reports a total_results consistent with a short/empty end must error once
// sensorListMaxPages is exceeded, never silently return a truncated list.
func TestSensorSearchList_PageCapExceeded_Errors(t *testing.T) {
	withShrunkSensorPaging(t, 1, 3)

	handler := func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"result_set":    []map[string]any{{"id": page + 1, "name": "s", "uuid": strconv.Itoa(page)}},
			"total_results": 1000,
		})
	}
	c, server := createTestClient(t, handler)
	defer server.Close()

	s := &sensorSearch{svc: sdk.NewDeviceSensorsV2Service(c)}
	_, err := s.List(context.Background(), sensorFilters{OrganizationGroupUuid: types.StringValue("og-uuid-123")})
	if err == nil {
		t.Fatal("expected an error once the page cap is exceeded, got nil")
	}
}

// TestSensorSearchList_MissingTotalResults_Errors proves total_results is
// required, not optional: a response with no total_results at all (the
// field's *int zero value is nil) must error rather than be silently
// treated as a page count of 0 -- an organization group's first page
// legitimately reports 0 for a genuinely empty result, so a MISSING field
// must be distinguishable from a genuine, present 0.
func TestSensorSearchList_MissingTotalResults_Errors(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"result_set": []map[string]any{{"id": 1, "name": "s1", "uuid": "uuid-1"}},
			// total_results deliberately omitted.
		})
	}
	c, server := createTestClient(t, handler)
	defer server.Close()

	s := &sensorSearch{svc: sdk.NewDeviceSensorsV2Service(c)}
	_, err := s.List(context.Background(), sensorFilters{OrganizationGroupUuid: types.StringValue("og-uuid-123")})
	if err == nil {
		t.Fatal("expected an error when total_results is missing from the response, got nil")
	}
}

// TestSensorSearchList_DoubleWalkDisagreement_Errors proves the
// second-independent-walk defense: a multi-page result is re-walked from
// page 0, and if the second walk yields a different set of sensor UUIDs
// than the first (same total, different members -- a delete+create that
// preserves the count but shifts which sensors are visible), List must
// error rather than return either walk's result as if it were trustworthy.
func TestSensorSearchList_DoubleWalkDisagreement_Errors(t *testing.T) {
	withShrunkSensorPaging(t, 1, 50)

	// pageSize=1 (see withShrunkSensorPaging above) means a non-empty page
	// (n=1) is never "short" (n < pageSize), so each walk needs a 3rd,
	// EMPTY page to confirm the end after unique reaches total on a full
	// page -- exactly the real "page past the end reports 0 items" shape
	// scriptlookup/mb1 already document. page 2+ within a walk is that
	// confirming empty page.
	var walk int64 // 0-based walk counter, incremented each time page 0 is re-requested
	handler := func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page == 0 {
			atomic.AddInt64(&walk, 1)
		}
		currentWalk := atomic.LoadInt64(&walk)
		w.Header().Set("Content-Type", "application/json")
		// Walk 1 sees uuid-1/uuid-2; walk 2 sees uuid-1/uuid-3 -- same
		// total (2) both times, but a different member on page 1.
		switch {
		case page == 0:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result_set":    []map[string]any{{"id": 1, "name": "s1", "uuid": "uuid-1"}},
				"total_results": 2,
			})
		case page == 1 && currentWalk == 1:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result_set":    []map[string]any{{"id": 2, "name": "s2", "uuid": "uuid-2"}},
				"total_results": 2,
			})
		case page == 1:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result_set":    []map[string]any{{"id": 3, "name": "s3", "uuid": "uuid-3"}},
				"total_results": 2,
			})
		default: // page >= 2: the confirming empty page every walk needs
			_ = json.NewEncoder(w).Encode(map[string]any{
				"result_set":    []map[string]any{},
				"total_results": 2,
			})
		}
	}
	c, server := createTestClient(t, handler)
	defer server.Close()

	s := &sensorSearch{svc: sdk.NewDeviceSensorsV2Service(c)}
	_, err := s.List(context.Background(), sensorFilters{OrganizationGroupUuid: types.StringValue("og-uuid-123")})
	if err == nil {
		t.Fatal("expected an error when the two independent walks disagree on which sensors are visible, got nil")
	}
}

// scriptedSensorPage is one canned page; scriptedSensorHandler serves them, per 0-based page, a fixed result_set and
// total_results pair, so a test can vary total_results page by page.
type scriptedSensorPage struct {
	items []map[string]any
	total int
}

func scriptedSensorHandler(pages map[int]scriptedSensorPage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		p := pages[page]
		if p.items == nil {
			p.items = []map[string]any{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"result_set": p.items, "total_results": p.total})
	}
}

func listScripted(t *testing.T, pages map[int]scriptedSensorPage) ([]SensorSummary, error) {
	t.Helper()
	c, server := createTestClient(t, scriptedSensorHandler(pages))
	t.Cleanup(server.Close)
	s := &sensorSearch{svc: sdk.NewDeviceSensorsV2Service(c)}
	return s.List(context.Background(), sensorFilters{OrganizationGroupUuid: types.StringValue("og-uuid-123")})
}

func sensorItem(u string) map[string]any { return map[string]any{"name": u, "uuid": u} }

// A full page that reaches total_results must be confirmed by the next
// page; an extra sensor on that next page (a stale total) is an error, not
// a list that silently omits it.
func TestSensorSearchList_ExtraItemAfterTotalReached_Errors(t *testing.T) {
	withShrunkSensorPaging(t, 2, 50)
	_, err := listScripted(t, map[int]scriptedSensorPage{
		0: {items: []map[string]any{sensorItem("a"), sensorItem("b")}, total: 2},
		1: {items: []map[string]any{sensorItem("c")}, total: 2},
	})
	if err == nil {
		t.Fatal("expected an error when a sensor appears after total_results was reached, got nil")
	}
}

// A trailing empty page reporting total_results 0 must not override the
// total from the non-empty page before it.
func TestSensorSearchList_TrailingEmptyPageTotalZero_Ignored(t *testing.T) {
	withShrunkSensorPaging(t, 2, 50)
	out, err := listScripted(t, map[int]scriptedSensorPage{
		0: {items: []map[string]any{sensorItem("a"), sensorItem("b")}, total: 2},
		1: {total: 0},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 sensors, got %d", len(out))
	}
}

// A total_results that shrinks to exactly the count already seen must
// still be an error, not accepted as a complete list.
func TestSensorSearchList_TotalShrinksToSeenCount_Errors(t *testing.T) {
	withShrunkSensorPaging(t, 2, 50)
	_, err := listScripted(t, map[int]scriptedSensorPage{
		0: {items: []map[string]any{sensorItem("a"), sensorItem("b")}, total: 4},
		1: {items: []map[string]any{sensorItem("c")}, total: 3},
	})
	if err == nil {
		t.Fatal("expected an error when total_results shrinks mid-walk, got nil")
	}
}

// A duplicate across pages is not progress: raw item counts reaching
// total_results must not end the walk while a distinct sensor is missing.
func TestSensorSearchList_CrossPageDuplicate_Errors(t *testing.T) {
	withShrunkSensorPaging(t, 2, 50)
	_, err := listScripted(t, map[int]scriptedSensorPage{
		0: {items: []map[string]any{sensorItem("a"), sensorItem("b")}, total: 3},
		1: {items: []map[string]any{sensorItem("B")}, total: 3},
	})
	if err == nil {
		t.Fatal("expected an error when a page only repeats an earlier sensor, got nil")
	}
}

// An organization group with no sensors reports total_results 0 on an
// empty first page; that is an empty list, not an error.
func TestSensorSearchList_EmptyFirstPage_ReturnsEmpty(t *testing.T) {
	out, err := listScripted(t, map[int]scriptedSensorPage{0: {total: 0}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == nil || len(out) != 0 {
		t.Fatalf("expected an empty non-nil list, got %#v", out)
	}
}
