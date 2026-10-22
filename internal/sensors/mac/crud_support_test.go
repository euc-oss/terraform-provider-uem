package macsensor

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
)

type mockDeviceSensorsV2Service struct {
	createHeaders http.Header
	createBody    *sdk.BaseModelV2
	createErr     error

	fetchSensor *sdk.DeviceSensorResponseV2Model
	fetchErr    error

	updateErr error

	createReq  *sdk.DeviceSensorRequestV2Model
	fetchUUID  string
	updateUUID string
	updateReq  *sdk.DeviceSensorUpdateV2Model
}

func (m *mockDeviceSensorsV2Service) CreateDeviceSensorAsync(ctx context.Context, request *sdk.DeviceSensorRequestV2Model) (http.Header, *sdk.BaseModelV2, error) {
	m.createReq = request
	if m.createHeaders == nil {
		m.createHeaders = http.Header{}
	}
	return m.createHeaders, m.createBody, m.createErr
}

func (m *mockDeviceSensorsV2Service) GetDeviceSensorAsync(ctx context.Context, sensorUUID string) (http.Header, *sdk.DeviceSensorResponseV2Model, error) {
	m.fetchUUID = sensorUUID
	return http.Header{}, m.fetchSensor, m.fetchErr
}

func (m *mockDeviceSensorsV2Service) UpdateDeviceSensorAsync(ctx context.Context, sensorUUID string, request *sdk.DeviceSensorUpdateV2Model) (http.Header, error) {
	m.updateUUID = sensorUUID
	m.updateReq = request
	return http.Header{}, m.updateErr
}

func newResourceWithMockService(svc DeviceSensorsV2API) *macsensorResource {
	return &macsensorResource{
		client: &sdk.Client{},
		newDeviceSensorsV2API: func(c *sdk.Client) DeviceSensorsV2API {
			return svc
		},
	}
}

func TestCreateMacSensor_Success(t *testing.T) {
	svc := &mockDeviceSensorsV2Service{createHeaders: http.Header{"Location": []string{"/api/mdm/devicesensors/9af645a8-fef3-3e6d-3408-5cc69e0937d4"}}}
	r := newResourceWithMockService(svc)

	req := &sdk.DeviceSensorRequestV2Model{Name: "sensor"}
	got, err := r.createMacSensor(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "9af645a8-fef3-3e6d-3408-5cc69e0937d4" {
		t.Fatalf("sensor UUID mismatch: got %q", got)
	}
	if svc.createReq != req {
		t.Fatalf("create request mismatch: got %p want %p", svc.createReq, req)
	}
}

func TestCreateMacSensor_Errors(t *testing.T) {
	t.Run("create API fails", func(t *testing.T) {
		svc := &mockDeviceSensorsV2Service{createErr: errors.New("create failed")}
		r := newResourceWithMockService(svc)

		_, err := r.createMacSensor(context.Background(), &sdk.DeviceSensorRequestV2Model{})
		if err == nil || !strings.Contains(err.Error(), "unable to create mac sensor") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("location parse fails", func(t *testing.T) {
		svc := &mockDeviceSensorsV2Service{createHeaders: http.Header{"Location": []string{""}}}
		r := newResourceWithMockService(svc)

		_, err := r.createMacSensor(context.Background(), &sdk.DeviceSensorRequestV2Model{})
		if err == nil || !strings.Contains(err.Error(), "unable to parse sensor UUID") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestFetchMacSensorDetails(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		want := &sdk.DeviceSensorResponseV2Model{UUID: "id-1", Name: "n"}
		svc := &mockDeviceSensorsV2Service{fetchSensor: want}
		r := newResourceWithMockService(svc)

		got, err := r.fetchMacSensorDetails(context.Background(), "id-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != want {
			t.Fatalf("fetch response mismatch: got %p want %p", got, want)
		}
		if svc.fetchUUID != "id-1" {
			t.Fatalf("expected fetch UUID id-1, got %q", svc.fetchUUID)
		}
	})

	t.Run("api error", func(t *testing.T) {
		svc := &mockDeviceSensorsV2Service{fetchErr: errors.New("boom")}
		r := newResourceWithMockService(svc)

		_, err := r.fetchMacSensorDetails(context.Background(), "id-2")
		if err == nil || !strings.Contains(err.Error(), "unable to fetch mac sensor id-2") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestUpdateMacSensor(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		svc := &mockDeviceSensorsV2Service{}
		r := newResourceWithMockService(svc)
		req := &sdk.DeviceSensorUpdateV2Model{Description: "updated"}

		err := r.updateMacSensor(context.Background(), "id-1", req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if svc.updateUUID != "id-1" || svc.updateReq != req {
			t.Fatalf("update args mismatch: uuid=%q req=%p", svc.updateUUID, svc.updateReq)
		}
	})
}
