package pricing

import (
	"testing"
)

func TestCalculateCost(t *testing.T) {
	tests := []struct {
		name       string
		model      string
		usage      Usage
		wantCost   float64
		wantHasVal bool
	}{
		{
			name:  "claude-opus-5 standard turn",
			model: "claude-opus-5",
			usage: Usage{
				Input:      1000,
				Output:     200,
				CacheRead:  5000,
				CacheWrite: 1000,
				Reasoning:  100,
			},
			// (1000 * 0.000015) + (200 * 0.000075) + (5000 * 0.0000015) + (1000 * 0.00001875)
			// = 0.015 + 0.015 + 0.0075 + 0.01875 = 0.05625
			wantCost:   0.05625,
			wantHasVal: true,
		},
		{
			name:  "claude-haiku-4-5 date-suffixed match",
			model: "claude-haiku-4-5-20251001",
			usage: Usage{
				Input:      2000,
				Output:     500,
				CacheRead:  10000,
				CacheWrite: 2000,
			},
			// (2000 * 1e-6) + (500 * 5e-6) + (10000 * 1e-7) + (2000 * 1.25e-6)
			// = 0.002 + 0.0025 + 0.001 + 0.0025 = 0.008
			wantCost:   0.008,
			wantHasVal: true,
		},
		{
			name:  "unrecognized model returns hasValue false",
			model: "custom-local-model",
			usage: Usage{
				Input:  1000,
				Output: 500,
			},
			wantCost:   0,
			wantHasVal: false,
		},
	}

	p, err := NewPricingEngine()
	if err != nil {
		t.Fatalf("NewPricingEngine() error = %v", err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCost, gotHasVal := p.CalculateCost(tt.model, tt.usage)
			if gotHasVal != tt.wantHasVal {
				t.Fatalf("CalculateCost() gotHasVal = %v, want %v", gotHasVal, tt.wantHasVal)
			}
			if tt.wantHasVal && (gotCost-tt.wantCost > 1e-9 || tt.wantCost-gotCost > 1e-9) {
				t.Fatalf("CalculateCost() gotCost = %f, want %f", gotCost, tt.wantCost)
			}
		})
	}
}
