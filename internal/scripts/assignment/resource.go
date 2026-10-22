package assignment

import (
	"context"
	"fmt"
	"sync"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

var _ resource.Resource = &scriptAssignmentResource{}
var _ resource.ResourceWithImportState = &scriptAssignmentResource{}

func NewResource() resource.Resource {
	return &scriptAssignmentResource{}
}

func NewScriptAssignmentResource() resource.Resource {
	return NewResource()
}

type scriptAssignmentResource struct {
	client                     *sdk.Client
	newScriptAssignmentService func(*sdk.Client) scriptAssignmentServiceAPI

	scriptAssignmentSvc     scriptAssignmentServiceAPI
	scriptAssignmentSvcOnce sync.Once
	scriptAssignmentSvcErr  error

	newScriptService func(*sdk.Client) scriptServiceAPI
	scriptSvc        scriptServiceAPI
	scriptSvcOnce    sync.Once
	scriptSvcErr     error
}

// scriptService lazily builds the scripts service used for the parent
// script's organization group and the confirming list lookup.
func (r *scriptAssignmentResource) scriptService() (scriptServiceAPI, error) {
	r.scriptSvcOnce.Do(func() {
		if r.client == nil {
			r.scriptSvcErr = fmt.Errorf("script assignment resource is not configured: SDK client is nil")
			return
		}
		factory := r.newScriptService
		if factory == nil {
			factory = defaultScriptServiceFactory
		}
		r.scriptSvc = factory(r.client)
	})
	return r.scriptSvc, r.scriptSvcErr
}

func (r *scriptAssignmentResource) scriptAssignmentService(ctx context.Context) (scriptAssignmentServiceAPI, error) {
	_ = ctx
	r.scriptAssignmentSvcOnce.Do(func() {
		if r.client == nil {
			r.scriptAssignmentSvcErr = fmt.Errorf("script assignment resource is not configured: SDK client is nil")
			return
		}
		factory := r.newScriptAssignmentService
		if factory == nil {
			factory = defaultScriptAssignmentServiceFactory
		}
		r.scriptAssignmentSvc = factory(r.client)
	})
	return r.scriptAssignmentSvc, r.scriptAssignmentSvcErr
}

func (r *scriptAssignmentResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_script_assignment"
}
