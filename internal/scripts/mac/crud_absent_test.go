package macscript

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/euc-oss/terraform-sdk-uem/v26/client"
)

const absentTestOrgGroupUUID = "666ff6cc-aa5b-3c07-feaa-3a95d3a4bd2c"

// err5001000 mirrors what the pinned SDK returns when a GET or bulk delete
// of a deleted script keeps answering HTTP 500 errorCode 1000: after the
// retries run out it passes the final response through as an *sdk.APIError
// with Attempts set, wrapped by the service method's name. The wrap is kept
// so every Read/Delete test also proves the classifier unwraps it. See
// TestIsAmbiguousGoneAPIError_RealSDKExhaustedRetries for the real client.
func err5001000() error {
	return fmt.Errorf("ScriptsV1_GetScriptAsync: %w", &sdk.APIError{
		StatusCode: http.StatusInternalServerError,
		ErrorCode:  "1000",
		Message:    "Internal Server Error",
		Attempts:   4,
	})
}

// listPages answers the list call from fixed pages keyed by page index; any
// page past the end is empty with RecordCount 0, like the live out-of-range
// behaviour. RecordCount is otherwise the total across all pages.
func listPages(pages ...[]string) func(string, *sdk.ScriptsV1GetScriptsByOrganizationGroupAsyncOptions) (*sdk.ScriptsSearchResultV1, error) {
	total := 0
	for _, p := range pages {
		total += len(p)
	}
	return func(_ string, opts *sdk.ScriptsV1GetScriptsByOrganizationGroupAsyncOptions) (*sdk.ScriptsSearchResultV1, error) {
		page := 0
		if opts != nil && opts.Page != nil {
			page = *opts.Page
		}
		count := 0
		if page < len(pages) {
			count = total
		}
		res := &sdk.ScriptsSearchResultV1{RecordCount: &count, SearchResults: []sdk.ScriptResourceLiteV1{}}
		if page < len(pages) {
			for _, id := range pages[page] {
				res.SearchResults = append(res.SearchResults, sdk.ScriptResourceLiteV1{ScriptUUID: id})
			}
		}
		return res, nil
	}
}

// cappedList simulates a server that silently caps page_size at serverCap
// regardless of the requested size, over the given full id list.
func cappedList(serverCap int, ids []string) func(string, *sdk.ScriptsV1GetScriptsByOrganizationGroupAsyncOptions) (*sdk.ScriptsSearchResultV1, error) {
	return func(_ string, opts *sdk.ScriptsV1GetScriptsByOrganizationGroupAsyncOptions) (*sdk.ScriptsSearchResultV1, error) {
		count := len(ids)
		res := &sdk.ScriptsSearchResultV1{RecordCount: &count}
		for i := *opts.Page * serverCap; i < len(ids) && i < (*opts.Page+1)*serverCap; i++ {
			res.SearchResults = append(res.SearchResults, sdk.ScriptResourceLiteV1{ScriptUUID: ids[i]})
		}
		if len(res.SearchResults) == 0 {
			count = 0 // live: a page past the end reports RecordCount 0
		}
		return res, nil
	}
}

// fillerIDs returns n distinct uuids that never equal notFoundTestScriptUUID.
func fillerIDs(prefix string, n int) []string {
	ids := make([]string, n)
	for i := range ids {
		ids[i] = fmt.Sprintf("%s-%04d", prefix, i)
	}
	return ids
}

// withPageSize shrinks the list page size for a test and restores it.
func withPageSize(t *testing.T, n int) {
	t.Helper()
	old := scriptListPageSize
	scriptListPageSize = n
	t.Cleanup(func() { scriptListPageSize = old })
}

func seededState(t *testing.T, orgGroupUUID string) tfsdk.State {
	t.Helper()
	ctx := context.Background()
	state := emptyResourceState(t)
	diags := state.SetAttribute(ctx, path.Root("id"), notFoundTestScriptUUID)
	if orgGroupUUID != "" {
		diags.Append(state.SetAttribute(ctx, path.Root("organization_group_uuid"), orgGroupUUID)...)
	}
	if diags.HasError() {
		t.Fatalf("failed to seed state: %v", diags)
	}
	return state
}

