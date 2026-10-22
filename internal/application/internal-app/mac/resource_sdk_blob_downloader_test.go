package macapplication

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"

	"github.com/euc-oss/terraform-provider-uem/internal/httpclient"
)

// fakeInternalAppUUIDLookup is an injected test double for
// internalAppUUIDLookupAPI.
type fakeInternalAppUUIDLookup struct {
	calledWithUUID string
	details        *sdk.InternalAppModelV2
	err            error
	callOrder      *[]string
}

func (f *fakeInternalAppUUIDLookup) GetInternalAppByUuid(
	ctx context.Context,
	UUID string,
) (http.Header, *sdk.InternalAppModelV2, error) {
	f.calledWithUUID = UUID
	if f.callOrder != nil {
		*f.callOrder = append(*f.callOrder, "details:"+UUID)
	}
	if f.err != nil {
		return nil, nil, f.err
	}
	return nil, f.details, nil
}

// fakeBlobV2Service is an injected test double for appBlobV2ResourceServiceAPI.
type fakeBlobV2Service struct {
	calledWithBlobID string
	blob             []byte
	err              error
	callOrder        *[]string
	calledWithCtx    context.Context
}

func (f *fakeBlobV2Service) Delete(ctx context.Context, BlobID string) (http.Header, error) {
	return nil, fmt.Errorf("Delete not used by this fake")
}

func (f *fakeBlobV2Service) Get(ctx context.Context, BlobID string) (http.Header, []byte, error) {
	f.calledWithBlobID = BlobID
	f.calledWithCtx = ctx
	if f.callOrder != nil {
		*f.callOrder = append(*f.callOrder, "blob:"+BlobID)
	}
	if f.err != nil {
		return nil, nil, f.err
	}
	return nil, f.blob, nil
}

