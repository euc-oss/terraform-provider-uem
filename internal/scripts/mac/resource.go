package macscript

import (
	"context"
	"fmt"
	"sync"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// Ensure resource types satisfy framework interfaces.
var _ resource.Resource = &macscriptResource{}
var _ resource.ResourceWithImportState = &macscriptResource{}

func NewResource() resource.Resource {
	return &macscriptResource{}
}

func NewMacscriptUploadResource() resource.Resource {
	return NewResource()
}

// macscriptResource defines the resource implementation.
type macscriptResource struct {
	client                *sdk.Client
	newScriptServiceV1API func(*sdk.Client) ScriptServiceV1API

	// scriptServiceV1Svc is the lazily initialized Layer 2 service used by CRUD paths.
	scriptServiceV1Svc     ScriptServiceV1API
	scriptServiceV1SvcOnce sync.Once
	scriptServiceV1SvcErr  error
}

func (r *macscriptResource) MacScriptAppService(ctx context.Context) (ScriptServiceV1API, error) {
	_ = ctx
	r.scriptServiceV1SvcOnce.Do(func() {
		if r.client == nil {
			r.scriptServiceV1SvcErr = fmt.Errorf("internal app service is not configured: SDK client is nil")
			return
		}
		factory := r.newScriptServiceV1API
		if factory == nil {
			factory = defaulScriptServiceV1Factory
		}
		r.scriptServiceV1Svc = factory(r.client)
	})
	return r.scriptServiceV1Svc, r.scriptServiceV1SvcErr
}

func (r *macscriptResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_mac_script"
}
