package assignment

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/euc-oss/terraform-sdk-uem/v26/client"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
)

const stateRmText = "`terraform state rm <addr>`"

// err5001000 mirrors what the pinned SDK returns once retries of an
// assignments call on a deleted script run out: an *sdk.APIError with HTTP
// 500 errorCode 1000, wrapped by the service method's name.
func err5001000() error {
	return fmt.Errorf("ScriptAssignmentV1_GetScriptAssignmentsAsync: %w", &sdk.APIError{
		StatusCode: http.StatusInternalServerError,
		ErrorCode:  "1000",
		Message:    "Internal Server Error",
		Attempts:   4,
	})
}

func other500s() map[string]error {
	return map[string]error{
		"errorCode 1001": &sdk.APIError{StatusCode: http.StatusInternalServerError, ErrorCode: "1001", Message: "Internal Server Error"},
		"no errorCode":   &sdk.APIError{StatusCode: http.StatusInternalServerError, Message: "Internal Server Error"},
	}
}

// listIDs answers the OG-scoped list with one page holding ids; later pages
// are empty with RecordCount 0, as the live server answers them.
func listIDs(ids ...string) func(string, *sdk.ScriptsV1GetScriptsByOrganizationGroupAsyncOptions) (*sdk.ScriptsSearchResultV1, error) {
	return func(_ string, opts *sdk.ScriptsV1GetScriptsByOrganizationGroupAsyncOptions) (*sdk.ScriptsSearchResultV1, error) {
		count := 0
		res := &sdk.ScriptsSearchResultV1{RecordCount: &count, SearchResults: []sdk.ScriptResourceLiteV1{}}
		if opts != nil && opts.Page != nil && *opts.Page == 0 {
			count = len(ids)
			for _, id := range ids {
				res.SearchResults = append(res.SearchResults, sdk.ScriptResourceLiteV1{ScriptUUID: id})
			}
		}
		return res, nil
	}
}

func listFails(string, *sdk.ScriptsV1GetScriptsByOrganizationGroupAsyncOptions) (*sdk.ScriptsSearchResultV1, error) {
	return nil, errors.New("list failed")
}

// requireOriginalError asserts state was kept and the diagnostic carries the
// original error text with prefix, with or without the state rm hint.
func requireOriginalError(t *testing.T, diags diag.Diagnostics, state tfsdk.State, prefix string, wantHint bool) {
	t.Helper()
	if !diags.HasError() {
		t.Fatal("expected an error diagnostic")
	}
	if state.Raw.IsNull() {
		t.Fatal("expected state to be kept")
	}
	d := diags.Errors()[0]
	if d.Summary() != "Client Error" || !strings.HasPrefix(d.Detail(), prefix) || !strings.Contains(d.Detail(), "Internal Server Error") {
		t.Fatalf("expected the original error, got %q / %q", d.Summary(), d.Detail())
	}
	if got := strings.Contains(d.Detail(), stateRmText); got != wantHint {
		t.Fatalf("state rm hint present=%v, want %v: %q", got, wantHint, d.Detail())
	}
}

const (
	readPrefix   = "Unable to read script assignments, got error: "
	deletePrefix = "Unable to delete script assignments, got error: "
)

func TestRead_5001000_ConfirmedAbsent_RemovesFromState(t *testing.T) {
	s := &fakeScriptService{listFn: listIDs("other-uuid")}
	resp := runRead(t, newTestResource(&fakeAssignmentService{getErr: err5001000()}, s), testOrgGroupUUID)
	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error, got: %v", resp.Diagnostics.Errors())
	}
	if !resp.State.Raw.IsNull() {
		t.Fatal("expected state to be removed after a confirmed-absent 500/1000")
	}
	// 2, not 1: resolveAmbiguousGone runs once for the initial ambiguous-gone
	// decision, then again as notfound.Confirm's classify on the confirming
	// re-GET (also 500/1000 here) before the drop actually happens — see
	// internal-task (internal/common/notfound). notfound.Delay is zeroed by
	// TestMain in crud_notfound_confirm_test.go so this stays fast.
	if s.listCalls != 2 {
		t.Fatalf("expected 2 list calls (initial decision + Confirm's re-verification), got %d", s.listCalls)
	}
}

