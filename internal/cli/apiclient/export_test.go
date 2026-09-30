package apiclient

func VocabTokenFor(base, tabID string) string {
	if tabID == "" {
		return ""
	}
	_, token := VocabForAction(base, tabID)
	return token
}
