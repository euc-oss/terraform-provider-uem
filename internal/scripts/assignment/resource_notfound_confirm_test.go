package assignment

import (
	"context"
	"net/http"
	"os"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"

	"github.com/euc-oss/terraform-provider-uem/internal/common/notfound"
)

// TestMain zeroes notfound.Delay for every test in this package so the
// wiring tests below (and the two-request confirming re-GET they drive
// through Read) run with no wall-clock wait. This also speeds up the
// pre-existing TestRead_5001000_ConfirmedAbsent_RemovesFromState in
// crud_absent_test.go, which now also flows through notfound.Confirm.
func TestMain(m *testing.M) {
	notfound.Delay = 0
	os.Exit(m.Run())
}

// counterAssignmentService answers GetScriptAssignmentsAsync with err on
// its first call and result on every subsequent call ("always" keeps
// answering err on every call), counting calls so tests can drive Read's
// confirming re-GET (internal-task, internal/common/notfound) deterministically.
type counterAssignmentService struct {
	getCalls int
	err      error
	result   *sdk.ScriptAssignmentsSearchResultV1
	always   bool
}

func (f *counterAssignmentService) AddScriptAssignmentAsync(context.Context, string, *sdk.CreateScriptAssignmentV1) (http.Header, error) {
	return nil, nil
}

func (f *counterAssignmentService) BulkUpdateScriptAssignmentsAsync(context.Context, string, *sdk.BulkUpdateScriptAssignmentV1) (http.Header, error) {
	return nil, nil
}

func (f *counterAssignmentService) GetScriptAssignmentAsync(context.Context, string) (http.Header, *sdk.ScriptAssignmentResourceV1, error) {
	return nil, nil, nil
}

func (f *counterAssignmentService) GetScriptAssignmentsAsync(context.Context, string) (http.Header, *sdk.ScriptAssignmentsSearchResultV1, error) {
	f.getCalls++
	if f.always || f.getCalls == 1 {
		return nil, nil, f.err
	}
	return nil, f.result, nil
}

func confirmTestAssignmentResult() *sdk.ScriptAssignmentsSearchResultV1 {
	return &sdk.ScriptAssignmentsSearchResultV1{}
}

// TestRead_Flaky404ThenFound_KeepsState is the fail-on-revert wiring test
// for internal-task, site 1 (plain 404 classification): Read's first
// GetScriptAssignmentsAsync answers a plain 404, exactly like a
// genuinely-deleted script assignment would. Without notfound.Confirm wired
// in, Read would drop state on that single response. With it wired in,
// Read waits (zeroed here) and re-GETs once; the confirming re-GET
// succeeds, so the assignment must stay in state and
// GetScriptAssignmentsAsync must have been called exactly twice.
func TestRead_Flaky404ThenFound_KeepsState(t *testing.T) {
	a := &counterAssignmentService{
		err:    &sdk.APIError{StatusCode: http.StatusNotFound, Message: "not found"},
		result: confirmTestAssignmentResult(),
	}
	s := &fakeScriptService{listFn: listIDs("other-uuid")}
	r := newTestResource2(a, s)

	resp := runRead(t, r, testOrgGroupUUID)

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error for a flaky 404 followed by a successful confirming re-GET, got: %v", resp.Diagnostics.Errors())
	}
	if resp.State.Raw.IsNull() {
		t.Fatal("expected state to be KEPT after a flaky 404 followed by a successful confirming re-GET")
	}
	if a.getCalls != 2 {
		t.Fatalf("expected exactly 2 GetScriptAssignmentsAsync calls (original + confirming re-GET), got %d", a.getCalls)
	}
	if s.listCalls != 0 {
		t.Fatalf("expected no OG-list call on the plain-404 site, got %d", s.listCalls)
	}
}

// TestRead_FlakyAmbiguous5001000ThenFound_KeepsState is the fail-on-revert
// wiring test for internal-task, site 2 (ambiguous 500/1000 confirmed gone by
// the parent script's OG-scoped list, via resolveAmbiguousGone): the first
// GetScriptAssignmentsAsync answers 500/1000, and the list scan confirms
// the parent script absent — exactly the shape that, before this fix,
// dropped state immediately. With notfound.Confirm wired in AFTER that
// decision, Read waits (zeroed here) and re-GETs once more; that confirming
// re-GET succeeds, so the assignment must stay in state. Because the
// confirming refetch succeeds, Confirm never re-invokes
// resolveAmbiguousGone's list scan, so exactly 1 list call is expected (the
// initial decision only).
func TestRead_FlakyAmbiguous5001000ThenFound_KeepsState(t *testing.T) {
	a := &counterAssignmentService{
		err:    err5001000(),
		result: confirmTestAssignmentResult(),
	}
	s := &fakeScriptService{listFn: listIDs("other-uuid")} // testScriptUUID absent from the OG list
	r := newTestResource2(a, s)

	resp := runRead(t, r, testOrgGroupUUID)

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error for a flaky ambiguous 500/1000 followed by a successful confirming re-GET, got: %v", resp.Diagnostics.Errors())
	}
	if resp.State.Raw.IsNull() {
		t.Fatal("expected state to be KEPT after a flaky ambiguous 500/1000 followed by a successful confirming re-GET")
	}
	if a.getCalls != 2 {
		t.Fatalf("expected exactly 2 GetScriptAssignmentsAsync calls (original + confirming re-GET), got %d", a.getCalls)
	}
	if s.listCalls != 1 {
		t.Fatalf("expected exactly 1 OG-list call (the initial ambiguous-gone decision only; the confirming re-GET succeeded so Confirm never re-invoked resolveAmbiguousGone), got %d", s.listCalls)
	}
}

// TestRead_ConfirmedAmbiguous5001000_RemovesState proves the "gone twice"
// path for site 2 still exercises notfound.Confirm (not around it): both
// the original and the confirming GetScriptAssignmentsAsync answer the
// ambiguous 500/1000 shape, and the list confirms the parent script absent
// both times, so the assignment must be dropped from state — see also the
// updated call-count assertions in
// TestRead_5001000_ConfirmedAbsent_RemovesFromState (crud_absent_test.go),
// which already covers this path with the fakeAssignmentService fake.
func TestRead_ConfirmedAmbiguous5001000_RemovesState(t *testing.T) {
	a := &counterAssignmentService{err: err5001000(), always: true}
	s := &fakeScriptService{listFn: listIDs("other-uuid")}
	r := newTestResource2(a, s)

	resp := runRead(t, r, testOrgGroupUUID)

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error dropping a confirmed-gone assignment, got: %v", resp.Diagnostics.Errors())
	}
	if !resp.State.Raw.IsNull() {
		t.Fatal("expected state to be REMOVED when both the original and confirming GetScriptAssignmentsAsync classify as ambiguous-gone")
	}
	if a.getCalls != 2 {
		t.Fatalf("expected exactly 2 GetScriptAssignmentsAsync calls (original + confirming re-GET), got %d", a.getCalls)
	}
	if s.listCalls != 2 {
		t.Fatalf("expected exactly 2 OG-list calls (initial decision + Confirm's re-verification), got %d", s.listCalls)
	}
}

// newTestResource2 mirrors newTestResource (fakes_test.go) but takes the
// counterAssignmentService fake defined in this file.
func newTestResource2(a scriptAssignmentServiceAPI, s *fakeScriptService) *scriptAssignmentResource {
	return &scriptAssignmentResource{
		client:                     &sdk.Client{},
		newScriptAssignmentService: func(*sdk.Client) scriptAssignmentServiceAPI { return a },
		newScriptService:           func(*sdk.Client) scriptServiceAPI { return s },
	}
}
