package assignment

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	commonerrors "github.com/euc-oss/terraform-provider-uem/internal/common/errors"
	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
)

type purchasedAppAssignmentServiceAPI interface {
	GetAssignmentRuleAsync(
		ctx context.Context,
		ApplicationUUID string,
	) (http.Header, *sdk.AppAssignmentRuleV2Model, error)
	UpdateAssignmentRuleAsync(
		ctx context.Context,
		ApplicationUUID string,
		request *sdk.AppAssignmentRuleV2Model,
	) (http.Header, error)
}

type resourceConfigData struct {
	client                           *sdk.Client
	newPurchasedAppAssignmentService func(c *sdk.Client) purchasedAppAssignmentServiceAPI
	newVppPlatformLookup             func(c *sdk.Client) vppPlatformLookupAPI
}

// defaultPurchasedAppAssignmentServiceFactory binds to api_version=v2.
func defaultPurchasedAppAssignmentServiceFactory(c *sdk.Client) purchasedAppAssignmentServiceAPI {
	return sdk.NewAppsV2Service(c)
}

func isNotFoundAPIError(err error) bool {
	return commonerrors.IsNotFoundAPIError(err)
}

func resourceTypeErrorDetail(got any) string {
	return fmt.Sprintf(
		"Expected *resourceConfigData or *sdk.Client, got: %T. Please report this issue to the provider developers.",
		got,
	)
}

// purchasedApplicationAssignmentResource defines the resource implementation.
type purchasedApplicationAssignmentResource struct {
	client                           *sdk.Client
	newPurchasedAppAssignmentService func(*sdk.Client) purchasedAppAssignmentServiceAPI
	newVppPlatformLookup             func(*sdk.Client) vppPlatformLookupAPI

	purchasedAppAssignmentSvc     purchasedAppAssignmentServiceAPI
	purchasedAppAssignmentSvcOnce sync.Once
	purchasedAppAssignmentSvcErr  error

	vppPlatformLookupSvc     vppPlatformLookupAPI
	vppPlatformLookupSvcOnce sync.Once
	vppPlatformLookupSvcErr  error
}

func (r *purchasedApplicationAssignmentResource) purchasedAppAssignmentService(ctx context.Context) (purchasedAppAssignmentServiceAPI, error) {
	_ = ctx
	r.purchasedAppAssignmentSvcOnce.Do(func() {
		if r.client == nil {
			r.purchasedAppAssignmentSvcErr = fmt.Errorf("purchased application assignment resource is not configured: SDK client is nil")
			return
		}
		factory := r.newPurchasedAppAssignmentService
		if factory == nil {
			factory = defaultPurchasedAppAssignmentServiceFactory
		}
		r.purchasedAppAssignmentSvc = factory(r.client)
	})
	return r.purchasedAppAssignmentSvc, r.purchasedAppAssignmentSvcErr
}

// vppPlatformLookupService lazily builds the platform lookup used by
// ModifyPlan (resource_modify_plan.go, internal-task), mirroring the
// purchasedAppAssignmentService lazy-build pattern above.
func (r *purchasedApplicationAssignmentResource) vppPlatformLookupService(ctx context.Context) (vppPlatformLookupAPI, error) {
	_ = ctx
	r.vppPlatformLookupSvcOnce.Do(func() {
		if r.client == nil {
			r.vppPlatformLookupSvcErr = fmt.Errorf("purchased application assignment resource is not configured: SDK client is nil")
			return
		}
		factory := r.newVppPlatformLookup
		if factory == nil {
			factory = defaultVppPlatformLookupFactory
		}
		r.vppPlatformLookupSvc = factory(r.client)
	})
	return r.vppPlatformLookupSvc, r.vppPlatformLookupSvcErr
}
