package summarizer

import (
	internalsummarizer "github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/summarizer"
)

type Summarizer = internalsummarizer.Summarizer
type Config = internalsummarizer.Config
type SummaryInput = internalsummarizer.SummaryInput
type SummaryResult = internalsummarizer.SummaryResult
type ToolExecution = internalsummarizer.ToolExecution

var New = internalsummarizer.New
var FallbackSummary = internalsummarizer.FallbackSummary
