package assignment

import (
	"context"
	"fmt"
	"sync"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

var _ resource.Resource = &sensorAssignmentResource{}
var _ resource.ResourceWithImportState = &sensorAssignmentResource{}

func NewResource() resource.Resource {
	return &sensorAssignmentResource{}
}

func NewSensorAssignmentResource() resource.Resource {
	return NewResource()
}

type sensorAssignmentResource struct {
	client                     *sdk.Client
	newSensorAssignmentService func(*sdk.Client) sensorAssignmentServiceAPI

	sensorAssignmentSvc     sensorAssignmentServiceAPI
	sensorAssignmentSvcOnce sync.Once
	sensorAssignmentSvcErr  error
}

func (r *sensorAssignmentResource) sensorAssignmentService(ctx context.Context) (sensorAssignmentServiceAPI, error) {
	_ = ctx
	r.sensorAssignmentSvcOnce.Do(func() {
		if r.client == nil {
			r.sensorAssignmentSvcErr = fmt.Errorf("sensor assignment resource is not configured: SDK client is nil")
			return
		}
		factory := r.newSensorAssignmentService
		if factory == nil {
			factory = defaultSensorAssignmentServiceFactory
		}
		r.sensorAssignmentSvc = factory(r.client)
	})
	return r.sensorAssignmentSvc, r.sensorAssignmentSvcErr
}

func (r *sensorAssignmentResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sensor_assignment"
}
