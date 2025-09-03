// Copyright 2025 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package physical

import (
	"bytes"
	"strconv"

	"github.com/cockroachdb/cockroach/pkg/sql/opt"
)

type Pheromone struct {
	initial pheromoneTerm
}

// pheromoneTerm represents a term in the grammar, which could be either a
// nonterminal (*pheromoneProduction) or a terminal (*pheromoneExpr).
type pheromoneTerm interface {
	gatherTopExprs(visited map[pheromoneTerm]struct{}) []pheromoneTerm
	gatherAllRules(visited map[pheromoneTerm]struct{}) []pheromoneTerm
}

// pheromoneProduction represents a nonterminal and its associated production
// rule in the grammar.
type pheromoneProduction struct {
	name       string
	alternates []pheromoneTerm
}

// pheromoneExpr represents a terminal in the grammar matching an opt.Expr in the memo.
type pheromoneExpr struct {
	op       opt.Operator
	children []pheromoneTerm
}

var _ pheromoneTerm = &pheromoneProduction{}
var _ pheromoneTerm = &pheromoneExpr{}

var anyPheromoneTerm pheromoneTerm
var nonePheromoneTerm pheromoneTerm = &pheromoneProduction{name: "none"}
var AnyPheromone = Pheromone{anyPheromoneTerm}
var NonePheromone = Pheromone{nonePheromoneTerm}

func (p Pheromone) Any() bool {
	return p.initial == nil
}

// head returns the first terminal reachable from the initial term, or
// nonePheromoneTerm if no terminals are reachable from the initial term.
func (p Pheromone) head() pheromoneTerm {
	if p.initial == nil {
		return anyPheromoneTerm
	}
	if p.initial == nonePheromoneTerm {
		return nonePheromoneTerm
	}

	// Handle common cases.
	switch t := p.initial.(type) {
	case *pheromoneProduction:
		if len(t.alternates) == 0 {
			return nonePheromoneTerm
		}
		if pe, ok := t.alternates[0].(*pheromoneExpr); ok {
			return pe
		}
		// We have production rules referencing other production rules, so now we
		// need the more heavyweight graph search to handle cycles.
		visited := make(map[pheromoneTerm]struct{})
		exprs := t.gatherTopExprs(visited)
		if len(exprs) == 0 {
			return nonePheromoneTerm
		}
		return exprs[0]
	case *pheromoneExpr:
		return p.initial
	default:
		return nonePheromoneTerm
	}
}

// Tail returns a copy of the pheromone without the first terminal reachable
// from the initial term, or NonePheromone if no terminals are reachable from
// the initial term.
func (p Pheromone) Tail() Pheromone {
	if p.initial == nil {
		return AnyPheromone
	}
	if p.initial == nonePheromoneTerm {
		return NonePheromone
	}

	// Handle common cases.
	switch t := p.initial.(type) {
	case *pheromoneProduction:
		if len(t.alternates) == 0 {
			return NonePheromone
		}
		if _, ok := t.alternates[0].(*pheromoneExpr); ok {
			// Allocate a new initial production rule without the first terminal.
			return Pheromone{&pheromoneProduction{
				alternates: t.alternates[1:],
			}}
		}
		// We have production rules referencing other production rules, so now we
		// need to handle cycles. We'll allocate a new initial production rule with
		// all reachable terminals except the first, rather than trying to mutate
		// the graph.
		visited := make(map[pheromoneTerm]struct{})
		exprs := t.gatherTopExprs(visited)
		if len(exprs) == 0 {
			return NonePheromone
		}
		// Allocate a new initial production rule without the first terminal.
		return Pheromone{&pheromoneProduction{
			alternates: exprs[1:],
		}}
	case *pheromoneExpr:
		return NonePheromone
	default:
		return NonePheromone
	}
}

func (pp *pheromoneProduction) gatherTopExprs(visited map[pheromoneTerm]struct{}) []pheromoneTerm {
	if _, ok := visited[pp]; ok {
		return nil
	}
	visited[pp] = struct{}{}

	exprs := make([]pheromoneTerm, 0, len(pp.alternates))
	for _, alt := range pp.alternates {
		if alt == nil {
			exprs = append(exprs, nil)
		} else if alt != nonePheromoneTerm {
			exprs = append(exprs, alt.gatherTopExprs(visited)...)
		}
	}
	return exprs
}

func (pe *pheromoneExpr) gatherTopExprs(visited map[pheromoneTerm]struct{}) []pheromoneTerm {
	return []pheromoneTerm{pe}
}

func (pp *pheromoneProduction) gatherAllRules(visited map[pheromoneTerm]struct{}) []pheromoneTerm {
	if _, ok := visited[pp]; ok {
		return nil
	}
	visited[pp] = struct{}{}

	rules := make([]pheromoneTerm, 0, len(pp.alternates)+1)
	rules = append(rules, pp)
	for _, alt := range pp.alternates {
		if alt != nil && alt != nonePheromoneTerm {
			rules = append(rules, alt.gatherAllRules(visited)...)
		}
	}
	return rules
}