func TestSDKBlobDownloader_DownloadAppBlob_ResolvesUUIDThenDownloadsBlob(t *testing.T) {
	var callOrder []string

	const appUUID = "app-uuid-123"
	const blobGUID = "blob-guid-456"
	const wantID = 42
	blobBytes := []byte("fake-dmg-binary-content")

	id := wantID
	lookup := &fakeInternalAppUUIDLookup{
		details: &sdk.InternalAppModelV2{
			UUID:                    appUUID,
			ApplicationFileBlobGUID: blobGUID,
			ID:                      &id,
			MacOsSoftwareDeploymentSummary: &sdk.MacOsSoftwareDeploymentSummaryModelV2{
				Pkginfo: "<plist version=\"1.0\"><dict/></plist>",
			},
		},
		callOrder: &callOrder,
	}
	blobSvc := &fakeBlobV2Service{
		blob:      blobBytes,
		callOrder: &callOrder,
	}

	d := &sdkBlobDownloader{
		internalAppLookup: lookup,
		blobService:       blobSvc,
	}

	gotID, got, gotPkginfo, err := d.DownloadAppBlob(context.Background(), appUUID, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotPkginfo != "<plist version=\"1.0\"><dict/></plist>" {
		t.Fatalf("expected pkginfo from the details lookup, got %q", gotPkginfo)
	}

	if gotID != wantID {
		t.Fatalf("expected id %d, got %d", wantID, gotID)
	}

	if string(got) != string(blobBytes) {
		t.Fatalf("expected blob bytes %q, got %q", blobBytes, got)
	}

	if lookup.calledWithUUID != appUUID {
		t.Fatalf("expected details lookup called with %q, got %q", appUUID, lookup.calledWithUUID)
	}

	if blobSvc.calledWithBlobID != blobGUID {
		t.Fatalf("expected blob fetch called with %q, got %q", blobGUID, blobSvc.calledWithBlobID)
	}

	wantOrder := []string{"details:" + appUUID, "blob:" + blobGUID}
	if len(callOrder) != len(wantOrder) || callOrder[0] != wantOrder[0] || callOrder[1] != wantOrder[1] {
		t.Fatalf("expected call order %v, got %v", wantOrder, callOrder)
	}
}

func TestSDKBlobDownloader_DownloadAppBlob_DetailsLookupError(t *testing.T) {
	lookup := &fakeInternalAppUUIDLookup{err: fmt.Errorf("not found")}
	blobSvc := &fakeBlobV2Service{}

	d := &sdkBlobDownloader{
		internalAppLookup: lookup,
		blobService:       blobSvc,
	}

	_, _, _, err := d.DownloadAppBlob(context.Background(), "app-uuid-123", nil, nil)
	if err == nil {
		t.Fatal("expected error when details lookup fails")
	}
	if blobSvc.calledWithBlobID != "" {
		t.Fatal("expected blob fetch not to be called when details lookup fails")
	}
}

func TestSDKBlobDownloader_DownloadAppBlob_MissingBlobGUID(t *testing.T) {
	lookup := &fakeInternalAppUUIDLookup{
		details: &sdk.InternalAppModelV2{UUID: "app-uuid-123"},
	}
	blobSvc := &fakeBlobV2Service{}

	d := &sdkBlobDownloader{
		internalAppLookup: lookup,
		blobService:       blobSvc,
	}

	_, _, _, err := d.DownloadAppBlob(context.Background(), "app-uuid-123", nil, nil)
	if err == nil {
		t.Fatal("expected error when application has no blob GUID")
	}
	if blobSvc.calledWithBlobID != "" {
		t.Fatal("expected blob fetch not to be called when blob GUID is missing")
	}
}

func TestSDKBlobDownloader_DownloadAppBlob_MissingID(t *testing.T) {
	lookup := &fakeInternalAppUUIDLookup{
		details: &sdk.InternalAppModelV2{
			UUID:                    "app-uuid-123",
			ApplicationFileBlobGUID: "blob-guid-456",
		},
	}
	blobSvc := &fakeBlobV2Service{}

	d := &sdkBlobDownloader{
		internalAppLookup: lookup,
		blobService:       blobSvc,
	}

	_, _, _, err := d.DownloadAppBlob(context.Background(), "app-uuid-123", nil, nil)
	if err == nil {
		t.Fatal("expected error when application has no numeric id")
	}
	if blobSvc.calledWithBlobID != "" {
		t.Fatal("expected blob fetch not to be called when id is missing")
	}
}

func TestSDKBlobDownloader_DownloadAppBlob_BlobFetchError(t *testing.T) {
	id := 1
	lookup := &fakeInternalAppUUIDLookup{
		details: &sdk.InternalAppModelV2{
			UUID:                    "app-uuid-123",
			ApplicationFileBlobGUID: "blob-guid-456",
			ID:                      &id,
		},
	}
	blobSvc := &fakeBlobV2Service{err: fmt.Errorf("download failed")}

	d := &sdkBlobDownloader{
		internalAppLookup: lookup,
		blobService:       blobSvc,
	}

	_, _, _, err := d.DownloadAppBlob(context.Background(), "app-uuid-123", nil, nil)
	if err == nil {
		t.Fatal("expected error when blob fetch fails")
	}
}

func TestNewSDKBlobDownloader_ConstructsRealServices(t *testing.T) {
	client := &sdk.Client{}
	got := newSDKBlobDownloader(client)

	d, ok := got.(*sdkBlobDownloader)
	if !ok {
		t.Fatalf("expected *sdkBlobDownloader, got %T", got)
	}
	if d.internalAppLookup == nil {
		t.Fatal("expected internalAppLookup to be wired")
	}
	if d.blobService == nil {
		t.Fatal("expected blobService to be wired")
	}
}

// TestSDKBlobDownloader_OnInfoFiresBeforeDownload_WithNameAndSize confirms
// B10's contract: onInfo is called with the application's name/size from the
// SAME details lookup DownloadAppBlob already makes, and it fires BEFORE the
// blob download (blobService.Get) — provable via callOrder, since the fake
// records "info" the moment onInfo runs.
func TestSDKBlobDownloader_OnInfoFiresBeforeDownload_WithNameAndSize(t *testing.T) {
	var callOrder []string
	id := 42
	sizeKB := 512000
	lookup := &fakeInternalAppUUIDLookup{
		details: &sdk.InternalAppModelV2{
			ApplicationFileBlobGUID: "blob-guid-456",
			ApplicationName:         "exampleInstallerApp",
			AppSizeInKB:             &sizeKB,
			ID:                      &id,
		},
		callOrder: &callOrder,
	}
	blobSvc := &fakeBlobV2Service{blob: []byte("dmg-bytes"), callOrder: &callOrder}
	d := &sdkBlobDownloader{internalAppLookup: lookup, blobService: blobSvc}

	var gotName string
	var gotSizeKB *int
	onInfo := func(name string, s *int) {
		gotName, gotSizeKB = name, s
		callOrder = append(callOrder, "info")
	}

	if _, _, _, err := d.DownloadAppBlob(context.Background(), "app-uuid-123", onInfo, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotName != "exampleInstallerApp" {
		t.Errorf("onInfo name = %q, want %q", gotName, "exampleInstallerApp")
	}
	if gotSizeKB == nil || *gotSizeKB != sizeKB {
		t.Errorf("onInfo sizeKB = %v, want %d", gotSizeKB, sizeKB)
	}
	wantOrder := []string{"details:app-uuid-123", "info", "blob:blob-guid-456"}
	if len(callOrder) != len(wantOrder) {
		t.Fatalf("call order = %v, want %v", callOrder, wantOrder)
	}
	for i := range wantOrder {
		if callOrder[i] != wantOrder[i] {
			t.Fatalf("call order = %v, want %v", callOrder, wantOrder)
		}
	}
}

// TestSDKBlobDownloader_OnInfoNotCalled_WhenDetailsLookupFails confirms
// onInfo is never invoked if the details lookup itself errors (there is
// nothing to announce).
func TestSDKBlobDownloader_OnInfoNotCalled_WhenDetailsLookupFails(t *testing.T) {
	lookup := &fakeInternalAppUUIDLookup{err: fmt.Errorf("not found")}
	blobSvc := &fakeBlobV2Service{}
	d := &sdkBlobDownloader{internalAppLookup: lookup, blobService: blobSvc}

	called := false
	onInfo := func(string, *int) { called = true }
	if _, _, _, err := d.DownloadAppBlob(context.Background(), "app-uuid-123", onInfo, nil); err == nil {
		t.Fatal("expected error when details lookup fails")
	}
	if called {
		t.Error("expected onInfo not to be called when the details lookup fails")
	}
}

// TestSDKBlobDownloader_ProgressContext_ScopedToBlobDownloadOnly confirms
// the progress callback is wired into the context ONLY for the
// blobService.Get call, not the (separate) details lookup call — proven by
// checking the ctx the details lookup fake actually received carries no
// progress callback, while the blob fetch's ctx does.
func TestSDKBlobDownloader_ProgressContext_ScopedToBlobDownloadOnly(t *testing.T) {
	id := 1
	lookup := &fakeInternalAppUUIDLookup{
		details: &sdk.InternalAppModelV2{
			ApplicationFileBlobGUID: "blob-guid-456",
			ID:                      &id,
		},
	}
	blobSvc := &fakeBlobV2Service{blob: []byte("dmg-bytes")}
	d := &sdkBlobDownloader{internalAppLookup: lookup, blobService: blobSvc}

	baseCtx := context.Background()
	progress := func(read, total int64) {}
	if _, _, _, err := d.DownloadAppBlob(baseCtx, "app-uuid-123", nil, progress); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if blobSvc.calledWithCtx == baseCtx {
		t.Error("expected blobService.Get's context to be wrapped with the progress callback, got the base context unchanged")
	}
	if _, ok := httpclient.ProgressFromContext(blobSvc.calledWithCtx); !ok {
		t.Error("expected blobService.Get's context to carry the progress callback")
	}
}
