package store

import (
	"regexp"
	"testing"
)

func TestModerationRUTriggerPatternRulesCompile(t *testing.T) {
	if len(moderationRUTriggerPatternRules) != 500 {
		t.Fatalf("expected 500 RU trigger rules, got %d", len(moderationRUTriggerPatternRules))
	}
	for _, rule := range moderationRUTriggerPatternRules {
		if _, err := regexp.Compile(rule.Pattern); err != nil {
			t.Fatalf("rule %q does not compile: %v", rule.Notes, err)
		}
	}
}