func runRead(t *testing.T, svc *mockScriptService, orgGroupUUID string) *resource.ReadResponse {
	t.Helper()
	r := newResourceWithMockService(svc)
	state := seededState(t, orgGroupUUID)
	resp := &resource.ReadResponse{State: state}
	r.Read(context.Background(), resource.ReadRequest{State: state}, resp)
	return resp
}

func runDelete(t *testing.T, svc *mockScriptService) *resource.DeleteResponse {
	t.Helper()
	r := newResourceWithMockService(svc)
	state := seededState(t, absentTestOrgGroupUUID)
	resp := &resource.DeleteResponse{State: state}
	r.Delete(context.Background(), resource.DeleteRequest{State: state}, resp)
	return resp
}

// requireOriginalReadError asserts the Read kept state and surfaced the
// original fetch error text unchanged.
func requireOriginalReadError(t *testing.T, resp *resource.ReadResponse) {
	t.Helper()
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error diagnostic")
	}
	if resp.State.Raw.IsNull() {
		t.Fatal("expected state to be kept")
	}
	d := resp.Diagnostics.Errors()[0]
	if d.Summary() != "Client Error" || !strings.HasPrefix(d.Detail(), "Unable to fetch script details: ") ||
		!strings.Contains(d.Detail(), "Internal Server Error") {
		t.Fatalf("expected the original fetch error, got %q / %q", d.Summary(), d.Detail())
	}
}

func TestRead_5001000_ConfirmedAbsent_RemovesFromState(t *testing.T) {
	svc := &mockScriptService{fetchErr: err5001000(), listFn: listPages([]string{"other-uuid"})}
	resp := runRead(t, svc, absentTestOrgGroupUUID)

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error, got: %v", resp.Diagnostics.Errors())
	}
	if !resp.State.Raw.IsNull() {
		t.Fatal("expected state to be removed after a confirmed-absent 500/1000")
	}
	// 2, not 1: confirmScriptGone runs once for the initial ambiguous-gone
	// decision, then again as notfound.Confirm's classify on the confirming
	// re-GET (also 500/1000 here) before the drop actually happens — see
	// internal-task (internal/common/notfound). notfound.Delay is zeroed by
	// TestMain in crud_notfound_confirm_test.go so this stays fast.
	if svc.listCalls != 2 {
		t.Fatalf("expected 2 list calls (initial decision + Confirm's re-verification), got %d", svc.listCalls)
	}
}

func TestRead_5001000_StillListed_KeepsStateAndErrors(t *testing.T) {
	svc := &mockScriptService{fetchErr: err5001000(), listFn: listPages([]string{"other-uuid", notFoundTestScriptUUID})}
	requireOriginalReadError(t, runRead(t, svc, absentTestOrgGroupUUID))
}

func TestRead_5001000_ListError_KeepsStateAndErrors(t *testing.T) {
	svc := &mockScriptService{
		fetchErr: err5001000(),
		listFn: func(string, *sdk.ScriptsV1GetScriptsByOrganizationGroupAsyncOptions) (*sdk.ScriptsSearchResultV1, error) {
			return nil, errors.New("list failed")
		},
	}
	requireOriginalReadError(t, runRead(t, svc, absentTestOrgGroupUUID))
}

func TestRead_5001000_MissingRecordCount_KeepsStateAndErrors(t *testing.T) {
	svc := &mockScriptService{
		fetchErr: err5001000(),
		listFn: func(string, *sdk.ScriptsV1GetScriptsByOrganizationGroupAsyncOptions) (*sdk.ScriptsSearchResultV1, error) {
			return &sdk.ScriptsSearchResultV1{SearchResults: []sdk.ScriptResourceLiteV1{}}, nil
		},
	}
	requireOriginalReadError(t, runRead(t, svc, absentTestOrgGroupUUID))
}

