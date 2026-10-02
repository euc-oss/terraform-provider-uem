package macapplication

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	tf "github.com/euc-oss/terraform-provider-uem/internal/application/internal-app/mac/models"
	appState "github.com/euc-oss/terraform-provider-uem/internal/application/internal-app/mac/state"
	commonerrors "github.com/euc-oss/terraform-provider-uem/internal/common/errors"
	"github.com/euc-oss/terraform-provider-uem/internal/common/notfound"
	"github.com/euc-oss/terraform-provider-uem/internal/httpclient"
	"github.com/euc-oss/terraform-provider-uem/internal/providerdata"
)

// isAmbiguousNotFoundOrInaccessibleAPIError reports whether err is the
// shape UEM 26.2 answers a GET for a deleted (or never-existing) internal
// app id with: HTTP 401, errorCode "7000", message "Application not found
// or user does not have access to it." Live-confirmed 2026-09-23 on as<internal-env>
// 26.2, and confirmed AMBIGUOUS by a live control call: an existing app
// fetched with the same credentials returns 200, so the credentials are
// valid — this 401/7000 genuinely means "this id is gone or you can't see
// it", not "your auth is broken". Evidence:
// internal-design-doc; commit
// 4dc4e3ef7f. Because it's ambiguous, callers must not treat
// it as a plain not-found: Read both removes the resource from state AND
// surfaces a warning telling the user to verify access before applying a
// plan that would recreate it (a real access problem, misclassified as a
// deletion, could otherwise cause a duplicate app to be created). A 401
// WITHOUT errorCode 7000 (e.g. a genuinely expired token) must not match.
func isAmbiguousNotFoundOrInaccessibleAPIError(err error) bool {
	return commonerrors.IsAPIErrorMatch(err, http.StatusUnauthorized, "7000", "")
}

func (r *macapplicationResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	switch data := req.ProviderData.(type) {
	case *resourceConfigData:
		if data == nil || data.client == nil {
			resp.Diagnostics.AddError("Unexpected Resource Configure Type", "Provider resource data was nil or missing a configured SDK client.")
			return
		}
		r.client = data.client
		if data.newBlobV1ResourceService != nil {
			r.newBlobV1ResourceService = data.newBlobV1ResourceService
		} else {
			r.newBlobV1ResourceService = defaultBlobV1ResourceServiceFactory
		}

		if data.newBlobV2ResourceService != nil {
			r.newBlobV2ResourceService = data.newBlobV2ResourceService
		} else {
			r.newBlobV2ResourceService = defaultBlobV2ResourceServiceFactory
		}

		if data.newMacAppResourceService != nil {
			r.newMacAppResourceService = data.newMacAppResourceService
		} else {
			r.newMacAppResourceService = defaultMacAppResourceServiceFactory
		}

		if data.newInternalAppResourceService != nil {
			r.newInternalAppResourceService = data.newInternalAppResourceService
		} else {
			r.newInternalAppResourceService = defaultInternalAppResourceServiceFactory
		}
	case *providerdata.ProviderData:
		if data == nil || data.Client == nil {
			resp.Diagnostics.AddError("Unexpected Resource Configure Type", "Provider resource data was nil or missing a configured SDK client.")
			return
		}
		r.client = data.Client
		r.newBlobV1ResourceService = defaultBlobV1ResourceServiceFactory
		r.newBlobV2ResourceService = defaultBlobV2ResourceServiceFactory
		r.newMacAppResourceService = defaultMacAppResourceServiceFactory
		r.newInternalAppResourceService = defaultInternalAppResourceServiceFactory
		r.appBinaryStoragePath = data.AppBinaryStoragePath
		r.blobDownloader = newSDKBlobDownloader(data.Client)
	case *sdk.Client:
		// Backward-compatible path for direct unit tests that still pass *sdk.Client.
		r.client = data
		r.newBlobV1ResourceService = defaultBlobV1ResourceServiceFactory
		r.newBlobV2ResourceService = defaultBlobV2ResourceServiceFactory
		r.newMacAppResourceService = defaultMacAppResourceServiceFactory
		r.newInternalAppResourceService = defaultInternalAppResourceServiceFactory
	default:
		resp.Diagnostics.AddError("Unexpected Resource Configure Type", resourceTypeErrorDetail(req.ProviderData))
		return
	}

	tflog.Trace(ctx, "configure a macOS application resource")
}

