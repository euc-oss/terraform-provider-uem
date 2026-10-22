package models

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestValidateScriptUUID(t *testing.T) {
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
			got, err := ValidateScriptUUID(tt.input)
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

func TestFetchValidScriptUUID(t *testing.T) {
	tests := []struct {
		name    string
		model   ScriptAssignmentRuleModel
		want    string
		wantErr bool
	}{
		{
			name:  "valid known value",
			model: ScriptAssignmentRuleModel{ScriptUUID: types.StringValue("  script-uuid-1  ")},
			want:  "script-uuid-1",
		},
		{
			name:    "null value",
			model:   ScriptAssignmentRuleModel{ScriptUUID: types.StringNull()},
			wantErr: true,
		},
		{
			name:    "unknown value",
			model:   ScriptAssignmentRuleModel{ScriptUUID: types.StringUnknown()},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.model.FetchValidScriptUUID()
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
