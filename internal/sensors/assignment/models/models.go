package models

import (
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

type SensorAssignmentModel struct {
	ID          types.String `tfsdk:"id"`
	SensorUUID  types.String `tfsdk:"sensor_uuid"`
	Assignments types.List   `tfsdk:"assignments"`
}

func ValidateSensorUUID(sensorUUID string) (string, error) {
	value := strings.TrimSpace(sensorUUID)
	if value == "" {
		return "", fmt.Errorf("sensor_uuid must not be empty")
	}
	return value, nil
}

func (m SensorAssignmentModel) FetchValidSensorUUID() (string, error) {
	if m.SensorUUID.IsNull() || m.SensorUUID.IsUnknown() {
		return "", fmt.Errorf("sensor_uuid must be set and known")
	}
	return ValidateSensorUUID(m.SensorUUID.ValueString())
}

type SensorAssignmentEntryModel struct {
	AssignmentUUID  types.String `tfsdk:"assignment_uuid"`
	Name            types.String `tfsdk:"name"`
	Ranking         types.Int64  `tfsdk:"ranking"`
	SmartGroupUUIDs types.List   `tfsdk:"smart_group_uuids"`
	TriggerType     types.String `tfsdk:"trigger_type"`
	EventTriggers   types.List   `tfsdk:"event_triggers"`
}