func (r *macapplicationResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse) {

	var data tf.MacApplicationResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	orgGroupID, err := data.FetchValidOrgGroupID()
	if err != nil {
		resp.Diagnostics.AddError("Invalid org_group_id", err.Error())
		return
	}

	dmgFilePath, err := data.FetchValidDMGFilePath()
	if err != nil {
		resp.Diagnostics.AddError("Invalid dmg_file_path", err.Error())
		return
	}

	plistFilePath, err := data.FetchValidPlistFilePath()
	if err != nil {
		resp.Diagnostics.AddError("Invalid plist_file_path", err.Error())
		return
	}

	// app_version is optional (B16 decision table row #99, CORRECT):
	// canonical Q9 confirms UEM's Version field on
	// MacOsCreateApplicationRequestV1Model is optional server-side, with no
	// FluentValidation rule requiring it, so an omitted/blank app_version
	// must not fail Create.
	appversion := data.FetchOptionalAppVersion()

	appUploadResponse, dmgBytes, err := r.uploadBlob(ctx, dmgFilePath, orgGroupID, true)
	if err != nil {
		resp.Diagnostics.AddError("Blob Upload Error", fmt.Sprintf("Failed to upload DMG binary: %s", err))
		return
	}

	plistUploadResponse, plistBytes, err := r.uploadBlob(ctx, plistFilePath, orgGroupID, false)
	if err != nil {
		r.cleanupBlobs(ctx, *appUploadResponse.Value)
		resp.Diagnostics.AddError("Blob Upload Error", fmt.Sprintf("Failed to upload plist file: %s", err))
		return
	}

	// The icon is optional (applicationIconId in the create body).
	var iconBlobID *int
	var iconBytes []byte
	if p := data.IconFilePath; !p.IsNull() && !p.IsUnknown() && p.ValueString() != "" {
		iconUploadResponse, b, err := r.uploadBlob(ctx, p.ValueString(), orgGroupID, false)
		if err != nil {
			r.cleanupBlobs(ctx, *appUploadResponse.Value, *plistUploadResponse.Value)
			resp.Diagnostics.AddError("Blob Upload Error", fmt.Sprintf("Failed to upload icon file: %s", err))
			return
		}
		iconBlobID, iconBytes = iconUploadResponse.Value, b
	}

	appCreateResponse, appId, err := r.createMacOSApp(
		ctx,
		orgGroupID,
		*appUploadResponse.Value,
		*plistUploadResponse.Value,
		iconBlobID,
		appversion)

	if err != nil {
		tflog.Trace(ctx, "app creating failed, deleting the blob.")
		blobs := []int{*appUploadResponse.Value, *plistUploadResponse.Value}
		if iconBlobID != nil {
			blobs = append(blobs, *iconBlobID)
		}
		r.cleanupBlobs(ctx, blobs...)
		resp.Diagnostics.AddError("App Creation Failed", fmt.Sprintf("Failed to create macOS application: %s", err))
		return
	}

	dmgSum := sha256.Sum256(dmgBytes)
	created := &createArtifacts{
		dmgFileSHA256:  hex.EncodeToString(dmgSum[:]),
		includeContent: data.IncludeContent.ValueBool(),
	}
	// A plist that is not XML (e.g. binary) has no canonical identity; leave
	// plist_file_sha256 null so any later plist_file_path change replaces.
	if plistSHA, err := canonicalPlistSHA256(plistBytes); err == nil {
		created.plistFileSHA256 = plistSHA
	}
	if iconBytes != nil {
		iconSum := sha256.Sum256(iconBytes)
		created.iconFileSHA256 = hex.EncodeToString(iconSum[:])
	}
	if created.includeContent {
		created.dmgContentBase64 = base64.StdEncoding.EncodeToString(dmgBytes)
		created.plistContentBase64 = base64.StdEncoding.EncodeToString(plistBytes)
	}

	r.refreshIntoState(ctx, appId, appCreateResponse, &data, &resp.State, &resp.Diagnostics, created)
	tflog.Trace(ctx, "created a macOS application resource")
}

