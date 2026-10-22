package macapplication

import (
	"context"
	"fmt"
	"sync"

	"github.com/euc-oss/terraform-provider-uem/internal/httpclient"
	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int32planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"

	tf "github.com/euc-oss/terraform-provider-uem/internal/application/internal-app/mac/models"
)

// Ensure resource types satisfy framework interfaces.
var _ resource.Resource = &macapplicationResource{}
var _ resource.ResourceWithImportState = &macapplicationResource{}

func NewResource() resource.Resource {
	return &macapplicationResource{}
}

func NewMacApplicationUploadResource() resource.Resource {
	return NewResource()
}

// macapplicationResource defines the resource implementation.
type macapplicationResource struct {
	client                        *sdk.Client
	newBlobV1ResourceService      func(*sdk.Client) appBlobV1ResourceServiceAPI
	newBlobV2ResourceService      func(*sdk.Client) appBlobV2ResourceServiceAPI
	newMacAppResourceService      func(*sdk.Client) macAppResourceServiceAPI
	newInternalAppResourceService func(*sdk.Client) InternalAppsV1ServiceAPI

	// blobV1ResourceSvc and blobV2ResourceSvc are lazily initialized Layer 2 services
	// used by CRUD paths: V1 for upload operations and V2 for delete operations.
	blobV1ResourceSvc     appBlobV1ResourceServiceAPI
	blobV1ResourceSvcOnce sync.Once
	blobV1ResourceSvcErr  error

	blobV2ResourceSvc     appBlobV2ResourceServiceAPI
	blobV2ResourceSvcOnce sync.Once
	blobV2ResourceSvcErr  error

	appService     macAppResourceServiceAPI
	appServiceOnce sync.Once
	appServiceErr  error

	internalAppService     InternalAppsV1ServiceAPI
	internalAppServiceOnce sync.Once
	internalAppServiceErr  error

	// appBinaryStoragePath is the provider-level directory under which
	// imported application binaries are written (<appBinaryStoragePath>/<uuid>/app.dmg).
	appBinaryStoragePath string
	// blobDownloader downloads an application binary blob by app UUID during
	// ImportState. Injected via Configure(); a fake is injected directly in
	// unit tests.
	blobDownloader appBlobDownloader
}

// appInfoFunc is called once by appBlobDownloader.DownloadAppBlob, as soon
// as the application's details are known and BEFORE the (potentially large,
// potentially slow) blob download itself starts — so callers (ImportState)
// can log the app's name/size up front (B10) without waiting for the
// download to finish or fail. It is never called if the details lookup
// itself fails. Name is "" and sizeKB is nil when the underlying API left
// those fields empty.
type appInfoFunc func(name string, sizeKB *int)

// appBlobDownloader downloads the raw application binary blob for a given
// application UUID. Implementations are used by ImportState to reconstruct
// dmg_file_path/dmg_file_sha256 for a resource brought under management via
// `terraform import`.
type appBlobDownloader interface {
	// onInfo (may be nil) is invoked once the app's name/size are known,
	// before the download itself starts (B10). progress (may be nil) is
	// invoked as the download body is read (see httpclient.ProgressFunc);
	// it is wired only around the blob download, never the details lookup.
	// pkginfo is the application's inline MacOsSoftwareDeploymentSummary
	// Pkginfo from the same details lookup ("" when absent).
	DownloadAppBlob(ctx context.Context, appUUID string, onInfo appInfoFunc, progress httpclient.ProgressFunc) (id int, blob []byte, pkginfo string, err error)
}

// sdkBlobDownloader is the production appBlobDownloader backed by the SDK
// client. It resolves an application UUID to its details via the internal
// apps V2 service, extracts the application file blob GUID, and downloads
// the blob bytes via the blobs V2 service.
type sdkBlobDownloader struct {
	client            *sdk.Client
	internalAppLookup internalAppUUIDLookupAPI
	blobService       appBlobV2ResourceServiceAPI
}

