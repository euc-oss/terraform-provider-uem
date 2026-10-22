package models

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestFetchValidOrganizationGroupUUID(t *testing.T) {
	tests := []struct {
		name    string
		model   MacScriptResourceModel
		want    string
		wantErr bool
	}{
		{
			name: "valid and trimmed",
			model: MacScriptResourceModel{
				OrganizationGroupUUID: types.StringValue(" bafde89c-041e-1756-082b-933aaf16cad8 "),
			},
			want: "bafde89c-041e-1756-082b-933aaf16cad8",
		},
		{
			name: "null value",
			model: MacScriptResourceModel{
				OrganizationGroupUUID: types.StringNull(),
			},
			wantErr: true,
		},
		{
			name: "empty after trim",
			model: MacScriptResourceModel{
				OrganizationGroupUUID: types.StringValue("   "),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.model.FetchValidOrganizationGroupUUID()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (value=%q)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("value mismatch: got %q want %q", got, tt.want)
			}
		})
	}
}

func TestValidateScriptUUID(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "valid and trimmed",
			input: " 05d17100-b346-c29d-6760-a0fdedcf8623 ",
			want:  "05d17100-b346-c29d-6760-a0fdedcf8623",
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "whitespace only",
			input:   "   ",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidateScriptUUID(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (value=%q)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("value mismatch: got %q want %q", got, tt.want)
			}
		})
	}
}

func TestFetchValidScriptUUID(t *testing.T) {
	tests := []struct {
		name    string
		model   MacScriptResourceModel
		want    string
		wantErr bool
	}{
		{
			name: "valid and trimmed",
			model: MacScriptResourceModel{
				ID: types.StringValue(" 05d17100-b346-c29d-6760-a0fdedcf8623 "),
			},
			want: "05d17100-b346-c29d-6760-a0fdedcf8623",
		},
		{
			name: "unknown value",
			model: MacScriptResourceModel{
				ID: types.StringUnknown(),
			},
			wantErr: true,
		},
		{
			name: "empty string",
			model: MacScriptResourceModel{
				ID: types.StringValue(""),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.model.FetchValidScriptUUID()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (value=%q)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("value mismatch: got %q want %q", got, tt.want)
			}
		})
	}
}
