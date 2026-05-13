package store

import (
	"regexp"
	"testing"

	"github.com/skrashevich/botmux/internal/models"
)

func TestModerationRUTriggerPatternRulesCompile(t *testing.T) {
	if len(moderationRUTriggerPatternRules) != 500 {
		t.Fatalf("expected 500 RU trigger rules, got %d", len(moderationRUTriggerPatternRules))
	}
	if len(moderationRUProfanityRules) != 40 {
		t.Fatalf("expected 40 RU profanity rules, got %d", len(moderationRUProfanityRules))
	}
	all := append([]models.ModerationRule{}, moderationRUTriggerPatternRules...)
	all = append(all, moderationRUProfanityRules...)
	for _, rule := range all {
		if _, err := regexp.Compile(rule.Pattern); err != nil {
			t.Fatalf("rule %q does not compile: %v", rule.Notes, err)
		}
	}
}