// createArtifacts carries the values Create derives from the freshly
// uploaded DMG/plist bytes (SHA-256, and — only when include_content is
// true — the base64-encoded content) into refreshIntoState. Read passes nil
// here: its behavior is unchanged, preserving whatever is already in state
// rather than recomputing these from a fetch that has no bytes on hand.
type createArtifacts struct {
	dmgFileSHA256      string
	plistFileSHA256    string
	iconFileSHA256     string // "" when no icon was uploaded
	includeContent     bool
	dmgContentBase64   string
	plistContentBase64 string
}

func (r *macapplicationResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse) {

	var data tf.MacApplicationResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	applicationId, err := data.FetchValidAppID()
	if err != nil {
		resp.Diagnostics.AddError("Invalid application id", err.Error())
		return
	}

	fetchResponse, err := r.fetchMacApplicationDetails(ctx, applicationId)
	if err != nil {
		if !isAmbiguousNotFoundOrInaccessibleAPIError(err) {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to fetch application details: %s", err))
			return
		}
		// Confirm protects against a single flaky not-found response during a
		// live-tenant refresh dropping real Terraform state (see the 7fx
		// incident documented on internal/common/notfound): wait briefly and
		// re-GET once before actually dropping state and surfacing the
		// ambiguity warning below.
		confirmed, stillNotFound, confirmErr := notfound.Confirm(
			ctx,
			isAmbiguousNotFoundOrInaccessibleAPIError,
			func(ctx context.Context) (*sdk.InternalAppModelV1, error) {
				return r.fetchMacApplicationDetails(ctx, applicationId)
			},
		)
		if confirmErr != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to fetch application details: %s", confirmErr))
			return
		}
		if stillNotFound {
			resp.State.RemoveResource(ctx)
			resp.Diagnostics.AddWarning(
				"macOS Application Not Found Or Not Accessible",
				fmt.Sprintf(
					"The macOS internal application with id %d was not found, or the credentials "+
						"currently configured for this provider do not have access to it (API error 401, "+
						"errorCode 7000). This resource has been removed from Terraform state. Before "+
						"applying a plan that would recreate it, verify whether the application still "+
						"exists in the console and that these credentials have access to it — if this was "+
						"actually an access problem rather than a real deletion, recreating it here would "+
						"create a duplicate application.",
					applicationId,
				),
			)
			return
		}
		tflog.Warn(ctx, fmt.Sprintf("UEM returned not-found then found for macOS internal application %d; kept in state", applicationId), map[string]any{
			"application_id": applicationId,
		})
		fetchResponse = confirmed
	}

	tflog.Warn(ctx, "state set", map[string]any{
		"application_id": applicationId,
	})

	r.refreshIntoState(ctx, *fetchResponse.ID, fetchResponse, &data, &resp.State, &resp.Diagnostics, nil)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Warn(ctx, "read end", map[string]any{
		"application_id": applicationId,
	})

	tflog.Trace(ctx, "read a macOS application resource")
}

func (r *macapplicationResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse) {

	// The SDK exposes no update endpoint for macOS internal applications, so
	// Update never calls the API: it copies the plan straight into state.
	// Only changes the plan modifiers deliberately keep in place reach it —
	// a dmg_file_path/plist_file_path change to a file with the same content
	// identity (dmg_file_sha256 / plist_file_sha256), an app_version change
	// between numerically equivalent forms ("1.0.0" vs "1.0.0.0"), or an
	// org_group_id fill-in after a bare-uuid import. Every other change to
	// those attributes, and any include_content change, plans a replace.
	// Those in-place changes need no upload and no API write; the prior
	// sha256/content values are carried by UseStateForUnknown. (The earlier
	// delete-then-create implementation was incorrect regardless — it wrote
	// into locally seeded DeleteResponse/CreateResponse copies, so the inner
	// results never propagated to this real *resource.UpdateResponse.)
	var data tf.MacApplicationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)

	tflog.Trace(ctx, "updated a macOS application resource (state-only: path or padded-version change, no API call)")
}

