package macsensor

import (
	"context"
	"fmt"
	"sync"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

var _ resource.Resource = &macsensorResource{}
var _ resource.ResourceWithImportState = &macsensorResource{}

func NewResource() resource.Resource {
	return &macsensorResource{}
}

func NewMacSensorResource() resource.Resource {
	return NewResource()
}

type macsensorResource struct {
	client                 *sdk.Client
	newDeviceSensorsV2API  func(*sdk.Client) DeviceSensorsV2API
	deviceSensorsV2Svc     DeviceSensorsV2API
	deviceSensorsV2SvcOnce sync.Once
	deviceSensorsV2SvcErr  error
}

func (r *macsensorResource) DeviceSensorsService(ctx context.Context) (DeviceSensorsV2API, error) {
	_ = ctx
	r.deviceSensorsV2SvcOnce.Do(func() {
		if r.client == nil {
			r.deviceSensorsV2SvcErr = fmt.Errorf("device sensors service is not configured: SDK client is nil")
			return
		}
		factory := r.newDeviceSensorsV2API
		if factory == nil {
			factory = defaultDeviceSensorsV2Factory
		}
		r.deviceSensorsV2Svc = factory(r.client)
	})
	return r.deviceSensorsV2Svc, r.deviceSensorsV2SvcErr
}

func (r *macsensorResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_mac_sensor"
}
