package models

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestValidateAppUUID(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "valid uuid", input: "abc-123", want: "abc-123"},
		{name: "trim whitespace", input: "  abc-123  ", want: "abc-123"},
		{name: "empty", input: "", wantErr: true},
		{name: "whitespace only", input: "   ", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidateAppUUID(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("unexpected result: got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFetchValidAppUUID(t *testing.T) {
	tests := []struct {
		name    string
		model   AppAssignmentRuleModel
		want    string
		wantErr bool
	}{
		{
			name:  "valid known value",
			model: AppAssignmentRuleModel{ApplicationUUID: types.StringValue("  app-uuid-1  ")},
			want:  "app-uuid-1",
		},
		{
			name:    "null value",
			model:   AppAssignmentRuleModel{ApplicationUUID: types.StringNull()},
			wantErr: true,
		},
		{
			name:    "unknown value",
			model:   AppAssignmentRuleModel{ApplicationUUID: types.StringUnknown()},
			wantErr: true,
		},
		{
			name:    "empty string",
			model:   AppAssignmentRuleModel{ApplicationUUID: types.StringValue("   ")},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.model.FetchValidAppUUID()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("unexpected result: got %q, want %q", got, tt.want)
			}
		})
	}
}
