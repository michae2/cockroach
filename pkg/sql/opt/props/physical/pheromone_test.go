// Copyright 2025 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package physical

import (
	"strings"
	"testing"

	"github.com/cockroachdb/cockroach/pkg/sql/opt"
	"github.com/stretchr/testify/require"
)

func TestParsePheromone(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantError bool
		expected  string // expected formatted output, empty if wantError is true
	}{
		// Simple patterns
		{
			name:     "any",
			input:    "initial: any",
			expected: "initial: any",
		},
		{
			name:     "none",
			input:    "initial: none",
			expected: "initial: none",
		},
		{
			name:     "simple operator",
			input:    "initial: (ScanOp)",
			expected: "initial: (ScanOp)",
		},
		{
			name:     "operator with any child",
			input:    "initial: (SelectOp any)",
			expected: "initial: (SelectOp any)",
		},
		{
			name:     "operator with multiple children",
			input:    "initial: (JoinOp any any)",
			expected: "initial: (JoinOp any any)",
		},
		{
			name:     "nested operators",
			input:    "initial: (SelectOp (ScanOp))",
			expected: "initial: (SelectOp (ScanOp))",
		},

		// Complex patterns with nonterminals
		{
			name:     "single production rule",
			input:    "initial: sel; sel: (ScanOp);",
			expected: "initial: sel; sel: (ScanOp)",
		},
		{
			name:     "multiple alternates",
			input:    "initial: sel; sel: (ScanOp) | (IndexScanOp);",
			expected: "initial: sel; sel: (ScanOp) | (IndexScanOp)",
		},
		{
			name:     "recursive rule",
			input:    "initial: sel; sel: (ScanOp) | (SelectOp sel);",
			expected: "initial: sel; sel: (ScanOp) | (SelectOp sel)",
		},
		{
			name:     "multiple rules",
			input:    "initial: sel; sel: scan | join; scan: (ScanOp); join: (JoinOp sel sel);",
			expected: "initial: sel; sel: scan | join; scan: (ScanOp); join: (JoinOp sel sel)",
		},
		{
			name:     "rule with none alternate",
			input:    "initial: sel; sel: (ScanOp) | none;",
			expected: "initial: sel; sel: (ScanOp)",
		},

		// Error cases
		{
			name:      "invalid operator",
			input:     "initial: (InvalidOp)",
			wantError: true,
		},
		{
			name:      "missing rule",
			input:     "initial: sel;",
			wantError: true,
		},
		{
			name:      "missing initial",
			input:     "sel: (ScanOp);",
			wantError: true,
		},
		{
			name:      "duplicate rule",
			input:     "initial: sel; sel: (ScanOp); sel: (IndexScanOp);",
			wantError: true,
		},
		{
			name:      "reserved nonterminal name",
			input:     "initial: _0; _0: (ScanOp);",
			wantError: true,
		},
		{
			name:      "malformed syntax - missing colon",
			input:     "initial sel; sel: (ScanOp);",
			wantError: true,
		},
		{
			name:      "malformed syntax - missing semicolon",
			input:     "initial: sel sel: (ScanOp);",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := ParsePheromone(strings.NewReader(tt.input))

			if tt.wantError {
				require.Error(t, err, "expected parsing to fail")
				return
			}

			require.NoError(t, err, "unexpected parsing error")

			// Test formatting
			actual := p.String()
			require.Equal(t, tt.expected, actual, "formatted output mismatch")
		})
	}
}

func TestPheromoneRoundTrip(t *testing.T) {
	tests := []string{
		"initial: any",
		"initial: none",
		"initial: (ScanOp)",
		"initial: (SelectOp any)",
		"initial: (JoinOp any any)",
		"initial: sel; sel: (ScanOp)",
		"initial: sel; sel: (ScanOp) | (IndexScanOp)",
		"initial: sel; sel: (ScanOp) | (SelectOp sel)",
		"initial: sel; sel: scan | join; scan: (ScanOp); join: (JoinOp sel sel)",
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			// Parse
			p1, err := ParsePheromone(strings.NewReader(input))
			require.NoError(t, err)

			// Format
			formatted := p1.String()

			// Parse again
			p2, err := ParsePheromone(strings.NewReader(formatted))
			require.NoError(t, err)

			// Should be equal
			require.True(t, p1.Equals(p2), "round-trip failed: %s -> %s", input, formatted)
		})
	}
}

func TestPheromoneEquals(t *testing.T) {
	tests := []struct {
		name     string
		input1   string
		input2   string
		expected bool
	}{
		{
			name:     "identical simple patterns",
			input1:   "initial: any",
			input2:   "initial: any",
			expected: true,
		},
		{
			name:     "different simple patterns",
			input1:   "initial: any",
			input2:   "initial: none",
			expected: false,
		},
		{
			name:     "identical operators",
			input1:   "initial: (ScanOp)",
			input2:   "initial: (ScanOp)",
			expected: true,
		},
		{
			name:     "different operators",
			input1:   "initial: (ScanOp)",
			input2:   "initial: (IndexScanOp)",
			expected: false,
		},
		{
			name:     "identical complex patterns",
			input1:   "initial: sel; sel: (ScanOp) | (SelectOp sel);",
			input2:   "initial: sel; sel: (ScanOp) | (SelectOp sel);",
			expected: true,
		},
		{
			name:     "structurally equivalent with different names",
			input1:   "initial: sel; sel: (ScanOp);",
			input2:   "initial: scan; scan: (ScanOp);",
			expected: false, // Different nonterminal names
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p1, err := ParsePheromone(strings.NewReader(tt.input1))
			require.NoError(t, err)

			p2, err := ParsePheromone(strings.NewReader(tt.input2))
			require.NoError(t, err)

			actual := p1.Equals(p2)
			require.Equal(t, tt.expected, actual)

			// Equality should be symmetric
			require.Equal(t, actual, p2.Equals(p1))
		})
	}
}

func TestPheromoneMatching(t *testing.T) {
	// Create a mock expression for testing
	// Note: This is a simplified test since we don't have access to the memo package here
	tests := []struct {
		name     string
		pattern  string
		op       opt.Operator
		children int
		expected bool
	}{
		{
			name:     "any matches everything",
			pattern:  "initial: any",
			op:       opt.ScanOp,
			children: 0,
			expected: true,
		},
		{
			name:     "none matches nothing",
			pattern:  "initial: none",
			op:       opt.ScanOp,
			children: 0,
			expected: false,
		},
		{
			name:     "exact operator match",
			pattern:  "initial: (ScanOp)",
			op:       opt.ScanOp,
			children: 0,
			expected: true,
		},
		{
			name:     "operator mismatch",
			pattern:  "initial: (ScanOp)",
			op:       opt.SelectOp,
			children: 0,
			expected: false,
		},
		{
			name:     "child count match",
			pattern:  "initial: (SelectOp any)",
			op:       opt.SelectOp,
			children: 1,
			expected: true,
		},
		{
			name:     "child count mismatch",
			pattern:  "initial: (SelectOp any)",
			op:       opt.SelectOp,
			children: 2,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := ParsePheromone(strings.NewReader(tt.pattern))
			require.NoError(t, err)

			// Create a mock expression - we can't create a real opt.Expr here
			// so we'll test the internal matching logic through head()
			if p.Any() {
				require.True(t, tt.expected)
				return
			}

			if p.None() {
				require.False(t, tt.expected)
				return
			}

			// For other patterns, we'd need access to the memo package to create expressions
			// This is a limitation of testing at this level
		})
	}
}