func TestDelete_5001000_ConfirmedAbsent_Succeeds(t *testing.T) {
	s := &fakeScriptService{listFn: listIDs("other-uuid")}
	resp := runDelete(t, newTestResource(&fakeAssignmentService{bulkErr: err5001000()}, s), testOrgGroupUUID)
	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error, got: %v", resp.Diagnostics.Errors())
	}
	if s.listCalls != 1 {
		t.Fatalf("expected 1 list call, got %d", s.listCalls)
	}
}

func TestRead_5001000_StillListed_KeepsStateAndErrors(t *testing.T) {
	s := &fakeScriptService{listFn: listIDs("other-uuid", strings.ToUpper(testScriptUUID))}
	resp := runRead(t, newTestResource(&fakeAssignmentService{getErr: err5001000()}, s), testOrgGroupUUID)
	requireOriginalError(t, resp.Diagnostics, resp.State, readPrefix, false)
}

func TestDelete_5001000_StillListed_Errors(t *testing.T) {
	s := &fakeScriptService{listFn: listIDs(testScriptUUID)}
	resp := runDelete(t, newTestResource(&fakeAssignmentService{bulkErr: err5001000()}, s), testOrgGroupUUID)
	requireOriginalError(t, resp.Diagnostics, resp.State, deletePrefix, false)
}

func TestRead_5001000_ListError_KeepsStateAndErrors(t *testing.T) {
	s := &fakeScriptService{listFn: listFails}
	resp := runRead(t, newTestResource(&fakeAssignmentService{getErr: err5001000()}, s), testOrgGroupUUID)
	requireOriginalError(t, resp.Diagnostics, resp.State, readPrefix, false)
	if s.listCalls != 1 {
		t.Fatalf("expected 1 list call, got %d", s.listCalls)
	}
}

func TestDelete_5001000_ListError_Errors(t *testing.T) {
	s := &fakeScriptService{listFn: listFails}
	resp := runDelete(t, newTestResource(&fakeAssignmentService{bulkErr: err5001000()}, s), testOrgGroupUUID)
	requireOriginalError(t, resp.Diagnostics, resp.State, deletePrefix, false)
}

func TestRead_5001000_MissingRecordCount_KeepsStateAndErrors(t *testing.T) {
	s := &fakeScriptService{listFn: func(string, *sdk.ScriptsV1GetScriptsByOrganizationGroupAsyncOptions) (*sdk.ScriptsSearchResultV1, error) {
		return &sdk.ScriptsSearchResultV1{SearchResults: []sdk.ScriptResourceLiteV1{}}, nil
	}}
	resp := runRead(t, newTestResource(&fakeAssignmentService{getErr: err5001000()}, s), testOrgGroupUUID)
	requireOriginalError(t, resp.Diagnostics, resp.State, readPrefix, false)
}

func TestRead_5001000_NoOrgGroup_ErrorsWithStateRmHint(t *testing.T) {
	s := &fakeScriptService{listFn: listIDs()}
	resp := runRead(t, newTestResource(&fakeAssignmentService{getErr: err5001000()}, s), "")
	requireOriginalError(t, resp.Diagnostics, resp.State, readPrefix, true)
	if s.listCalls != 0 {
		t.Fatalf("expected no list call without an organization group, got %d", s.listCalls)
	}
}

func TestDelete_5001000_NoOrgGroup_ErrorsWithStateRmHint(t *testing.T) {
	s := &fakeScriptService{listFn: listIDs()}
	resp := runDelete(t, newTestResource(&fakeAssignmentService{bulkErr: err5001000()}, s), "")
	requireOriginalError(t, resp.Diagnostics, resp.State, deletePrefix, true)
	if s.listCalls != 0 {
		t.Fatalf("expected no list call without an organization group, got %d", s.listCalls)
	}
}

func TestRead_Other500_NeverLists(t *testing.T) {
	for name, getErr := range other500s() {
		t.Run(name, func(t *testing.T) {
			s := &fakeScriptService{listFn: listIDs()}
			resp := runRead(t, newTestResource(&fakeAssignmentService{getErr: getErr}, s), testOrgGroupUUID)
			requireOriginalError(t, resp.Diagnostics, resp.State, readPrefix, false)
			if s.listCalls != 0 {
				t.Fatalf("expected no list call, got %d", s.listCalls)
			}
		})
	}
}