func TestRead_5001000_InvalidStateOrgGroup_KeepsStateAndErrors(t *testing.T) {
	svc := &mockScriptService{fetchErr: err5001000(), listFn: listPages()}
	requireOriginalReadError(t, runRead(t, svc, ""))
	if svc.listCalls != 0 {
		t.Fatalf("expected no list call without a valid OG, got %d", svc.listCalls)
	}
}

func TestRead_Other500_NeverLists(t *testing.T) {
	cases := map[string]error{
		"errorCode 1001": &sdk.APIError{StatusCode: http.StatusInternalServerError, ErrorCode: "1001", Message: "Internal Server Error"},
		"no errorCode":   &sdk.APIError{StatusCode: http.StatusInternalServerError, Message: "Internal Server Error"},
	}
	for name, fetchErr := range cases {
		t.Run(name, func(t *testing.T) {
			svc := &mockScriptService{fetchErr: fetchErr, listFn: listPages()}
			requireOriginalReadError(t, runRead(t, svc, absentTestOrgGroupUUID))
			if svc.listCalls != 0 {
				t.Fatalf("expected no list call, got %d", svc.listCalls)
			}
		})
	}
}

func TestRead_404_RemovesWithoutListing(t *testing.T) {
	svc := &mockScriptService{fetchErr: &sdk.APIError{StatusCode: http.StatusNotFound, Message: "not found"}, listFn: listPages()}
	resp := runRead(t, svc, absentTestOrgGroupUUID)
	if resp.Diagnostics.HasError() || !resp.State.Raw.IsNull() {
		t.Fatalf("expected 404 to remove state without error, diags: %v", resp.Diagnostics)
	}
	if svc.listCalls != 0 {
		t.Fatalf("expected no list call on 404, got %d", svc.listCalls)
	}
}

func TestRead_5001000_FoundOnSecondPage_KeepsStateAndErrors(t *testing.T) {
	withPageSize(t, 3)
	svc := &mockScriptService{
		fetchErr: err5001000(),
		listFn:   listPages(fillerIDs("p0", 3), []string{"x", notFoundTestScriptUUID}),
	}
	requireOriginalReadError(t, runRead(t, svc, absentTestOrgGroupUUID))
	if svc.listCalls != 2 {
		t.Fatalf("expected 2 list calls, got %d", svc.listCalls)
	}
}

func TestRead_5001000_AbsentAcrossTwoPages_RemovesFromState(t *testing.T) {
	withPageSize(t, 3)
	svc := &mockScriptService{
		fetchErr: err5001000(),
		listFn:   listPages(fillerIDs("p0", 3), fillerIDs("p1", 3)),
	}
	resp := runRead(t, svc, absentTestOrgGroupUUID)
	if resp.Diagnostics.HasError() || !resp.State.Raw.IsNull() {
		t.Fatalf("expected removal, diags: %v", resp.Diagnostics)
	}
	// RecordCount (6) is reached on the full second page, so a third
	// (empty) page is fetched to confirm the end before declaring absent.
	// A multi-page absent is confirmed by a second walk: 3 calls each, for
	// 6 total per confirmScriptGone call. Read now runs confirmScriptGone
	// twice (initial decision + notfound.Confirm's re-verification, internal-task), so 12 total.
	if svc.listCalls != 12 {
		t.Fatalf("expected 12 list calls, got %d", svc.listCalls)
	}
}