func (r *macapplicationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data tf.MacApplicationResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	applicationId, err := data.FetchValidAppID()
	if err != nil {
		resp.Diagnostics.AddError("Invalid application id", err.Error())
		return
	}

	_, err = r.deleteMacOSApp(ctx, applicationId)
	if err != nil {
		if isNotFoundAPIError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete macOS application, got error: %s", err))
		return
	}

	tflog.Trace(ctx, "deleted a macOS application resource")
}

func (r *macapplicationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import ID is either a bare UUID (`<uuid>`) or a composite
	// `<uuid>,<org_group_id>`. org_group_id cannot be resolved from the SDK
	// during import (InternalAppModelV2 only exposes the org group's
	// NAME/UUID via ManagedBy/ManagedByUUID, not its numeric ID), so callers
	// who need org_group_id populated (and thus no RequiresReplace surprise
	// on the next apply) must supply it explicitly via the composite form.
	fields := strings.Split(req.ID, ",")
	var rawUUID string
	var orgGroupIDField string
	haveOrgGroupID := false
	switch len(fields) {
	case 1:
		rawUUID = fields[0]
	case 2:
		rawUUID = fields[0]
		orgGroupIDField = strings.TrimSpace(fields[1])
		haveOrgGroupID = true
	default:
		resp.Diagnostics.AddError(
			"Invalid Import ID",
			fmt.Sprintf("import ID must be either <uuid> or <uuid>,<org_group_id>, got: %q", req.ID))
		return
	}

	appUUID, err := tf.ValidateAppUUID(rawUUID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Application UUID", err.Error())
		return
	}

	// Same disposition as models.ValidateID (b16 row #101, mirroring row
	// #98): UEM source: AirWatch API/AW.Mam.Api/AW.Mam.Api/Controllers/macOS/Apps/V1/MacOsAppsV1Controller.cs:263-268
	// (canonical Q8) confirms org_group_id must be a known organization
	// group (validated server-side via ValidateOrganizationGroup), but has
	// no explicit `> 0` check of its own — the positive-integer requirement
	// here is stricter than UEM, kept as harmless defensive input hygiene.
	var orgGroupID int32
	if haveOrgGroupID {
		parsed, err := strconv.ParseInt(orgGroupIDField, 10, 32)
		if err != nil || parsed <= 0 {
			resp.Diagnostics.AddError(
				"Invalid Import ID",
				fmt.Sprintf("org_group_id in import ID must be a positive integer, got: %q", orgGroupIDField))
			return
		}
		orgGroupID = int32(parsed)
	}

	if r.blobDownloader == nil {
		resp.Diagnostics.AddError(
			"Provider Not Configured",
			"The app binary downloader is not configured; this is a bug if Configure() ran.")
		return
	}

	// B10: announce the app's name/size before the (potentially large,
	// potentially slow) download starts, and log coarse progress as it
	// downloads. appName/infoReceived are captured by both closures below
	// so a stall error can be reported by name even though it surfaces from
	// DownloadAppBlob's return, not from onInfo itself.
	var appName string
	var infoReceived bool
	lastProgressBucket := 0
	onInfo := func(name string, sizeKB *int) {
		appName = name
		infoReceived = true
		sizeDesc := "size unknown"
		if sizeKB != nil {
			sizeDesc = fmt.Sprintf("%.1f MB", float64(*sizeKB)/1024)
		}
		tflog.Info(ctx, fmt.Sprintf("uem_mac_application import: downloading %s (%s)", displayAppName(name, appUUID), sizeDesc))
	}
	progress := func(read, total int64) {
		// Guard against the details lookup's own small JSON response: only
		// report progress once onInfo has actually fired for THIS download.
		if !infoReceived || total <= 0 {
			return
		}
		bucket := int(float64(read) / float64(total) * 100)
		bucket -= bucket % 10
		if bucket <= 0 || bucket <= lastProgressBucket {
			return
		}
		lastProgressBucket = bucket
		tflog.Info(ctx, fmt.Sprintf("uem_mac_application import: %s download %d%% (%d/%d bytes)", displayAppName(appName, appUUID), bucket, read, total))
	}

	appID, blobBytes, pkginfo, err := r.blobDownloader.DownloadAppBlob(ctx, appUUID, onInfo, progress)
	if err != nil {
		var stallErr *httpclient.StallError
		if errors.As(err, &stallErr) {
			resp.Diagnostics.AddError("Download Stalled", stallDiagnosticMessage(displayAppName(appName, appUUID), stallErr))
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to download application blob for import: %s", err))
		return
	}

	destDir := artifactDir(r.appBinaryStoragePath, appUUID)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		resp.Diagnostics.AddError("Filesystem Error", fmt.Sprintf("Unable to create %s: %s", destDir, err))
		return
	}

	// B31: name the file by its actual content, not a hardcoded assumption —
	// UEM may hold a flat package (.pkg, a xar archive) instead of a DMG, and
	// saving it as app.dmg leaves a real .pkg looking corrupted on disk.
	//
	// F10: recorded as filepath.Clean(join(storageRoot, uuid, name)) with no
	// absolutising — destDir/blobFileName are already Clean (filepath.Join
	// cleans its result), and the explicit Clean here is a no-op that just
	// documents the invariant a plan-modifier-level path comparison
	// (cleanEquivalentPathUseState, plan_modifiers.go) relies on.
	dmgPath := filepath.Clean(filepath.Join(destDir, blobFileName(blobBytes)))
	if err := os.WriteFile(dmgPath, blobBytes, 0o644); err != nil {
		resp.Diagnostics.AddError("Filesystem Error", fmt.Sprintf("Unable to write %s: %s", dmgPath, err))
		return
	}

	sum := sha256.Sum256(blobBytes)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), int32(appID))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("uuid"), appUUID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("dmg_file_path"), dmgPath)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("dmg_file_sha256"), hex.EncodeToString(sum[:]))...)

	// plist_file_sha256 comes from the pkginfo the same V2 details lookup
	// already returned (no extra API call). Non-XML/empty pkginfo leaves it
	// null, so a later plist_file_path change replaces (the old behavior).
	if plistSHA, err := canonicalPlistSHA256([]byte(pkginfo)); err == nil {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("plist_file_sha256"), plistSHA)...)
	}

	// B32/F9: write the pkginfo and icon UEM holds next to the binary, in the
	// same destDir, and point plist_file_path / icon_file_path at them, so an
	// imported application carries the same create inputs a new one would.
	plistPath, err := writeImportedPlist(destDir, pkginfo)
	if err != nil {
		resp.Diagnostics.AddError("Filesystem Error", err.Error())
		return
	}
	if plistPath != "" {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("plist_file_path"), plistPath)...)
	}
	if icons, ok := r.blobDownloader.(appIconDownloader); ok {
		icon, uemIconFileName, err := icons.DownloadAppIcon(ctx, appUUID)
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to download application icon for import: %s", err))
			return
		}
		iconPath, iconSHA, err := writeImportedIcon(destDir, icon, uemIconFileName)
		if err != nil {
			resp.Diagnostics.AddError("Filesystem Error", err.Error())
			return
		}
		if iconPath != "" {
			resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("icon_file_path"), iconPath)...)
			resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("icon_file_sha256"), iconSHA)...)
		}
	}

	// #2: org_group_id cannot be resolved from the SDK during import (see
	// comment above). When the caller supplied it via the composite import
	// ID, set it directly — this is exact, not a guess. Otherwise leave it
	// unset (a bare-UUID import): org_group_id's plan modifiers (see
	// resource.go) collapse this to a clean plan as long as it also stays
	// unset in configuration, so warn accordingly rather than claiming a
	// recreate risk that no longer exists.
	if haveOrgGroupID {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("org_group_id"), types.Int32Value(orgGroupID))...)
	} else {
		resp.Diagnostics.AddWarning(
			"org_group_id Not Set by Import",
			"This resource's organization_group_id could not be determined during import "+
				"(the Workspace ONE UEM API does not expose it for this endpoint). It will "+
				"remain unset in state. Leaving org_group_id unset in your Terraform "+
				"configuration too will produce a clean plan (no destroy/recreate). If you "+
				"instead set org_group_id in your configuration, that value is adopted into "+
				"state as-is on the next apply — it is NOT verified against this application's "+
				"real organization group, and no API call is made to relocate it, so only set "+
				"it if you already know the true value. To track the exact value from the "+
				"start instead, re-import using the composite form "+
				"'terraform import uem_mac_application.example <uuid>,<org_group_id>'.",
		)
	}
}

