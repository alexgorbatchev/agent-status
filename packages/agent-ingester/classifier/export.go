package classifier

import (
	internalclassifier "github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/classifier"
)

type Classifier = internalclassifier.Classifier

const DefaultIdleGracePeriod = internalclassifier.DefaultIdleGracePeriod

var New = internalclassifier.New