func TestRead_5001000_PageIgnored_KeepsStateAndErrors(t *testing.T) {
	withPageSize(t, 2)
	// A server that ignores the page parameter returns the same full page
	// forever. The second, repeated page adds no new uuid, so the lookup
	// fails there; the page cap is not what stops it.
	svc := &mockScriptService{
		fetchErr: err5001000(),
		listFn: func(string, *sdk.ScriptsV1GetScriptsByOrganizationGroupAsyncOptions) (*sdk.ScriptsSearchResultV1, error) {
			count := 1000
			return &sdk.ScriptsSearchResultV1{RecordCount: &count, SearchResults: []sdk.ScriptResourceLiteV1{{ScriptUUID: "a"}, {ScriptUUID: "b"}}}, nil
		},
	}
	requireOriginalReadError(t, runRead(t, svc, absentTestOrgGroupUUID))
	if svc.listCalls != 2 {
		t.Fatalf("expected 2 list calls, got %d", svc.listCalls)
	}
}

func TestDelete_5001000_ConfirmedAbsent_Succeeds(t *testing.T) {
	svc := &mockScriptService{deleteErr: err5001000(), listFn: listPages()}
	resp := runDelete(t, svc)
	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error, got: %v", resp.Diagnostics.Errors())
	}
	if svc.listCalls != 1 {
		t.Fatalf("expected 1 list call, got %d", svc.listCalls)
	}
}

func TestDelete_5001000_StillListed_Errors(t *testing.T) {
	svc := &mockScriptService{deleteErr: err5001000(), listFn: listPages([]string{notFoundTestScriptUUID})}
	resp := runDelete(t, svc)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error when the script is still listed")
	}
	d := resp.Diagnostics.Errors()[0]
	if d.Summary() != "Client Error" || !strings.HasPrefix(d.Detail(), "Unable to delete macOS script, got error: ") ||
		!strings.Contains(d.Detail(), "Internal Server Error") {
		t.Fatalf("expected the original delete error, got %q / %q", d.Summary(), d.Detail())
	}
}

func TestDelete_5001000_ListError_Errors(t *testing.T) {
	svc := &mockScriptService{
		deleteErr: err5001000(),
		listFn: func(string, *sdk.ScriptsV1GetScriptsByOrganizationGroupAsyncOptions) (*sdk.ScriptsSearchResultV1, error) {
			return nil, errors.New("list failed")
		},
	}
	if !runDelete(t, svc).Diagnostics.HasError() {
		t.Fatal("expected an error when the confirming lookup fails")
	}
}

func TestDelete_Other500_ErrorsWithoutListing(t *testing.T) {
	svc := &mockScriptService{
		deleteErr: &sdk.APIError{StatusCode: http.StatusInternalServerError, ErrorCode: "1001", Message: "Internal Server Error"},
		listFn:    listPages(),
	}
	if !runDelete(t, svc).Diagnostics.HasError() {
		t.Fatal("expected an error for a non-1000 500")
	}
	if svc.listCalls != 0 {
		t.Fatalf("expected no list call, got %d", svc.listCalls)
	}
}

func TestDelete_404_SucceedsWithoutListing(t *testing.T) {
	svc := &mockScriptService{deleteErr: &sdk.APIError{StatusCode: http.StatusNotFound, Message: "not found"}, listFn: listPages()}
	if resp := runDelete(t, svc); resp.Diagnostics.HasError() {
		t.Fatalf("expected no error on 404, got: %v", resp.Diagnostics.Errors())
	}
	if svc.listCalls != 0 {
		t.Fatalf("expected no list call on 404, got %d", svc.listCalls)
	}
}