func TestDelete_Other500_NeverLists(t *testing.T) {
	for name, bulkErr := range other500s() {
		t.Run(name, func(t *testing.T) {
			s := &fakeScriptService{listFn: listIDs()}
			resp := runDelete(t, newTestResource(&fakeAssignmentService{bulkErr: bulkErr}, s), testOrgGroupUUID)
			requireOriginalError(t, resp.Diagnostics, resp.State, deletePrefix, false)
			if s.listCalls != 0 {
				t.Fatalf("expected no list call, got %d", s.listCalls)
			}
		})
	}
}

func TestScriptAbsentFromOrgGroup_Paging(t *testing.T) {
	old := scriptListPageSize
	scriptListPageSize = 2
	t.Cleanup(func() { scriptListPageSize = old })
	ids := []string{"a", "b", "c"}
	paged := func(_ string, opts *sdk.ScriptsV1GetScriptsByOrganizationGroupAsyncOptions) (*sdk.ScriptsSearchResultV1, error) {
		count := len(ids)
		res := &sdk.ScriptsSearchResultV1{RecordCount: &count}
		for i := *opts.Page * 2; i < len(ids) && i < (*opts.Page+1)*2; i++ {
			res.SearchResults = append(res.SearchResults, sdk.ScriptResourceLiteV1{ScriptUUID: ids[i]})
		}
		if len(res.SearchResults) == 0 {
			count = 0 // live: a page past the end reports RecordCount 0
		}
		return res, nil
	}
	s := &fakeScriptService{listFn: paged}
	absent, err := scriptAbsentFromOrgGroup(t.Context(), s, testOrgGroupUUID, "c")
	if err != nil || absent {
		t.Fatalf("expected found on page 2, got absent=%v err=%v", absent, err)
	}
	absent, err = scriptAbsentFromOrgGroup(t.Context(), s, testOrgGroupUUID, "zzz")
	if err != nil || !absent {
		t.Fatalf("expected absent after all pages, got absent=%v err=%v", absent, err)
	}
}

// TestRead_5001000_PageIgnored_KeepsStateAndErrors: a server that ignores
// the page parameter answers every list call with the same full page. The
// repeated page adds no new script, so the lookup errors on the second call
// and state is kept, although 2+2 raw items reach RecordCount 4.
func TestRead_5001000_PageIgnored_KeepsStateAndErrors(t *testing.T) {
	old := scriptListPageSize
	scriptListPageSize = 2
	t.Cleanup(func() { scriptListPageSize = old })
	s := &fakeScriptService{listFn: func(string, *sdk.ScriptsV1GetScriptsByOrganizationGroupAsyncOptions) (*sdk.ScriptsSearchResultV1, error) {
		count := 4
		return &sdk.ScriptsSearchResultV1{RecordCount: &count, SearchResults: []sdk.ScriptResourceLiteV1{{ScriptUUID: "a"}, {ScriptUUID: "b"}}}, nil
	}}
	resp := runRead(t, newTestResource(&fakeAssignmentService{getErr: err5001000()}, s), testOrgGroupUUID)
	requireOriginalError(t, resp.Diagnostics, resp.State, readPrefix, false)
	if s.listCalls != 2 {
		t.Fatalf("expected 2 list calls, got %d", s.listCalls)
	}
}

// TestIsAmbiguousGoneAPIError_RealSDKExhaustedRetries drives the pinned SDK's
// assignments GET against a server that always answers 500 with errorCode
// 1000 (a JSON number, as on the wire) and checks the classifier matches the
// error it surfaces once retries run out, with Attempts set.
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
	svc := defaultScriptAssignmentServiceFactory(c)

	_, _, getErr := svc.GetScriptAssignmentsAsync(context.Background(), testScriptUUID)
	if !isAmbiguousGoneAPIError(getErr) {
		t.Fatalf("expected a 500/1000 match, got %v", getErr)
	}
	var apiErr *sdk.APIError
	if !errors.As(getErr, &apiErr) || apiErr.Attempts != 2 {
		t.Fatalf("expected Attempts 2 on the *sdk.APIError, got %v", getErr)
	}
}
