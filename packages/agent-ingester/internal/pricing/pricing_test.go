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
			wantCost:   0.05625,
			wantHasVal: true,
		},
		{
			name:  "case insensitivity and whitespace trim",
			model: "  CLAUDE-OPUS-5  ",
			usage: Usage{
				Input:      1000,
				Output:     200,
				CacheRead:  5000,
				CacheWrite: 1000,
				Reasoning:  100,
			},
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
			wantCost:   0.008,
			wantHasVal: true,
		},
		{
			name:  "claude-3-7-sonnet prefix match for date suffixed key in map",
			model: "claude-3-7-sonnet",
			usage: Usage{
				Input:  1000,
				Output: 200,
			},
			wantCost:   0.006,
			wantHasVal: true,
		},
		{
			name:  "empty model returns false",
			model: "",
			usage: Usage{
				Input: 1000,
			},
			wantCost:   0,
			wantHasVal: false,
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

func TestPricingEngine_LoadJSONError(t *testing.T) {
	e := &Engine{
		prices: make(map[string]ModelPricing),
	}
	err := e.loadJSON([]byte(`invalid-json`))
	if err == nil {
		t.Errorf("expected error on invalid json, got nil")
	}
}