func TestScriptAbsentFromOrgGroup(t *testing.T) {
	ctx := context.Background()

	t.Run("absent on empty list", func(t *testing.T) {
		svc := &mockScriptService{listFn: listPages()}
		absent, err := scriptAbsentFromOrgGroup(ctx, svc, absentTestOrgGroupUUID, notFoundTestScriptUUID)
		if err != nil || !absent {
			t.Fatalf("got absent=%v err=%v, want true/nil", absent, err)
		}
	})

	t.Run("found is not absent", func(t *testing.T) {
		svc := &mockScriptService{listFn: listPages([]string{notFoundTestScriptUUID})}
		absent, err := scriptAbsentFromOrgGroup(ctx, svc, absentTestOrgGroupUUID, notFoundTestScriptUUID)
		if err != nil || absent {
			t.Fatalf("got absent=%v err=%v, want false/nil", absent, err)
		}
	})

	t.Run("uuid comparison is case-insensitive", func(t *testing.T) {
		svc := &mockScriptService{listFn: listPages([]string{strings.ToUpper(notFoundTestScriptUUID)})}
		absent, err := scriptAbsentFromOrgGroup(ctx, svc, absentTestOrgGroupUUID, notFoundTestScriptUUID)
		if err != nil || absent {
			t.Fatalf("got absent=%v err=%v, want false/nil", absent, err)
		}
	})

	t.Run("list error is not absent", func(t *testing.T) {
		svc := &mockScriptService{listFn: func(string, *sdk.ScriptsV1GetScriptsByOrganizationGroupAsyncOptions) (*sdk.ScriptsSearchResultV1, error) {
			return nil, errors.New("list failed")
		}}
		absent, err := scriptAbsentFromOrgGroup(ctx, svc, absentTestOrgGroupUUID, notFoundTestScriptUUID)
		if err == nil || absent {
			t.Fatalf("got absent=%v err=%v, want false/error", absent, err)
		}
	})

	t.Run("server-capped page size still finds uuid on last page", func(t *testing.T) {
		withPageSize(t, 5)
		ids := append(fillerIDs("c", 4), notFoundTestScriptUUID)
		svc := &mockScriptService{listFn: cappedList(2, ids)}
		absent, err := scriptAbsentFromOrgGroup(ctx, svc, absentTestOrgGroupUUID, notFoundTestScriptUUID)
		if err != nil || absent {
			t.Fatalf("got absent=%v err=%v, want false/nil", absent, err)
		}
		if svc.listCalls != 3 {
			t.Fatalf("expected 3 list calls, got %d", svc.listCalls)
		}
	})

	t.Run("server-capped page size absent after all pages", func(t *testing.T) {
		withPageSize(t, 5)
		svc := &mockScriptService{listFn: cappedList(2, fillerIDs("c", 5))}
		absent, err := scriptAbsentFromOrgGroup(ctx, svc, absentTestOrgGroupUUID, notFoundTestScriptUUID)
		if err != nil || !absent {
			t.Fatalf("got absent=%v err=%v, want true/nil", absent, err)
		}
		// Two walks of 3 pages each confirm a multi-page absent.
		if svc.listCalls != 6 {
			t.Fatalf("expected 6 list calls, got %d", svc.listCalls)
		}
	})

	t.Run("missing RecordCount is an error", func(t *testing.T) {
		svc := &mockScriptService{listFn: func(string, *sdk.ScriptsV1GetScriptsByOrganizationGroupAsyncOptions) (*sdk.ScriptsSearchResultV1, error) {
			return &sdk.ScriptsSearchResultV1{SearchResults: []sdk.ScriptResourceLiteV1{{ScriptUUID: "other"}}}, nil
		}}
		absent, err := scriptAbsentFromOrgGroup(ctx, svc, absentTestOrgGroupUUID, notFoundTestScriptUUID)
		if err == nil || absent {
			t.Fatalf("got absent=%v err=%v, want false/error", absent, err)
		}
	})

	t.Run("empty page before RecordCount is reached is an error", func(t *testing.T) {
		svc := &mockScriptService{listFn: func(_ string, opts *sdk.ScriptsV1GetScriptsByOrganizationGroupAsyncOptions) (*sdk.ScriptsSearchResultV1, error) {
			count := 10
			res := &sdk.ScriptsSearchResultV1{RecordCount: &count}
			if *opts.Page == 0 {
				res.SearchResults = []sdk.ScriptResourceLiteV1{{ScriptUUID: "other"}}
			}
			return res, nil
		}}
		absent, err := scriptAbsentFromOrgGroup(ctx, svc, absentTestOrgGroupUUID, notFoundTestScriptUUID)
		if err == nil || absent {
			t.Fatalf("got absent=%v err=%v, want false/error", absent, err)
		}
	})

	t.Run("empty OG list with RecordCount 0 is absent", func(t *testing.T) {
		svc := &mockScriptService{listFn: func(string, *sdk.ScriptsV1GetScriptsByOrganizationGroupAsyncOptions) (*sdk.ScriptsSearchResultV1, error) {
			count := 0
			return &sdk.ScriptsSearchResultV1{RecordCount: &count, SearchResults: []sdk.ScriptResourceLiteV1{}}, nil
		}}
		absent, err := scriptAbsentFromOrgGroup(ctx, svc, absentTestOrgGroupUUID, notFoundTestScriptUUID)
		if err != nil || !absent {
			t.Fatalf("got absent=%v err=%v, want true/nil", absent, err)
		}
	})

	t.Run("sends OG, page from 0 and page size", func(t *testing.T) {
		var gotOG string
		var gotPage, gotSize int
		svc := &mockScriptService{listFn: func(og string, opts *sdk.ScriptsV1GetScriptsByOrganizationGroupAsyncOptions) (*sdk.ScriptsSearchResultV1, error) {
			gotOG, gotPage, gotSize = og, *opts.Page, *opts.PageSize
			count := 0
			return &sdk.ScriptsSearchResultV1{RecordCount: &count}, nil
		}}
		if _, err := scriptAbsentFromOrgGroup(ctx, svc, absentTestOrgGroupUUID, notFoundTestScriptUUID); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotOG != absentTestOrgGroupUUID || gotPage != 0 || gotSize != scriptListPageSize {
			t.Fatalf("got og=%q page=%d size=%d", gotOG, gotPage, gotSize)
		}
	})
}

