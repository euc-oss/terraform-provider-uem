// Package assignmentpriority implements the pure priority-ordering check
// shared by uem_application_assignment and uem_purchased_application_assignment
// ValidateConfig.
//
// UEM's VPP assignment-rules PUT requires the priorities of an assignment
// rule's assignments to be exactly the set {0, ..., n-1} (no gaps, no
// duplicates, starting at 0) — live-verified: [0, 2] and [0, 0] are both
// rejected with 400 "Priority of the assignments should be in ascending
// order with priority starting from 0.", while [1, 0] is accepted. But when
// UEM accepts a valid-but-out-of-order set like [1, 0], its GET returns the
// assignments re-sorted by priority, and both provider resources pair that
// readback with the configured assignments by list index rather than by
// priority (see each resource's state.go), so an out-of-order-but-valid
// config applies successfully and then fails with "Provider produced
// inconsistent result after apply". Check reports both problems at
// plan/validate time so neither surfaces only at apply time.
package assignmentpriority

import "github.com/hashicorp/terraform-plugin-framework/types"

// Kind identifies which rule a Finding violates.
type Kind int

const (
	// KindGap marks a priority value outside the valid range [0, n-1] for n
	// assignments, including a negative value. A single such value already
	// proves the multiset of priorities cannot be exactly {0, ..., n-1}.
	KindGap Kind = iota
	// KindDuplicate marks a repeated priority value that is otherwise
	// within [0, n-1]. Reported only when no KindGap findings exist; see
	// Check's doc comment for the precedence rule.
	KindDuplicate
	// KindOutOfOrder marks the first assignment whose priority does not
	// equal its list index, once the priority set has already been proven
	// to be exactly {0, ..., n-1}.
	KindOutOfOrder
)

// Finding is one problem found in a priority list, identified by the index
// of the offending assignment (its 0-based position in the list) and its
// configured priority value.
type Finding struct {
	Index int
	Value int64
	Kind  Kind
}

// Check validates a list of assignment priorities against UEM's requirement
// that the SET of priorities be exactly {0, ..., n-1} (rule (a) — KindGap /
// KindDuplicate), and, separately, that the LIST is already given in
// ascending priority order (rule (b) — KindOutOfOrder). Rule (b) exists
// because both provider resources pair the UEM readback with the configured
// assignments by list index rather than by priority, so a valid-but-out-of-
// order config applies and then reads back re-sorted by UEM, producing an
// inconsistent-result-after-apply error.
//
// Pointing rule (deterministic; the first non-empty step below wins and is
// returned — later steps never run once an earlier one finds something):
//  1. Any priority outside [0, n-1] (including negative values) is reported,
//     one Finding per offending element, in list order. Such a value alone
//     proves the set can't be {0, ..., n-1}, so this takes precedence.
//  2. Else, if the set still isn't exactly {0, ..., n-1} — only possible via
//     a duplicate, since every value is already known to be in range and by
//     pigeonhole a duplicate implies some other in-range value is missing —
//     each repeated value is reported at its second and later occurrences,
//     in list order. The duplicate finding alone is enough to point at the
//     problem: the missing value has no element of its own to point at.
//  3. Else the set is exactly {0, ..., n-1}; if the list isn't already
//     sorted by priority, the first index where priority != index is
//     reported (only one KindOutOfOrder finding is ever returned).
//  4. Else priorities are exactly 0..n-1 in order: no findings.
//
// Possible later improvement: if Read paired the UEM readback with the
// configured assignments by priority instead of by list index, rule (b)
// (KindOutOfOrder) could be dropped entirely, since a valid-but-out-of-order
// config would then read back consistently.
func Check(priorities []int64) []Finding {
	n := int64(len(priorities))

	var gaps []Finding
	for i, v := range priorities {
		if v < 0 || v >= n {
			gaps = append(gaps, Finding{Index: i, Value: v, Kind: KindGap})
		}
	}
	if len(gaps) > 0 {
		return gaps
	}

	seen := make(map[int64]bool, len(priorities))
	var dups []Finding
	for i, v := range priorities {
		if seen[v] {
			dups = append(dups, Finding{Index: i, Value: v, Kind: KindDuplicate})
			continue
		}
		seen[v] = true
	}
	if len(dups) > 0 {
		return dups
	}

	for i, v := range priorities {
		if v != int64(i) {
			return []Finding{{Index: i, Value: v, Kind: KindOutOfOrder}}
		}
	}
	return nil
}

// CollectPriorities reads assignments[].priority off a generic types.List of
// types.Object assignments — shared by uem_application_assignment's and
// uem_purchased_application_assignment's ValidateConfig, which otherwise
// each had their own byte-identical copy of this walk
// (collectAssignmentPriorities / collectPurchasedAssignmentPriorities).
//
// It returns ok=false — meaning "skip the priority check entirely" — if the
// list contains an unknown or null assignment, or an unknown priority
// (priority is Required on both resources so a null shouldn't occur, but it
// is treated the same as unknown defensively rather than assumed
// impossible).
func CollectPriorities(assignments types.List) (priorities []int64, ok bool) {
	elements := assignments.Elements()
	priorities = make([]int64, 0, len(elements))
	for _, element := range elements {
		assignment, isObject := element.(types.Object)
		if !isObject || assignment.IsNull() || assignment.IsUnknown() {
			return nil, false
		}
		priority, isInt64 := assignment.Attributes()["priority"].(types.Int64)
		if !isInt64 || priority.IsNull() || priority.IsUnknown() {
			return nil, false
		}
		priorities = append(priorities, priority.ValueInt64())
	}
	return priorities, true
}
