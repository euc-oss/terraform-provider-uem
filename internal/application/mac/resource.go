package macapplication

import (
	"context"
	"fmt"
	"sync"

	sdk "github.com/euc-oss/terraform-sdk-uem"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int32planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
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
	newBlobResourceService        func(*sdk.Client) appBlobResourceServiceAPI
	newMacAppResourceService      func(*sdk.Client) macAppResourceServiceAPI
	newInternalAppResourceService func(*sdk.Client) InternalAppsV1ServiceAPI

	// blobResourceSvc is the lazily initialized Layer 2 service used by CRUD paths.
	blobResourceSvc     appBlobResourceServiceAPI
	blobResourceSvcOnce sync.Once
	blobResourceSvcErr  error

	appService     macAppResourceServiceAPI
	appServiceOnce sync.Once
	appServiceErr  error

	internalAppService     InternalAppsV1ServiceAPI
	internalAppServiceOnce sync.Once
	internalAppServiceErr  error
}

// BlobResourceService returns the Layer 2 BlobResourceService, constructing it lazily on
// first use via the configured factory. Subsequent calls return the cached
// service — or the cached error, if the first attempt failed.
func (r *macapplicationResource) BlobResourceService(ctx context.Context) (appBlobResourceServiceAPI, error) {
	_ = ctx
	r.blobResourceSvcOnce.Do(func() {
		if r.client == nil {
			r.blobResourceSvcErr = fmt.Errorf("blob resource service is not configured: SDK client is nil")
			return
		}
		factory := r.newBlobResourceService
		if factory == nil {
			factory = defaultBlobResourceServiceFactory
		}
		r.blobResourceSvc = factory(r.client)
	})
	return r.blobResourceSvc, r.blobResourceSvcErr
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
				Required:            true,
				MarkdownDescription: "Organization Group ID (integer) where the file will be created",
				PlanModifiers: []planmodifier.Int32{
					int32planmodifier.RequiresReplace(),
				},
			},
			"dmg_file_path": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Path to the DMG file on disk",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"plist_file_path": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Path to the plist (pkginfo) XML file describing app metadata",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"app_version": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Application version string (e.g., \"2.6.22\")",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}