func TestIsAmbiguousGoneAPIError(t *testing.T) {
	if !isAmbiguousGoneAPIError(err5001000()) {
		t.Fatal("expected 500/1000 to match")
	}
	for _, err := range []error{
		&sdk.APIError{StatusCode: http.StatusInternalServerError, ErrorCode: "1001"},
		&sdk.APIError{StatusCode: http.StatusInternalServerError},
		&sdk.APIError{StatusCode: http.StatusBadRequest, ErrorCode: "1000"},
		errors.New("boom"),
	} {
		if isAmbiguousGoneAPIError(err) {
			t.Fatalf("expected no match for %v", err)
		}
	}
}

// TestIsAmbiguousGoneAPIError_RealSDKExhaustedRetries drives the pinned SDK
// against a server that always answers 500 with errorCode 1000 (a JSON
// number, as on the wire) and checks that the GET and the bulk delete both
// surface an error the classifier matches, with Attempts set.
func TestIsAmbiguousGoneAPIError_RealSDKExhaustedRetries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"errorCode":1000,"message":"Internal Server Error"}`))
	}))
	t.Cleanup(server.Close)

	c, err := client.NewClient(&client.Config{
		InstanceURL: server.URL,
		TenantCode:  "test-tenant",
		AuthMethod:  "basic",
		Username:    "test-user",
		Password:    "test-pass",
		MaxRetries:  1,
	})
	if err != nil {
		t.Fatalf("failed to create test client: %v", err)
	}
	svc := defaulScriptServiceV1Factory(c)

	_, _, getErr := svc.GetScriptAsync(context.Background(), notFoundTestScriptUUID)
	_, _, delErr := svc.ScriptBulkDeleteAsync(context.Background(), absentTestOrgGroupUUID, &[]string{notFoundTestScriptUUID})
	for name, err := range map[string]error{"get": getErr, "bulk delete": delErr} {
		if !isAmbiguousGoneAPIError(err) {
			t.Fatalf("%s: expected a 500/1000 match, got %v", name, err)
		}
		var apiErr *sdk.APIError
		if !errors.As(err, &apiErr) || apiErr.Attempts != 2 {
			t.Fatalf("%s: expected Attempts 2 on the *sdk.APIError, got %v", name, err)
		}
	}
}
