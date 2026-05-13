package moderation

import (
	"log"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/skrashevich/botmux/internal/models"
)

func NormalizeText(text string) string {
	s := strings.ToLower(strings.TrimSpace(text))
	s = strings.ReplaceAll(s, "ё", "е")
	var b strings.Builder
	runes := []rune(s)
	for i, r := range runes {
		if isSafeLetterSeparator(r) && i > 0 && i+1 < len(runes) && unicode.IsLetter(runes[i-1]) && unicode.IsLetter(runes[i+1]) {
			continue
		}
		if unicode.IsSpace(r) {
			b.WriteRune(' ')
			continue
		}
		b.WriteRune(r)
	}
	s = strings.Join(strings.Fields(b.String()), " ")
	var out strings.Builder
	var prev rune
	repeat := 0
	for _, r := range s {
		if r == prev {
			repeat++
		} else {
			prev = r
			repeat = 1
		}
		if repeat <= 3 {
			out.WriteRune(r)
		}
	}
	return out.String()
}

func isSafeLetterSeparator(r rune) bool {
	switch r {
	case '.', '-', '_', '*', '~', '|':
		return true
	default:
		return false
	}
}

func EvaluateRules(text string, rules []models.ModerationRule) models.ModerationPrefilterResult {
	normalized := NormalizeText(text)
	result := models.ModerationPrefilterResult{NormalizedText: normalized, Decision: "clean", Severity: "none", Category: "none"}
	var suspicious []models.ModerationRuleMatch
	var allow []models.ModerationRuleMatch
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		matched, errText := matchRule(text, normalized, rule)
		if errText != "" {
			log.Printf("[moderation] invalid rule id=%d kind=%s pattern=%q: %s", rule.ID, rule.Kind, rule.Pattern, errText)
		}
		if !matched {
			continue
		}
		rm := ruleMatch(rule, errText)
		if strings.HasPrefix(rule.Kind, "allow_") || rule.Mode == "allow" {
			allow = append(allow, rm)
		} else {
			suspicious = append(suspicious, rm)
		}
	}
	if len(allow) > 0 {
		result.Decision = "allowed"
		result.MatchedRules = allow
		result.Severity = "none"
		result.Category = "none"
		result.Reason = "allow rule matched"
		return result
	}
	if len(suspicious) == 0 {
		return result
	}
	result.MatchedRules = suspicious
	result.Decision = "soft_match"
	for _, m := range suspicious {
		if m.Mode == "hard" {
			result.Decision = "hard_match"
		}
		if severityRank(m.Severity) > severityRank(result.Severity) || (severityRank(m.Severity) == severityRank(result.Severity) && m.Confidence > result.Confidence) {
			result.Severity = m.Severity
			result.Category = m.Category
			result.Confidence = m.Confidence
			result.Action = m.Action
			result.DurationSeconds = m.DurationSeconds
		}
	}
	if result.Action == "" {
		result.Action = "none"
	}
	result.Reason = "local moderation rule matched"
	return result
}

func ShouldRunAILevel2(result models.ModerationPrefilterResult, cfg *models.ModerationChatConfig, lastRun time.Time) bool {
	if cfg == nil || !cfg.AILevel2Enabled || cfg.AILevel2ProviderID == 0 {
		return false
	}
	if result.Decision != "soft_match" && result.Decision != "hard_match" {
		return false
	}
	interval := cfg.AILevel2MinIntervalSeconds
	if interval <= 0 {
		interval = 3600
	}
	if lastRun.IsZero() {
		return true
	}
	return time.Since(lastRun) >= time.Duration(interval)*time.Second
}

func matchRule(original, normalized string, rule models.ModerationRule) (bool, string) {
	kind := strings.TrimPrefix(rule.Kind, "allow_")
	pattern := strings.TrimSpace(rule.Pattern)
	if pattern == "" {
		return false, ""
	}
	switch kind {
	case "keyword":
		return keywordMatch(normalized, NormalizeText(pattern)), ""
	case "phrase":
		return strings.Contains(normalized, NormalizeText(pattern)), ""
	case "regex":
		matched, err := matchRegexPattern(pattern, original, normalized)
		if err != nil {
			return false, err.Error()
		}
		return matched, ""
	default:
		return false, ""
	}
}

func matchRegexPattern(pattern, original, normalized string) (bool, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false, err
	}
	if re.MatchString(original) || re.MatchString(normalized) {
		return true, nil
	}
	if !strings.Contains(pattern, `\b`) && !strings.Contains(pattern, `\w`) {
		return false, nil
	}
	unicodePattern := unicodeWordRegexPattern(pattern)
	if unicodePattern == pattern {
		return false, nil
	}
	re, err = regexp.Compile(unicodePattern)
	if err != nil {
		return false, err
	}
	return re.MatchString(original) || re.MatchString(normalized), nil
}

func unicodeWordRegexPattern(pattern string) string {
	pattern = strings.ReplaceAll(pattern, `\w`, `[\p{L}\p{N}_]`)
	return strings.ReplaceAll(pattern, `\b`, `(?:^|$|[^\p{L}\p{N}_])`)
}

func keywordMatch(text, word string) bool {
	if word == "" {
		return false
	}
	idx := strings.Index(text, word)
	for idx >= 0 {
		beforeOK := idx == 0 || !isWordRune([]rune(text[:idx])[len([]rune(text[:idx]))-1])
		afterPos := idx + len(word)
		afterOK := afterPos >= len(text)
		if !afterOK {
			after := []rune(text[afterPos:])
			afterOK = len(after) == 0 || !isWordRune(after[0])
		}
		if beforeOK && afterOK {
			return true
		}
		next := strings.Index(text[idx+len(word):], word)
		if next < 0 {
			return false
		}
		idx += len(word) + next
	}
	return false
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

func ruleMatch(rule models.ModerationRule, errText string) models.ModerationRuleMatch {
	scope := "chat"
	if rule.BotID == 0 && rule.ChatID == 0 {
		scope = "global"
	}
	return models.ModerationRuleMatch{
		RuleID: rule.ID, BotID: rule.BotID, ChatID: rule.ChatID, Scope: scope, Language: rule.Language,
		Kind: rule.Kind, Pattern: rule.Pattern, Category: rule.Category, Severity: rule.Severity,
		Confidence: rule.Confidence, Action: rule.Action, DurationSeconds: rule.DurationSeconds,
		Mode: rule.Mode, Notes: rule.Notes, Error: errText,
	}
}