func (pe *pheromoneExpr) gatherAllRules(visited map[pheromoneTerm]struct{}) []pheromoneTerm {
	if _, ok := visited[pe]; ok {
		return nil
	}
	visited[pe] = struct{}{}

	rules := make([]pheromoneTerm, 0, len(pe.children))
	for _, child := range pe.children {
		if child != nil && child != nonePheromoneTerm {
			rules = append(rules, child.gatherAllRules(visited)...)
		}
	}
	return rules
}

func (p Pheromone) None() bool {
	term := p.head()
	if term == nil {
		return false
	}
	if _, ok := term.(*pheromoneExpr); ok {
		return false
	}
	return true
}

// Matches checks whether the first terminal reachable from the initial term
// matches this opt.Expr.
func (p Pheromone) Matches(e opt.Expr) (match bool) {
	term := p.head()
	if term == nil {
		return true
	}
	if pe, ok := term.(*pheromoneExpr); ok {
		return len(pe.children) == e.ChildCount() && (pe.op == opt.UnknownOp || pe.op == e.Op())
	}
	return false
}

// Child returns the pheromone for the nth child from the first terminal
// reachable from the initial term.
func (p Pheromone) Child(nth int) Pheromone {
	term := p.head()
	if term == nil {
		return AnyPheromone
	}
	if pe, ok := term.(*pheromoneExpr); ok {
		if len(pe.children) >= nth {
			return Pheromone{pe.children[nth]}
		}
	}
	return NonePheromone
}

func (p Pheromone) Format(b *bytes.Buffer) {
	if p.initial == nil {
		b.WriteString("initial: any")
		return
	}
	if p.initial == nonePheromoneTerm {
		b.WriteString("initial: none")
		return
	}

	visited := make(map[pheromoneTerm]struct{})
	rules := p.initial.gatherAllRules(visited)
	names := make(map[*pheromoneProduction]string, len(rules))
	for i, rule := range rules {
		pp := rule.(*pheromoneProduction)
		if pp.name == "" {
			names[pp] = "p" + strconv.Itoa(i)
		} else {
			names[pp] = pp.name
		}
	}

	var format func(pheromoneTerm)
	format = func(term pheromoneTerm) {
		if term == nil {
			b.WriteString("any")
			return
		}
		if term == nonePheromoneTerm {
			b.WriteString("none")
			return
		}
		switch t := term.(type) {
		case *pheromoneProduction:
			b.WriteString(names[t])
		case *pheromoneExpr:
			b.WriteRune('(')
			b.WriteString(t.op.String())
			for _, child := range t.children {
				b.WriteRune(' ')
				format(child)
			}
			b.WriteRune(')')
		}
	}

	b.WriteString("initial: ")
	format(p.initial)
	for _, rule := range rules {
		pp := rule.(*pheromoneProduction)
		b.WriteString("; ")
		b.WriteString(names[pp])
		if len(pp.alternates) == 0 {
			b.WriteString(": none")
		} else {
			for i, alt := range pp.alternates {
				if i == 0 {
					b.WriteString(": ")
				} else {
					b.WriteString(" | ")
				}
				format(alt)
			}
		}
	}
}

func (p Pheromone) String() string {
	var b bytes.Buffer
	p.Format(&b)
	return b.String()
}

func (p Pheromone) Equals(other Pheromone) bool {
	if p.initial == other.initial {
		return true
	}
	if p.initial == nil || other.initial == nil {
		return false
	}

	// Handle common cases.
	switch t1 := p.initial.(type) {
	case *pheromoneProduction:
		t2, ok := other.initial.(*pheromoneProduction)
		if !ok {
			return false
		}
		if t1.name == t2.name && len(t1.alternates) == 0 && len(t2.alternates) == 0 {
			return true
		}
	case *pheromoneExpr:
		t2, ok := other.initial.(*pheromoneExpr)
		if !ok {
			return false
		}
		if t1.op == t2.op || len(t1.children) == 0 && len(t2.children) == 0 {
			return true
		}
	default:
		return false
	}

	// The grammar is more complicated, so now we need the more heavyweight graph
	// search to handle cycles and memoization.
	equal := make(map[struct{ a, b pheromoneTerm }]struct{})
	var equals func(a, b pheromoneTerm) bool
	equals = func(a, b pheromoneTerm) bool {
		if a == b {
			return true
		}
		if a == nil || b == nil {
			return false
		}
		ab := struct{ a, b pheromoneTerm }{a, b}
		if _, ok := equal[ab]; ok {
			return true
		}
		equal[ab] = struct{}{}
		switch t1 := a.(type) {
		case *pheromoneProduction:
			t2, ok := b.(*pheromoneProduction)
			if !ok {
				return false
			}
			if t1.name != t2.name || len(t1.alternates) != len(t2.alternates) {
				return false
			}
			for i := range t1.alternates {
				if !equals(t1.alternates[i], t2.alternates[i]) {
					return false
				}
			}
			return true
		case *pheromoneExpr:
			t2, ok := b.(*pheromoneExpr)
			if !ok {
				return false
			}
			if t1.op != t2.op || len(t1.children) != len(t2.children) {
				return false
			}
			for i := range t1.children {
				if !equals(t1.children[i], t2.children[i]) {
					return false
				}
			}
			return true
		default:
			return false
		}
	}

	return equals(p.initial, other.initial)
}
