package moderation

import (
	"testing"

	"github.com/skrashevich/botmux/internal/models"
)

func TestEvaluateRulesUnicodeRegexWordBoundary(t *testing.T) {
	rules := []models.ModerationRule{
		{ID: 1, Enabled: true, Kind: "regex", Pattern: `(?i)\bпохуй\b`, Category: "insult", Severity: "medium", Confidence: 0.78, Mode: "soft"},
	}
	got := EvaluateRules("похуй", rules)
	if got.Decision != "soft_match" {
		t.Fatalf("expected soft_match, got %+v", got)
	}
	got = EvaluateRules("непохуйный", rules)
	if got.Decision != "clean" {
		t.Fatalf("expected clean for embedded word, got %+v", got)
	}
}

func TestEvaluateRulesUnicodeRegexWordChar(t *testing.T) {
	rules := []models.ModerationRule{
		{ID: 1, Enabled: true, Kind: "regex", Pattern: `(?i)\bхуйн\w*\b`, Category: "insult", Severity: "medium", Confidence: 0.78, Mode: "soft"},
	}
	got := EvaluateRules("хуйню", rules)
	if got.Decision != "soft_match" {
		t.Fatalf("expected soft_match, got %+v", got)
	}
}

func TestEvaluateRulesProfanityHardMute(t *testing.T) {
	rules := []models.ModerationRule{
		{ID: 1, Enabled: true, Language: "ru", Kind: "regex", Pattern: `(?i)\b(бля(?:д[ьи]|ть)?|сука|хуй|ху[её]\w*|пизд\w*|п[ие]здец|[её]б\w*)\b`, Category: "profanity", Severity: "medium", Confidence: 0.90, Mode: "hard", Action: "mute", DurationSeconds: 300},
	}
	got := EvaluateRules("б.л.я.дь", rules)
	if got.Decision != "hard_match" || got.Action != "mute" || got.DurationSeconds != 300 || got.Category != "profanity" {
		t.Fatalf("expected hard profanity mute, got %+v", got)
	}
}
