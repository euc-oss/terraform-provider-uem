package deployment

import (
	"context"
	"fmt"
	"sync"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

var _ resource.Resource = &updateDeploymentResource{}
var _ resource.ResourceWithImportState = &updateDeploymentResource{}

func NewResource() resource.Resource {
	return &updateDeploymentResource{}
}

type updateDeploymentResource struct {
	client         *sdk.Client
	newUpdatesV1   func(*sdk.Client) UpdatesV1API
	updatesV1Svc   UpdatesV1API
	updatesV1Once  sync.Once
	updatesV1Error error
}

func (r *updateDeploymentResource) UpdatesService(ctx context.Context) (UpdatesV1API, error) {
	_ = ctx
	r.updatesV1Once.Do(func() {
		if r.client == nil {
			r.updatesV1Error = fmt.Errorf("updates service is not configured: SDK client is nil")
			return
		}
		factory := r.newUpdatesV1
		if factory == nil {
			factory = defaultUpdatesV1Factory
		}
		r.updatesV1Svc = factory(r.client)
	})
	return r.updatesV1Svc, r.updatesV1Error
}

func (r *updateDeploymentResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_update_deployment"
}
