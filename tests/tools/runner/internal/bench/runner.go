package bench

import "time"

type ToolCall struct {
	ID             string
	Command        string
	TimeoutSeconds int
}

type ToolExecutionResult struct {
	ID      string
	IsError bool
	Content string
}

type UsageCounters struct {
	RequestCount             int
	InputTokens              int
	OutputTokens             int
	CacheCreationInputTokens int
	CacheReadInputTokens     int
}

type Runner interface {
	Provider() string
	Source() string
	Model() string
	ToolDefinitions() any
	InitialConversation(userPrompt string) []any
	Send(systemPrompt string, conversation []any) (any, error)
	ExtractToolCalls(response any, defaultTimeout time.Duration) []ToolCall
	AppendToolResults(conversation []any, response any, results []ToolExecutionResult) []any
	ExtractFinalText(response any) string
	Usage() UsageCounters
}
