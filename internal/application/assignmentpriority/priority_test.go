package assignmentpriority

import (
	"reflect"
	"testing"
)

// TestCheck exercises the deterministic pointing rule for all combinations
// requested by internal-task.
func TestCheck(t *testing.T) {
	tests := []struct {
		name       string
		priorities []int64
		want       []Finding
	}{
		{"empty", nil, nil},
		{"single zero", []int64{0}, nil},
		{"single out of range", []int64{1}, []Finding{{Index: 0, Value: 1, Kind: KindGap}}},
		{"ascending in order", []int64{0, 1, 2}, nil},
		{"gap above range", []int64{0, 2}, []Finding{{Index: 1, Value: 2, Kind: KindGap}}},
		{"duplicate zero", []int64{0, 0}, []Finding{{Index: 1, Value: 0, Kind: KindDuplicate}}},
		{"valid but out of order", []int64{1, 0}, []Finding{{Index: 0, Value: 1, Kind: KindOutOfOrder}}},
		{"valid permutation out of order", []int64{2, 0, 1}, []Finding{{Index: 0, Value: 2, Kind: KindOutOfOrder}}},
		{"duplicate one, missing two", []int64{0, 1, 1}, []Finding{{Index: 2, Value: 1, Kind: KindDuplicate}}},
		{"negative value", []int64{-1, 0}, []Finding{{Index: 0, Value: -1, Kind: KindGap}}},
		{"value beyond range", []int64{0, 1, 3}, []Finding{{Index: 2, Value: 3, Kind: KindGap}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Check(tt.priorities)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Check(%v) = %#v, want %#v", tt.priorities, got, tt.want)
			}
		})
	}
}

// TestCheckMultipleGapsReportedInOrder proves every out-of-range value is
// reported, not just the first, and in list order.
func TestCheckMultipleGapsReportedInOrder(t *testing.T) {
	got := Check([]int64{1, 5, 7})
	want := []Finding{
		{Index: 1, Value: 5, Kind: KindGap},
		{Index: 2, Value: 7, Kind: KindGap},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Check = %#v, want %#v", got, want)
	}
}

// TestCheckGapsTakePrecedenceOverDuplicates proves that when both an
// out-of-range value and an in-range duplicate are present, only the gap
// finding(s) are reported.
func TestCheckGapsTakePrecedenceOverDuplicates(t *testing.T) {
	got := Check([]int64{0, 0, 5})
	want := []Finding{{Index: 2, Value: 5, Kind: KindGap}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Check = %#v, want %#v", got, want)
	}
}

func TestDiagnosticText(t *testing.T) {
	t.Run("gap", func(t *testing.T) {
		summary, detail := Diagnostic(Finding{Index: 2, Value: 3, Kind: KindGap}, 3)
		if summary != "Invalid assignments[2].priority" {
			t.Fatalf("summary = %q", summary)
		}
		want := `assignments[2].priority is 3; UEM requires the priorities of the 3 assignments to be exactly 0 to 2, ` +
			`with no gaps or duplicates (UEM rejects otherwise: "Priority of the assignments should be in ascending order with priority starting from 0.")`
		if detail != want {
			t.Fatalf("detail = %q, want %q", detail, want)
		}
	})

	t.Run("duplicate uses the same wording as gap", func(t *testing.T) {
		_, gapDetail := Diagnostic(Finding{Index: 1, Value: 0, Kind: KindGap}, 2)
		_, dupDetail := Diagnostic(Finding{Index: 1, Value: 0, Kind: KindDuplicate}, 2)
		if gapDetail != dupDetail {
			t.Fatalf("gap detail %q != duplicate detail %q", gapDetail, dupDetail)
		}
	})

	t.Run("out of order", func(t *testing.T) {
		summary, detail := Diagnostic(Finding{Index: 0, Value: 1, Kind: KindOutOfOrder}, 2)
		if summary != "assignments[0].priority out of order" {
			t.Fatalf("summary = %q", summary)
		}
		want := "assignments[0].priority is 1 but it is listed at position 0; list assignments in priority order (0 first). " +
			"UEM stores and returns them sorted by priority, so any other order produces an inconsistent result after apply."
		if detail != want {
			t.Fatalf("detail = %q, want %q", detail, want)
		}
	})
}
