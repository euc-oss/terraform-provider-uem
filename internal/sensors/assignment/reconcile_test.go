package assignment

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/types"

	tf "github.com/euc-oss/terraform-provider-uem/internal/sensors/assignment/models"
)

// dupTolerantSensorAssignmentService is a minimal sensorAssignmentServiceAPI
// stub for TestReconcileAssignments_DuplicateDesiredEntriesAreNotRejected: it
// starts with no current assignments and answers every
// AddDeviceSensorAssignmentAsync call with a distinct fake UUID, counting
// how many times it was called.
type dupTolerantSensorAssignmentService struct {
	addCalls    int
	updateCalls int
}

func (f *dupTolerantSensorAssignmentService) AddDeviceSensorAssignmentAsync(
	_ context.Context, _ string, _ *sdk.DeviceSensorAssignmentRequestV1ModelV2,
) (http.Header, *sdk.BaseModelV2, error) {
	f.addCalls++
	return nil, &sdk.BaseModelV2{UUID: fmt.Sprintf("11111111-1111-1111-1111-%012d", f.addCalls)}, nil
}

func (f *dupTolerantSensorAssignmentService) GetDeviceSensorAssignmentsAsync(
	context.Context, string,
) (http.Header, *[]sdk.DeviceSensorAssignmentResponseV1ModelV2, error) {
	empty := []sdk.DeviceSensorAssignmentResponseV1ModelV2{}
	return nil, &empty, nil
}

func (f *dupTolerantSensorAssignmentService) GetDeviceSensorAssignmentAsync(
	context.Context, string,
) (http.Header, *sdk.DeviceSensorAssignmentResponseV1ModelV2, error) {
	return nil, nil, nil
}

func (f *dupTolerantSensorAssignmentService) UpdateDeviceSensorAssignmentAsync(
	context.Context, string, *sdk.DeviceSensorAssignmentRequestV1ModelV2,
) (http.Header, *sdk.BaseExceptionModelV2, error) {
	f.updateCalls++
	return nil, nil, nil
}

func (f *dupTolerantSensorAssignmentService) DeleteDeviceSensorAssignmentAsync(
	context.Context, string,
) (http.Header, error) {
	return nil, nil
}

func (f *dupTolerantSensorAssignmentService) BulkUpdateDeviceSensorAssignmentRankingsAsync(
	context.Context, string, *[]sdk.DeviceSensorAssignmentRankingV1ModelV2, *sdk.DeviceSensorsV2BulkUpdateDeviceSensorAssignmentRankingsAsyncOptions,
) (http.Header, error) {
	return nil, nil
}

// TestReconcileAssignments_DuplicateDesiredEntriesAreNotRejected covers b16
// decision table row #187 (REMOVE per faithful doctrine). Canonical Q26: no
// uniqueness check in AssignmentGroup_Save; no server rule found for a
// duplicate assignment name or natural key per sensor. Two desired
// entries sharing the same natural key (no assignment_uuid, same name) must
// no longer be rejected client-side as "Duplicate assignment"; each is now
// passed straight through — the first is created, and the second (sharing
// the same natural key, now resolved to that just-created UUID) is treated
// as a matched update rather than being dropped or erroring — exactly the
// no-uniqueness-rule behavior this row's removal restores.
func TestReconcileAssignments_DuplicateDesiredEntriesAreNotRejected(t *testing.T) {
	dup := tf.SensorAssignmentEntryModel{
		Name:            types.StringValue("same-name"),
		AssignmentUUID:  types.StringNull(),
		Ranking:         types.Int64Null(),
		SmartGroupUUIDs: types.ListNull(types.StringType),
		TriggerType:     types.StringNull(),
		EventTriggers:   types.ListNull(types.StringType),
	}
	desired := []tf.SensorAssignmentEntryModel{dup, dup}

	svc := &dupTolerantSensorAssignmentService{}
	diags := reconcileAssignments(context.Background(), svc, "sensor-uuid", desired)
	if diags.HasError() {
		t.Fatalf("expected no diagnostics for duplicate desired entries, got: %v", diags)
	}
	if svc.addCalls != 1 {
		t.Fatalf("expected the first duplicate to be created (1 Add call), got %d", svc.addCalls)
	}
	if svc.updateCalls != 1 {
		t.Fatalf("expected the second duplicate to reach the update path rather than being dropped or erroring, got %d update calls", svc.updateCalls)
	}
}
