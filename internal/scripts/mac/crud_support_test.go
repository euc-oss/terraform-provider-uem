package macscript

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
)

type mockScriptService struct {
	createHeaders http.Header
	createErr     error

	fetchScript *sdk.ScriptResourceV1
	fetchErr    error

	updateErr error
	deleteErr error

	createOrgGroup string
	createReq      *sdk.CreateScriptV1

	fetchUUID string

	updateUUID string
	updateReq  *sdk.UpdateScriptV1

	deleteOrgGroup string
	deleteReq      *[]string

	// listFn, when set, answers GetScriptsByOrganizationGroupAsync.
	listFn    func(orgGroupUUID string, opts *sdk.ScriptsV1GetScriptsByOrganizationGroupAsyncOptions) (*sdk.ScriptsSearchResultV1, error)
	listCalls int
}

func (m *mockScriptService) GetScriptsByOrganizationGroupAsync(ctx context.Context, OrganizationGroupUUID string, opts *sdk.ScriptsV1GetScriptsByOrganizationGroupAsyncOptions) (http.Header, *sdk.ScriptsSearchResultV1, error) {
	m.listCalls++
	if m.listFn == nil {
		return http.Header{}, &sdk.ScriptsSearchResultV1{}, nil
	}
	res, err := m.listFn(OrganizationGroupUUID, opts)
	return http.Header{}, res, err
}

func (m *mockScriptService) CreateScriptAsync(ctx context.Context, OrganizationGroupUUID string, request *sdk.CreateScriptV1) (http.Header, error) {
	m.createOrgGroup = OrganizationGroupUUID
	m.createReq = request
	if m.createHeaders == nil {
		m.createHeaders = http.Header{}
	}
	return m.createHeaders, m.createErr
}

func (m *mockScriptService) GetScriptAsync(ctx context.Context, ScriptUUID string) (http.Header, *sdk.ScriptResourceV1, error) {
	m.fetchUUID = ScriptUUID
	return http.Header{}, m.fetchScript, m.fetchErr
}

func (m *mockScriptService) ScriptBulkDeleteAsync(ctx context.Context, OrganizationGroupUUID string, request *[]string) (http.Header, *sdk.DeleteScriptResourceV1, error) {
	m.deleteOrgGroup = OrganizationGroupUUID
	m.deleteReq = request
	return http.Header{}, &sdk.DeleteScriptResourceV1{}, m.deleteErr
}

func (m *mockScriptService) ReplaceScriptDefinitionAsync(ctx context.Context, ScriptUUID string, request *sdk.UpdateScriptV1) (http.Header, error) {
	m.updateUUID = ScriptUUID
	m.updateReq = request
	return http.Header{}, m.updateErr
}

func newResourceWithMockService(svc ScriptServiceV1API) *macscriptResource {
	return &macscriptResource{
		client: &sdk.Client{},
		newScriptServiceV1API: func(c *sdk.Client) ScriptServiceV1API {
			return svc
		},
	}
}

func TestCreateMacScript_Success(t *testing.T) {
	svc := &mockScriptService{createHeaders: http.Header{"Location": []string{"/API/mdm/groups/111/scripts/9af645a8-fef3-3e6d-3408-5cc69e0937d4"}}}
	r := newResourceWithMockService(svc)

	req := &sdk.CreateScriptV1{Name: "script"}
	got, err := r.createMacScript(context.Background(), "org-1", req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "9af645a8-fef3-3e6d-3408-5cc69e0937d4" {
		t.Fatalf("script UUID mismatch: got %q", got)
	}
	if svc.createOrgGroup != "org-1" || svc.createReq != req {
		t.Fatalf("create args mismatch: org=%q req=%p", svc.createOrgGroup, svc.createReq)
	}
}

func TestCreateMacScript_Errors(t *testing.T) {
	t.Run("create API fails", func(t *testing.T) {
		svc := &mockScriptService{createErr: errors.New("create failed")}
		r := newResourceWithMockService(svc)

		_, err := r.createMacScript(context.Background(), "org", &sdk.CreateScriptV1{})
		if err == nil || !strings.Contains(err.Error(), "unable to create mac script") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("location parse fails", func(t *testing.T) {
		svc := &mockScriptService{createHeaders: http.Header{"Location": []string{""}}}
		r := newResourceWithMockService(svc)

		_, err := r.createMacScript(context.Background(), "org", &sdk.CreateScriptV1{})
		if err == nil || !strings.Contains(err.Error(), "unable to parse script UUID") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestFetchMacscriptDetails(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		want := &sdk.ScriptResourceV1{ScriptUUID: "id-1", Name: "n"}
		svc := &mockScriptService{fetchScript: want}
		r := newResourceWithMockService(svc)

		got, err := r.fetchMacscriptDetails(context.Background(), "id-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != want {
			t.Fatalf("fetch response mismatch: got %p want %p", got, want)
		}
		if svc.fetchUUID != "id-1" {
			t.Fatalf("expected fetch UUID id-1, got %q", svc.fetchUUID)
		}
	})

	t.Run("api error", func(t *testing.T) {
		svc := &mockScriptService{fetchErr: errors.New("boom")}
		r := newResourceWithMockService(svc)

		_, err := r.fetchMacscriptDetails(context.Background(), "id-2")
		if err == nil || !strings.Contains(err.Error(), "unable to fetch mac script id-2") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestUpdateMacScript(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		svc := &mockScriptService{}
		r := newResourceWithMockService(svc)
		req := &sdk.UpdateScriptV1{Name: "u"}

		err := r.updateMacScript(context.Background(), "id-1", req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if svc.updateUUID != "id-1" || svc.updateReq != req {
			t.Fatalf("update args mismatch: uuid=%q req=%p", svc.updateUUID, svc.updateReq)
		}
	})

	t.Run("api error", func(t *testing.T) {
		svc := &mockScriptService{updateErr: errors.New("update failed")}
		r := newResourceWithMockService(svc)

		err := r.updateMacScript(context.Background(), "id-2", &sdk.UpdateScriptV1{})
		if err == nil || !strings.Contains(err.Error(), "unable to update mac script id-2") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestDeleteMacScript(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		svc := &mockScriptService{}
		r := newResourceWithMockService(svc)

		err := r.deleteMacScript(context.Background(), "org-1", "id-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if svc.deleteOrgGroup != "org-1" {
			t.Fatalf("delete org mismatch: got %q", svc.deleteOrgGroup)
		}
		if svc.deleteReq == nil || len(*svc.deleteReq) != 1 || (*svc.deleteReq)[0] != "id-1" {
			t.Fatalf("delete request mismatch: %#v", svc.deleteReq)
		}
	})

	t.Run("api error", func(t *testing.T) {
		svc := &mockScriptService{deleteErr: errors.New("delete failed")}
		r := newResourceWithMockService(svc)

		err := r.deleteMacScript(context.Background(), "org-2", "id-2")
		if err == nil || !strings.Contains(err.Error(), "unable to delete mac script id-2") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}
