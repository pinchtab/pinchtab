package bridge

import (
	"testing"
)

func TestAdBlockPatterns(t *testing.T) {
	// Verify we have a good number of patterns
	if len(AdBlockPatterns) < 50 {
		t.Errorf("expected at least 50 ad block patterns, got %d", len(AdBlockPatterns))
	}

	// Check for some essential patterns
	essentialPatterns := []string{
		"*google-analytics.com/*",
		"*doubleclick.net/*",
		"*facebook.com/tr/*",
		"*segment.io/*",
	}

	patternMap := make(map[string]bool)
	for _, p := range AdBlockPatterns {
		patternMap[p] = true
	}

	for _, essential := range essentialPatterns {
		if !patternMap[essential] {
			t.Errorf("missing essential pattern: %s", essential)
		}
	}
}

func TestCombineBlockPatterns(t *testing.T) {
	list1 := []string{"*.jpg", "*.png", "*.gif"}
	list2 := []string{"*.mp4", "*.png", "*.webm"} // .png is duplicate
	list3 := []string{"*.pdf", "*.doc"}

	combined := CombineBlockPatterns(list1, list2, list3)

	// Should have 7 unique patterns
	if len(combined) != 7 {
		t.Errorf("expected 7 unique patterns, got %d", len(combined))
	}

	// Verify all patterns are present
	expected := map[string]bool{
		"*.jpg":  true,
		"*.png":  true,
		"*.gif":  true,
		"*.mp4":  true,
		"*.webm": true,
		"*.pdf":  true,
		"*.doc":  true,
	}

	for _, pattern := range combined {
		if !expected[pattern] {
			t.Errorf("unexpected pattern: %s", pattern)
		}
		delete(expected, pattern)
	}

	if len(expected) > 0 {
		t.Errorf("missing patterns: %v", expected)
	}
}

func TestCombineBlockPatterns_Empty(t *testing.T) {
	// Test with empty lists
	combined := CombineBlockPatterns([]string{}, []string{})
	if len(combined) != 0 {
		t.Errorf("expected empty result for empty inputs, got %d patterns", len(combined))
	}

	// Test with one empty list
	list := []string{"*.jpg", "*.png"}
	combined = CombineBlockPatterns(list, []string{})
	if len(combined) != 2 {
		t.Errorf("expected 2 patterns, got %d", len(combined))
	}
}
