package pricing

import (
	internalpricing "github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/pricing"
)

type Engine = internalpricing.Engine
type Usage = internalpricing.Usage

var NewPricingEngine = internalpricing.NewPricingEngine