func (r *macapplicationResource) uploadBlob(
	ctx context.Context,
	filePath string,
	orgGroupID int,
	isApplication bool) (*sdk.EntityV1Model, []byte, error) {

	blobV1Svc, err := r.BlobV1ResourceService(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to initialize blob V1 resource service: %w", err)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	fileName := filepath.Base(filePath)

	sizeMB := float64(len(data)) / (1024 * 1024)
	tflog.Info(ctx, "Uploading blob", map[string]interface{}{
		"file":    fileName,
		"size_mb": fmt.Sprintf("%.1f", sizeMB),
	})

	opts := CreateBlobUploadOptions(fileName, isApplication, orgGroupID)
	_, response, err := blobV1Svc.UploadBlobAsync(ctx, data, opts)

	if err != nil {
		return nil, nil, fmt.Errorf("blob upload failed for %s (%.1f MB): %w", fileName, sizeMB, err)
	}

	if response == nil ||
		response.Value == nil || *response.Value == 0 ||
		response.UUID == "" {
		return nil, nil, fmt.Errorf("blob upload returned invalid ID for file %s", fileName)
	}

	tflog.Info(ctx, "Blob upload successful", map[string]interface{}{
		"file":                fileName,
		"size_mb":             fmt.Sprintf("%.1f", sizeMB),
		"blob_id":             *response.Value,
		"blob_uuid":           response.UUID,
		"is_application_blob": isApplication,
		"org_group_id":        orgGroupID,
	})

	return response, data, nil
}

func (r *macapplicationResource) deleteBlob(
	ctx context.Context,
	blobId int) error {

	blobV2Svc, err := r.BlobV2ResourceService(ctx)
	if err != nil {
		return fmt.Errorf("unable to initialize blob V2 resource service: %w", err)
	}

	_, err = blobV2Svc.Delete(ctx, strconv.Itoa(blobId))
	if err != nil {
		return fmt.Errorf("blob deletion failed for blob ID %d: %w", blobId, err)
	}

	return nil
}

func (r *macapplicationResource) cleanupBlobs(ctx context.Context, blobIDs ...int) {
	if len(blobIDs) == 0 {
		return
	}

	errCh := make(chan error, len(blobIDs))
	var wg sync.WaitGroup
	for _, id := range blobIDs {
		blobID := id
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := r.deleteBlob(ctx, blobID); err != nil {
				errCh <- err
			}
		}()
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		tflog.Warn(ctx, "blob cleanup failed", map[string]any{"error": err.Error()})
	}
}

