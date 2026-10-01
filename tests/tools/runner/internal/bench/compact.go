package bench

const (
	CompactAfterAnthropicMessages = 14
	KeepRecentAnthropicMessages   = 10
	CompactAfterOpenAIItems       = 16
	KeepRecentOpenAIItems         = 10
)

func CompactAnthropicConversation(conversation []any, summary string) []any {
	if len(conversation) <= CompactAfterAnthropicMessages {
		return conversation
	}
	head := conversation[0]
	recentStart := max(len(conversation)-KeepRecentAnthropicMessages, 1)
	recent := conversation[recentStart:]

	result := make([]any, 0, 2+len(recent))
	result = append(result, head)
	result = append(result, map[string]any{
		"role":    "user",
		"content": summary,
	})
	result = append(result, recent...)
	return result
}

func CompactOpenAIConversation(conversation []any, summary string) []any {
	if len(conversation) <= CompactAfterOpenAIItems {
		return conversation
	}
	head := conversation[0]
	recentStart := max(len(conversation)-KeepRecentOpenAIItems, 1)
	recent := conversation[recentStart:]

	result := make([]any, 0, 2+len(recent))
	result = append(result, head)
	result = append(result, map[string]any{
		"role": "user",
		"content": []any{
			map[string]any{"type": "input_text", "text": summary},
		},
	})
	result = append(result, recent...)
	return result
}

func CompactConversation(provider string, conversation []any, summary string) []any {
	if provider == "anthropic" {
		return CompactAnthropicConversation(conversation, summary)
	}
	return CompactOpenAIConversation(conversation, summary)
}
