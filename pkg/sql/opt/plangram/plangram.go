// Copyright 2026 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package plangram

import (
	"github.com/cockroachdb/cockroach/pkg/sql/opt/memo"
	"github.com/cockroachdb/cockroach/pkg/sql/opt/props/physical"
)

// VisibleToPlanGram reports whether expr corresponds to a node in the PlanGram.
// Invisible expressions (e.g. Distribute, Barrier) are "seen through" by the
// grammar. They must be unary so the requirement passes straight to the child.
func VisibleToPlanGram(expr memo.RelExpr) bool {
	switch expr.(type) {
	case *memo.NormCycleTestRelExpr, *memo.MemoCycleTestRelExpr, *memo.BarrierExpr,
		*memo.DistributeExpr, *memo.ExplainExpr:
		return false
	default:
		return true
	}
}

// BuildChildRequired returns the PlanGram requirement for the nth child of
// parent. Invisible parents pass their requirement through unchanged. Visible
// parents that match the grammar descend into the nth child term. Visible
// parents that don't match propagate NonePlanGram, which penalizes the entire
// subtree below the mismatch.
func BuildChildRequired(
	parent memo.RelExpr, required physical.PlanGram, childIdx int,
) physical.PlanGram {
	if !VisibleToPlanGram(parent) {
		return required
	}
	if !required.Matches(parent) {
		return physical.NonePlanGram
	}
	return required.Child(childIdx)
}

// CanProvide reports whether e can satisfy the PlanGram requirement. Invisible
// expressions always can. For visible expressions, the PlanGram must point to a
// concrete expression (not a production with unexpanded alternates) because the
// coster can only match against a PlanGram expression, not a production.
// Alternate expansion happens in enforceProps before costing.
func CanProvide(e memo.RelExpr, required physical.PlanGram) bool {
	if !VisibleToPlanGram(e) {
		return true
	}
	return !required.HasAlternates()
}