func (d *sdkBlobDownloader) DownloadAppBlob(ctx context.Context, appUUID string, onInfo appInfoFunc, progress httpclient.ProgressFunc) (int, []byte, string, error) {
	if d.internalAppLookup == nil || d.blobService == nil {
		return 0, nil, "", fmt.Errorf("blob downloader is not configured")
	}

	_, details, err := d.internalAppLookup.GetInternalAppByUuid(ctx, appUUID)
	if err != nil {
		return 0, nil, "", fmt.Errorf("failed to fetch internal app details for uuid %s: %w", appUUID, err)
	}
	if details == nil || details.ApplicationFileBlobGUID == "" {
		return 0, nil, "", fmt.Errorf("internal app %s has no application file blob GUID", appUUID)
	}
	if details.ID == nil {
		return 0, nil, "", fmt.Errorf("internal app %s has no numeric id", appUUID)
	}

	// B10: announce name/size BEFORE the download starts — details is
	// already fully populated by the GetInternalAppByUuid call above, so
	// this adds no extra API call.
	if onInfo != nil {
		onInfo(details.ApplicationName, details.AppSizeInKB)
	}

	downloadCtx := ctx
	if progress != nil {
		// Scoped to ONLY this call, not the details lookup above, so a
		// small JSON response never triggers a spurious progress log line.
		downloadCtx = httpclient.WithProgress(ctx, progress)
	}
	_, blob, err := d.blobService.Get(downloadCtx, details.ApplicationFileBlobGUID)
	if err != nil {
		return 0, nil, "", fmt.Errorf("failed to download blob %s: %w", details.ApplicationFileBlobGUID, err)
	}

	pkginfo := ""
	if details.MacOsSoftwareDeploymentSummary != nil {
		pkginfo = details.MacOsSoftwareDeploymentSummary.Pkginfo
	}

	return *details.ID, blob, pkginfo, nil
}

// newSDKBlobDownloader constructs the production appBlobDownloader, wiring
// the real internal apps V2 (UUID lookup) and blobs V2 (download) services.
func newSDKBlobDownloader(client *sdk.Client) appBlobDownloader {
	return &sdkBlobDownloader{
		client:            client,
		internalAppLookup: sdk.NewInternalAppsV2Service(client),
		blobService:       sdk.NewBlobsV2Service(client),
	}
}

// BlobV1ResourceService returns the Layer 2 Blob V1 resource service,
// constructing it lazily on first use via the configured factory. V1 is used
// for upload flows, while delete flows use the V2 service. Subsequent calls
// return the cached service — or the cached error, if the first attempt
// failed.
func (r *macapplicationResource) BlobV1ResourceService(ctx context.Context) (appBlobV1ResourceServiceAPI, error) {
	_ = ctx
	r.blobV1ResourceSvcOnce.Do(func() {
		if r.client == nil {
			r.blobV1ResourceSvcErr = fmt.Errorf("blob V1 resource service is not configured: SDK client is nil")
			return
		}
		factory := r.newBlobV1ResourceService
		if factory == nil {
			factory = defaultBlobV1ResourceServiceFactory
		}
		r.blobV1ResourceSvc = factory(r.client)
	})
	return r.blobV1ResourceSvc, r.blobV1ResourceSvcErr
}

// BlobV2ResourceService returns the Layer 2 Blob V2 resource service,
// constructing it lazily on first use via the configured factory. V2 is used
// for delete flows, while upload flows use the V1 service. Subsequent calls
// return the cached service — or the cached error, if the first attempt
// failed.
func (r *macapplicationResource) BlobV2ResourceService(ctx context.Context) (appBlobV2ResourceServiceAPI, error) {
	_ = ctx
	r.blobV2ResourceSvcOnce.Do(func() {
		if r.client == nil {
			r.blobV2ResourceSvcErr = fmt.Errorf("blob V2 resource service is not configured: SDK client is nil")
			return
		}
		factory := r.newBlobV2ResourceService
		if factory == nil {
			factory = defaultBlobV2ResourceServiceFactory
		}
		r.blobV2ResourceSvc = factory(r.client)
	})
	return r.blobV2ResourceSvc, r.blobV2ResourceSvcErr
}

func (r *macapplicationResource) MacAppService(ctx context.Context) (macAppResourceServiceAPI, error) {
	_ = ctx
	r.appServiceOnce.Do(func() {
		if r.client == nil {
			r.appServiceErr = fmt.Errorf("mac application service is not configured: SDK client is nil")
			return
		}
		factory := r.newMacAppResourceService
		if factory == nil {
			factory = defaultMacAppResourceServiceFactory
		}
		r.appService = factory(r.client)
	})
	return r.appService, r.appServiceErr
}

func (r *macapplicationResource) InternalAppService(ctx context.Context) (InternalAppsV1ServiceAPI, error) {
	_ = ctx
	r.internalAppServiceOnce.Do(func() {
		if r.client == nil {
			r.internalAppServiceErr = fmt.Errorf("internal app service is not configured: SDK client is nil")
			return
		}
		factory := r.newInternalAppResourceService
		if factory == nil {
			factory = defaultInternalAppResourceServiceFactory
		}
		r.internalAppService = factory(r.client)
	})
	return r.internalAppService, r.internalAppServiceErr
}

func (r *macapplicationResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_mac_application"
}