func (r *macapplicationResource) createMacOSApp(
	ctx context.Context,
	orgGroupID int,
	dmgBlobID int,
	plistBlobID int,
	iconBlobID *int,
	version string) (*sdk.InternalAppModelV1, int, error) {

	macAppSvc, err := r.MacAppService(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("unable to initialize mac application resource service: %w", err)
	}

	header, err := macAppSvc.CreateMacOSApplication(
		ctx,
		orgGroupID,
		CreateMacOSApplicationRequest(dmgBlobID, plistBlobID, iconBlobID, version))

	if err != nil {
		return nil, 0, fmt.Errorf("failed to create macOS application: %w", err)
	}

	appId, err := parseAppIDFromLocation(header)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to parse application ID from location header: %w", err)
	}

	fetchApplicationDetailsResponse, err := r.fetchMacApplicationDetails(ctx, appId)
	if err != nil {
		return nil, 0, err
	}

	return fetchApplicationDetailsResponse, appId, nil
}

func (r *macapplicationResource) fetchMacApplicationDetails(
	ctx context.Context,
	applicationID int) (*sdk.InternalAppModelV1, error) {
	internalAppSvc, err := r.InternalAppService(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to initialize internal app resource service: %w", err)
	}

	_, response, err := internalAppSvc.GetInternalAppByIdAsync(ctx, applicationID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch internal app details: %w", err)
	}

	return response, nil
}

