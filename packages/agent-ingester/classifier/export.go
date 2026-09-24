package classifier

import (
	internalclassifier "github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/classifier"
)

type Classifier = internalclassifier.Classifier

const DefaultIdleGracePeriod = internalclassifier.DefaultIdleGracePeriod
const DefaultTurnEndGraceWindow = internalclassifier.DefaultTurnEndGraceWindow
const DefaultTurnEndGrace = internalclassifier.DefaultTurnEndGrace

var New = internalclassifier.New