func (r *macapplicationResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Uploads a macOS application to Workspace ONE UEM. " +
			"This resource handles the full upload workflow: uploading the file as a blob, ",

		Attributes: map[string]schema.Attribute{
			"id": schema.Int32Attribute{
				Computed:            true,
				MarkdownDescription: "The internal ID assigned by UEM after creation",
				PlanModifiers: []planmodifier.Int32{
					int32planmodifier.UseStateForUnknown(),
				},
			},
			"uuid": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The internal UUID assigned by UEM after creation",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"org_group_id": schema.Int32Attribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "Organization Group ID (integer) where the file will be created. " +
					"Required to create a new application. " +
					"The Workspace ONE UEM API does not expose a way to resolve this value from an " +
					"application's UUID alone, so a bare `terraform import uem_mac_application.example <uuid>` " +
					"leaves this attribute unset in state, producing a clean plan with no destroy/recreate " +
					"(supply it explicitly instead via the composite import ID form " +
					"`terraform import uem_mac_application.example <uuid>,<org_group_id>` if you want it " +
					"tracked from the start). Once a known value is set — either via the composite import " +
					"form or a subsequent apply — changing it to a different value still forces the " +
					"resource to be destroyed and recreated.",
				PlanModifiers: []planmodifier.Int32{
					int32planmodifier.UseStateForUnknown(),
					int32planmodifier.RequiresReplaceIf(
						func(_ context.Context, req planmodifier.Int32Request, resp *int32planmodifier.RequiresReplaceIfFuncResponse) {
							resp.RequiresReplace = !req.StateValue.IsNull()
						},
						"If this attribute already has a known value and it changes, Terraform will destroy "+
							"and recreate the resource. A bare-uuid import leaving it null, followed by a "+
							"fill-in value, does not force replacement.",
						"If this attribute already has a known value and it changes, Terraform will destroy "+
							"and recreate the resource. A bare-uuid import leaving it null, followed by a "+
							"fill-in value, does not force replacement.",
					),
				},
			},
			"dmg_file_path": schema.StringAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "Path to the application binary on disk, uploaded at create time. This may be " +
					"a DMG or a flat package (PKG) — on import, the file is named app.pkg or app.dmg to match the " +
					"blob UEM actually holds (sniffed from its content, not assumed). The application's " +
					"identity is the file's content, not its path: changing only the path to a file with the same " +
					"SHA-256 as `dmg_file_sha256` is an in-place update that records the new path (no re-upload, no " +
					"API call). A path whose file content differs, or that is missing or unreadable at plan time, " +
					"forces the resource to be destroyed and recreated.",
				// UseStateForUnknown runs first so a null config value (marked
				// unknown by the framework once the plan differs, e.g. after an
				// import) resolves to the prior state value before the replace
				// rule sees it; otherwise the rule would see "unknown" and replace.
				// cleanEquivalentPathUseState runs next: a configured path that
				// is only cosmetically different from state (same string after
				// filepath.Clean — a local path comparison, not a UEM rule)
				// keeps the state value, so the replace rule never even sees a
				// changed plan value and no diff is planned at all.
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					cleanEquivalentPathUseState{},
					dmgFilePathRequiresReplace(),
				},
			},
			"plist_file_path": schema.StringAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "Path to the plist (pkginfo) XML file describing app metadata, uploaded at " +
					"create time. Identity is the plist's canonical content (see `plist_file_sha256`), not its path: " +
					"changing only the path to a plist that canonicalizes identically (whitespace, `<dict>` key " +
					"order and XML declaration differences are ignored) is an in-place update that records the new " +
					"path (no re-upload, no API call). A content change, a missing/unreadable/non-XML file at plan " +
					"time, or state without `plist_file_sha256` forces the resource to be destroyed and recreated.",
				// UseStateForUnknown runs first so a null config value (marked
				// unknown by the framework once the plan differs, e.g. after an
				// import) resolves to the prior state value before the replace
				// rule sees it; otherwise the rule would see "unknown" and replace.
				// cleanEquivalentPathUseState runs next: a configured path that
				// is only cosmetically different from state (same string after
				// filepath.Clean — a local path comparison, not a UEM rule)
				// keeps the state value, so the replace rule never even sees a
				// changed plan value and no diff is planned at all.
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					cleanEquivalentPathUseState{},
					plistFilePathRequiresReplace(),
				},
			},
			"app_version": schema.StringAttribute{
				// UEM reads the version back as a four-segment .NET-style
				// version ("2.6.22" -> "2.6.22.0", "1.01" -> "1.1.0.0");
				// AppVersionType's semantic equality treats those as the same
				// version so Read can populate it without a diff (and import
				// can recover it).
				//
				// Genuinely optional (B16 decision table row #99, CORRECT):
				// UEM source: AirWatch API/AW.Mam.Api/AW.Mam.Api/Controllers/macOS/Apps/V1/MacOsAppsV1Controller.cs:118-121
				// (canonical Q9) — Version has no FluentValidation rule and is
				// only validated when non-blank; omitting app_version no
				// longer fails Create (it did previously, contrary to this
				// attribute's own Optional schema declaration).
				CustomType: tf.AppVersionType{},
				Optional:   true,
				Computed:   true,
				MarkdownDescription: "Application version string (e.g., \"2.6.22\"), sent to UEM at create time and " +
					"read back from the API (UEM's `AirwatchAppVersion`, which UEM pads to four numeric segments, " +
					"e.g. \"2.6.22.0\"), so import recovers it. Optional: UEM does not require it, so omitting it is " +
					"sent as blank rather than rejected. Numerically equivalent versions — differing only by " +
					"trailing zero segments or leading zeros within a segment (\"1.01\" vs \"1.1.0.0\") — are treated " +
					"as the same version: no diff on refresh, and a configuration change between such forms is an " +
					"in-place update with no API call. Any other change forces the resource to be destroyed and " +
					"recreated. Versions containing non-numeric segments compare exactly; if UEM ever rewrites such " +
					"a version, a refresh may show a diff.",
				// UseStateForUnknown runs first so a null config value (marked
				// unknown by the framework once the plan differs, e.g. after an
				// import) resolves to the prior state value before the replace
				// rule sees it; otherwise the rule would see "unknown" and replace.
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					appVersionRequiresReplace(),
				},
			},
			"include_content": schema.BoolAttribute{
				Optional: true,
				MarkdownDescription: "When true, the provider returns the binary/metadata bytes as base64 in " +
					"dmg_content_base64 / plist_content_base64. When true AND no explicit *_file_path is set, " +
					"the provider returns bytes inline WITHOUT writing to disk. Default false: write to disk " +
					"(at app_binary_storage_path) and leave content attributes null. Enabling this for a large " +
					"binary stores its full base64 in Terraform state — cheap for a small plist, heavy for a DMG. " +
					"Changing this value forces replacement: dmg_content_base64/plist_content_base64 are only " +
					"populated by Create (Update is a no-op — the SDK has no update endpoint), so an in-place " +
					"toggle would silently leave stale/null content instead of actually (de)populating it.",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.RequiresReplace(),
				},
			},
			"dmg_content_base64": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Base64 of the DMG binary. Populated only when include_content = true.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"plist_content_base64": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Base64 of the plist. Populated only when include_content = true.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"dmg_file_sha256": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "SHA-256 of the DMG binary as recorded/derived at create or import, for drift detection.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"plist_file_sha256": schema.StringAttribute{
				Computed: true,
				MarkdownDescription: "SHA-256 of the plist's canonical XML form (insignificant whitespace, `<dict>` key " +
					"order and XML declaration/DOCTYPE removed), recorded at create (from the uploaded plist) or " +
					"import (from the application's pkginfo). Used to decide whether a `plist_file_path` change is a " +
					"content change. Null when the plist is not XML (e.g. a binary plist); never refreshed by read.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"icon_file_path": schema.StringAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "Path to the application icon file on disk, uploaded at create time (the create " +
					"body's `applicationIconId`). Import downloads the icon UEM holds and sets this path, naming " +
					"the file from UEM's Content-Disposition filename extension when it has a usable one (live " +
					"26.2 observed `<name>.png`), else from sniffing the downloaded bytes (PNG, JPEG, or GIF), " +
					"else a bare `icon` with no extension. Identity " +
					"is the file's content (see `icon_file_sha256`), not its path: changing only the path to a file " +
					"with the same SHA-256 is an in-place update that records the new path — including renaming " +
					"the imported file later to add or correct an extension, which is also an in-place path " +
					"update with no re-upload. A content change, or a " +
					"missing/unreadable file at plan time, forces the resource to be destroyed and recreated, since " +
					"UEM has no update for the application record.",
				// Same ordering reason as dmg_file_path above, including the
				// Clean-equivalent path rule: onboard renders this path as
				// "${path.root}/..." like dmg_file_path, which must plan no
				// diff against the recorded relative path.
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					cleanEquivalentPathUseState{},
					iconFilePathRequiresReplace(),
				},
			},
			"icon_file_sha256": schema.StringAttribute{
				Computed: true,
				MarkdownDescription: "SHA-256 of the icon file as recorded at create (from the uploaded file) or " +
					"import (from the downloaded icon). Null when the application has no icon. Never refreshed by read.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
	for name, a := range recordSchemaAttributes() {
		resp.Schema.Attributes[name] = a
	}
}