func (r *macapplicationResource) deleteMacOSApp(
	ctx context.Context,
	applicationID int) (http.Header, error) {

	internalAppSvc, err := r.InternalAppService(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to initialize internal app resource service: %w", err)
	}

	header, err := internalAppSvc.DeleteInternalAppAsync(ctx, applicationID)
	if err != nil {
		return nil, fmt.Errorf("unable to delete mac internal app resource service: %w", err)
	}

	return header, nil
}

func (r *macapplicationResource) refreshIntoState(
	ctx context.Context,
	appId int,
	internalAppModel *sdk.InternalAppModelV1,
	data *tf.MacApplicationResourceModel,
	state *tfsdk.State,
	diags *diag.Diagnostics,
	created *createArtifacts,
) {
	if err := appState.SetMinimalState(data, appId, internalAppModel.UUID); err != nil {
		diags.AddError(
			"Unable to refresh mac application state",
			fmt.Sprintf("Failed to populate minimal state for mac application %d: %s", appId, err),
		)
		return
	}

	// app_version: UEM echoes the version sent at create time as
	// AirwatchAppVersion, reformatted as a four-segment .NET-style version
	// ("2.6.22" -> "2.6.22.0", "1.01" -> "1.1.0.0"). Read (created == nil)
	// takes it so import recovers it; the framework's semantic-equality pass
	// (tf.AppVersionValue) keeps a prior configured "2.6.22" rather than
	// surfacing the padded form as a diff. Create keeps the planned value
	// exactly — overwriting it with the echo would be an inconsistent result
	// whenever UEM rewrites the version beyond what semantic equality
	// accepts. An empty API value leaves the prior state untouched.
	// Live-confirmed 2026-09-23 on as<internal-env> 26.2 (mac_application app_version
	// readback). Evidence:
	// internal-design-doc;
	// commit c03a7267ea.
	if v := internalAppModel.AirwatchAppVersion; created == nil && v != "" {
		data.AppVersion = tf.NewAppVersionValue(v)
	}

	diags.Append(setRecordFromV1(data, internalAppModel)...)

	if created != nil {
		if created.iconFileSHA256 != "" {
			data.IconFileSHA256 = types.StringValue(created.iconFileSHA256)
		} else {
			data.IconFileSHA256 = types.StringNull()
		}
		if data.IconFilePath.IsUnknown() {
			data.IconFilePath = types.StringNull()
		}
		data.DMGFileSHA256 = types.StringValue(created.dmgFileSHA256)
		// plist_file_sha256 is written only here (Create) and by ImportState —
		// never by Read.
		if created.plistFileSHA256 != "" {
			data.PlistFileSHA256 = types.StringValue(created.plistFileSHA256)
		} else {
			data.PlistFileSHA256 = types.StringNull()
		}
		if created.includeContent {
			data.DMGContentBase64 = types.StringValue(created.dmgContentBase64)
			data.PlistContentBase64 = types.StringValue(created.plistContentBase64)
		} else {
			data.DMGContentBase64 = types.StringNull()
			data.PlistContentBase64 = types.StringNull()
		}
	}

	diags.Append(state.Set(ctx, data)...)
}
