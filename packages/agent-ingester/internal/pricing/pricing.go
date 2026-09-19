package pricing

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

//go:embed models.json
var defaultModelsJSON []byte

type ModelPricing struct {
	InputCostPerToken            float64 `json:"input_cost_per_token"`
	OutputCostPerToken           float64 `json:"output_cost_per_token"`
	CacheReadInputTokenCost      float64 `json:"cache_read_input_token_cost"`
	CacheCreationInputTokenCost  float64 `json:"cache_creation_input_token_cost"`
}

type Usage struct {
	Input      int64
	Output     int64
	CacheRead  int64
	CacheWrite int64
	Reasoning  int64
}

type Engine struct {
	mu     sync.RWMutex
	prices map[string]ModelPricing
}

func NewPricingEngine() (*Engine, error) {
	e := &Engine{
		prices: make(map[string]ModelPricing),
	}
	if err := e.loadJSON(defaultModelsJSON); err != nil {
		return nil, fmt.Errorf("loading default models pricing: %w", err)
	}
	return e, nil
}

func (e *Engine) loadJSON(data []byte) error {
	var m map[string]ModelPricing
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for k, v := range m {
		e.prices[strings.ToLower(k)] = v
	}
	return nil
}

func (e *Engine) CalculateCost(model string, u Usage) (float64, bool) {
	if model == "" {
		return 0, false
	}
	normalized := strings.ToLower(strings.TrimSpace(model))

	e.mu.RLock()
	defer e.mu.RUnlock()

	pricing, ok := e.prices[normalized]
	if !ok {
		// Try prefix match (e.g. model without date suffix)
		for k, v := range e.prices {
			if strings.HasPrefix(normalized, k) || strings.HasPrefix(k, normalized) {
				pricing = v
				ok = true
				break
			}
		}
	}
	if !ok {
		return 0, false
	}

	cost := (float64(u.Input) * pricing.InputCostPerToken) +
		(float64(u.Output) * pricing.OutputCostPerToken) +
		(float64(u.CacheRead) * pricing.CacheReadInputTokenCost) +
		(float64(u.CacheWrite) * pricing.CacheCreationInputTokenCost)

	return cost, true
}
