package assignmentpriority

import "fmt"

// Diagnostic builds the summary and detail text for f, given n — the total
// number of assignments the finding was computed against. Both resources
// call this so the same problem produces byte-identical diagnostics
// regardless of which resource type is being validated.
func Diagnostic(f Finding, n int) (summary, detail string) {
	if f.Kind == KindOutOfOrder {
		summary = fmt.Sprintf("assignments[%d].priority out of order", f.Index)
		detail = fmt.Sprintf(
			"assignments[%d].priority is %d but it is listed at position %d; list assignments in priority order (0 first). "+
				"UEM stores and returns them sorted by priority, so any other order produces an inconsistent result after apply.",
			f.Index, f.Value, f.Index,
		)
		return summary, detail
	}

	// KindGap and KindDuplicate share the same wording: both mean the
	// multiset of priorities is not exactly {0, ..., n-1}.
	summary = fmt.Sprintf("Invalid assignments[%d].priority", f.Index)
	detail = fmt.Sprintf(
		"assignments[%d].priority is %d; UEM requires the priorities of the %d assignments to be exactly 0 to %d, "+
			"with no gaps or duplicates (UEM rejects otherwise: \"Priority of the assignments should be in ascending order with priority starting from 0.\")",
		f.Index, f.Value, n, n-1,
	)
	return summary, detail
}
