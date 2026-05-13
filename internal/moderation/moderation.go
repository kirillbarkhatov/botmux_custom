package moderation

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"strings"
	"time"

	"github.com/skrashevich/botmux/internal/models"
	"github.com/skrashevich/botmux/internal/store"
)

type Service struct {
	store  *store.Store
	client *Client
}

func NewService(s *store.Store) *Service {
	return &Service{store: s, client: &Client{}}
}

func (s *Service) EnsureDefaults(botID, chatID int64) error {
	return nil
}

func (s *Service) Process(ctx context.Context, msg Message, bot ActionBot) {
	if strings.TrimSpace(msg.Text) == "" {
		return
	}
	cfg, err := s.store.GetModerationChatConfig(msg.BotID, msg.ChatID)
	if err != nil {
		if err != sql.ErrNoRows {
			log.Printf("[moderation] config load failed bot=%d chat=%d: %v", msg.BotID, msg.ChatID, err)
		}
		return
	}
	if !cfg.Enabled {
		return
	}
	if cfg.SkipBotMessages && msg.FromIsBot {
		return
	}
	if bot != nil && msg.UserID != 0 && msg.UserID == bot.GetSelfID() {
		return
	}
	eventID, created, err := s.store.CreateModerationEvent(models.ModerationEvent{
		BotID: msg.BotID, ChatID: msg.ChatID, MessageID: msg.MessageID, UserID: msg.UserID, Username: msg.Username,
		MessageText: msg.Text, MessageDate: msg.Date, Status: "skipped", AlertChatID: cfg.AlertChatID,
	})
	if err != nil {
		log.Printf("[moderation] create event failed: %v", err)
		return
	}
	if !created {
		return
	}
	event := models.ModerationEvent{ID: eventID, BotID: msg.BotID, ChatID: msg.ChatID, MessageID: msg.MessageID, UserID: msg.UserID, Username: msg.Username, MessageText: msg.Text, MessageDate: msg.Date, AlertChatID: cfg.AlertChatID}
	if !cfg.RulesEnabled {
		event.Status = "skipped"
		event.ActionResult = "rules disabled"
		_ = s.store.UpdateModerationEvent(event)
		return
	}
	rules, err := s.store.ListModerationRules(msg.BotID, msg.ChatID, true)
	if err != nil {
		log.Printf("[moderation] rules load failed bot=%d chat=%d: %v", msg.BotID, msg.ChatID, err)
		event.Status = "skipped"
		event.ActionError = "rules load failed: " + err.Error()
		_ = s.store.UpdateModerationEvent(event)
		return
	}
	prefilter := EvaluateRules(msg.Text, rules)
	if prefilter.Decision == "clean" || prefilter.Decision == "allowed" {
		event.FinalToxic = false
		event.FinalSeverity = "none"
		event.FinalCategory = "none"
		event.FinalConfidence = 0
		event.FinalReason = prefilter.Reason
		event.FinalAction = "none"
		if prefilter.Decision == "allowed" {
			_ = s.saveLevel1Result(eventID, prefilter)
			event.Status = "ok"
		} else if cfg.LogCleanMessages {
			event.Status = "ok"
		} else {
			event.Status = "skipped"
		}
		_ = s.store.UpdateModerationEvent(event)
		return
	}
	_ = s.saveLevel1Result(eventID, prefilter)
	event.FinalToxic = true
	event.FinalSeverity = prefilter.Severity
	event.FinalCategory = prefilter.Category
	event.FinalConfidence = prefilter.Confidence
	event.FinalReason = prefilter.Reason
	event.FinalAction = "none"
	event.Status = "flagged"

	hardLevel := actionLevelFromPrefilter(prefilter)
	if prefilter.Decision == "hard_match" && hardLevel.Action != "" && hardLevel.Action != "none" {
		event.FinalAction = hardLevel.Action
		results, _ := s.store.GetModerationLevelResults(eventID)
		if err := s.applyAction(cfg, msg, &event, hardLevel, results, bot); err != nil {
			event.ActionError = err.Error()
			event.Status = "error"
		} else if hardLevel.Action == "mute" || hardLevel.Action == "ban" {
			event.Status = "action_taken"
		}
		_ = s.store.UpdateModerationEvent(event)
		return
	}

	_ = s.store.UpdateModerationEvent(event)
}

func (s *Service) TestClassify(ctx context.Context, req models.ModerationTestRequest) (*models.ModerationTestResponse, error) {
	return nil, fmt.Errorf("AI moderation classification is disabled in minimal moderation mode")
}

func (s *Service) TestRules(req models.ModerationRuleTestRequest) (*models.ModerationRuleTestResponse, error) {
	cfg, err := s.store.GetModerationChatConfig(req.BotID, req.ChatID)
	if err != nil {
		cfg = &models.ModerationChatConfig{BotID: req.BotID, ChatID: req.ChatID, RulesEnabled: true, AILevel2Enabled: true, AILevel2MinIntervalSeconds: 3600, AILevel2ContextMinutes: 60, MaxTextLengthForAI: 4000}
	}
	rules, err := s.store.ListModerationRules(req.BotID, req.ChatID, true)
	if err != nil {
		return nil, err
	}
	result := EvaluateRules(req.Text, rules)
	lastRun := parseTime(cfg.AILevel2LastRunAt)
	interval := cfg.AILevel2MinIntervalSeconds
	if interval <= 0 {
		interval = 3600
	}
	next := ""
	rateLimited := false
	if !lastRun.IsZero() {
		nextTime := lastRun.Add(time.Duration(interval) * time.Second)
		next = nextTime.Format(time.RFC3339)
		rateLimited = time.Now().Before(nextTime)
	}
	return &models.ModerationRuleTestResponse{
		NormalizedText: result.NormalizedText, Decision: result.Decision, MatchedRules: result.MatchedRules,
		WouldRunAILevel2: false, AIRateLimited: rateLimited, NextAIAllowedAt: next,
	}, nil
}

func (s *Service) saveLevel1Result(eventID int64, result models.ModerationPrefilterResult) error {
	raw, _ := json.Marshal(result.MatchedRules)
	res := models.ModerationLevelResult{
		EventID: eventID, Level: 1, ProviderKind: "rules", Model: "local-rules",
		Toxic:    result.Decision == "soft_match" || result.Decision == "hard_match",
		Severity: result.Severity, Category: result.Category, Confidence: result.Confidence,
		Reason: result.Reason, RawResponse: string(raw),
	}
	return s.store.SaveModerationLevelResult(res)
}

func (s *Service) runAILevel2(ctx context.Context, cfg *models.ModerationChatConfig, msg Message, prefilter models.ModerationPrefilterResult) models.ModerationLevelResult {
	provider, err := s.store.GetModerationProvider(cfg.AILevel2ProviderID)
	if err != nil || provider == nil || !provider.Enabled {
		return models.ModerationLevelResult{Level: 2, ProviderID: cfg.AILevel2ProviderID, Error: "AI Level 2 provider is not configured or disabled"}
	}
	recent := s.recentContextLevel2(msg, cfg)
	matches, _ := json.MarshalIndent(prefilter.MatchedRules, "", "  ")
	userPrompt := renderLevel2ContextualPrompt(msg, recent, string(matches), cfg.MaxTextLengthForAI)
	llmResult, err := s.client.Classify(ctx, *provider, Level2ContextualSystemPrompt, userPrompt)
	res := models.ModerationLevelResult{Level: 2, ProviderID: provider.ID, ProviderKind: provider.Kind, Model: provider.Model}
	if err != nil {
		res.Error = err.Error()
		return res
	}
	res.Toxic = llmResult.Verdict.Toxic
	res.Severity = llmResult.Verdict.Severity
	res.Category = llmResult.Verdict.Category
	res.Confidence = llmResult.Verdict.Confidence
	res.NeedsContext = llmResult.Verdict.NeedsContext
	res.ContextDependent = llmResult.Verdict.ContextDependent
	res.NeedsHumanReview = llmResult.Verdict.NeedsHumanReview
	res.Reason = llmResult.Verdict.Reason
	res.RawResponse = llmResult.Raw
	res.LatencyMS = llmResult.LatencyMS
	return res
}

func (s *Service) recentContextLevel2(msg Message, cfg *models.ModerationChatConfig) string {
	limit := 100
	minutes := cfg.AILevel2ContextMinutes
	if minutes <= 0 {
		minutes = 60
	}
	msgs, err := s.store.GetMessages(msg.BotID, msg.ChatID, limit, 0)
	if err != nil {
		return ""
	}
	cutoff := time.Now().Add(-time.Duration(minutes) * time.Minute).UnixMilli()
	var lines []string
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		if m.Date > 0 && m.Date < cutoff {
			continue
		}
		text := truncate(m.Text, 600)
		if text == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s (%d): %s", m.FromUser, m.FromID, text))
	}
	max := cfg.MaxTextLengthForAI
	if max <= 0 {
		max = 4000
	}
	return truncate(strings.Join(lines, "\n"), max)
}

func renderLevel2ContextualPrompt(msg Message, recent, matches string, max int) string {
	if max <= 0 {
		max = 4000
	}
	repl := map[string]string{
		"{{rule_matches}}":   truncate(matches, max),
		"{{recent_context}}": truncate(recent, max),
		"{{target_message}}": truncate(msg.Text, max),
		"{{username}}":       msg.Username,
		"{{user_id}}":        fmt.Sprintf("%d", msg.UserID),
		"{{chat_title}}":     msg.ChatTitle,
		"{{chat_id}}":        fmt.Sprintf("%d", msg.ChatID),
	}
	out := Level2ContextualUserPrompt
	for k, v := range repl {
		out = strings.ReplaceAll(out, k, v)
	}
	return out
}

func actionLevelFromPrefilter(result models.ModerationPrefilterResult) models.ModerationChatLevel {
	action := result.Action
	if action == "" {
		action = "none"
	}
	return models.ModerationChatLevel{
		Level: 1, Name: "Level 1: rules", Action: action, DurationSeconds: result.DurationSeconds,
		MinConfidence: 0.70, TriggerSeverity: "low",
	}
}

func parseTime(v string) time.Time {
	if v == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return time.Time{}
	}
	return t
}

func aiSkipReason(cfg *models.ModerationChatConfig, lastRun time.Time) string {
	if cfg == nil || !cfg.AILevel2Enabled {
		return "AI Level 2 skipped: disabled"
	}
	if cfg.AILevel2ProviderID == 0 {
		return "AI Level 2 skipped: provider is not configured"
	}
	if lastRun.IsZero() {
		return "AI Level 2 skipped"
	}
	interval := cfg.AILevel2MinIntervalSeconds
	if interval <= 0 {
		interval = 3600
	}
	next := lastRun.Add(time.Duration(interval) * time.Second)
	if time.Now().Before(next) {
		return "AI Level 2 skipped: rate limited until " + next.Format(time.RFC3339)
	}
	return "AI Level 2 skipped"
}

func (s *Service) runLevel(ctx context.Context, level models.ModerationChatLevel, msg Message, recent string, prior []models.ModerationLevelResult) models.ModerationLevelResult {
	provider, err := s.store.GetModerationProvider(level.ProviderID)
	if err != nil || provider == nil || !provider.Enabled {
		return models.ModerationLevelResult{Level: level.Level, ProviderID: level.ProviderID, Error: "provider is not configured or disabled"}
	}
	systemPrompt := level.SystemPrompt
	userPrompt := renderPrompt(level.UserPromptTemplate, msg, recent, prior)
	llmResult, err := s.client.Classify(ctx, *provider, systemPrompt, userPrompt)
	res := models.ModerationLevelResult{Level: level.Level, ProviderID: provider.ID, ProviderKind: provider.Kind, Model: provider.Model}
	if err != nil {
		res.Error = err.Error()
		return res
	}
	res.Toxic = llmResult.Verdict.Toxic
	res.Severity = llmResult.Verdict.Severity
	res.Category = llmResult.Verdict.Category
	res.Confidence = llmResult.Verdict.Confidence
	res.NeedsContext = llmResult.Verdict.NeedsContext
	res.ContextDependent = llmResult.Verdict.ContextDependent
	res.NeedsHumanReview = llmResult.Verdict.NeedsHumanReview
	res.Reason = llmResult.Verdict.Reason
	res.RawResponse = llmResult.Raw
	res.LatencyMS = llmResult.LatencyMS
	return res
}

func (s *Service) recentContext(msg Message, cfg *models.ModerationChatConfig) string {
	if cfg == nil || !cfg.IncludeContext {
		return ""
	}
	limit := cfg.ContextMessagesLimit
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	msgs, err := s.store.GetMessages(msg.BotID, msg.ChatID, limit, 0)
	if err != nil {
		return ""
	}
	var lines []string
	cutoff := int64(0)
	if cfg.ContextMinutes > 0 {
		cutoff = time.Now().Add(-time.Duration(cfg.ContextMinutes) * time.Minute).UnixMilli()
	}
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		if cutoff > 0 && m.Date > 0 && m.Date < cutoff {
			continue
		}
		text := truncate(m.Text, 600)
		if text == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s (%d): %s", m.FromUser, m.FromID, text))
	}
	return truncate(strings.Join(lines, "\n"), 12000)
}

func renderPrompt(tpl string, msg Message, recent string, prior []models.ModerationLevelResult) string {
	if tpl == "" {
		tpl = Level1UserPrompt
	}
	priorBytes, _ := json.Marshal(prior)
	repl := map[string]string{
		"{{message_text}}":   truncate(msg.Text, 4000),
		"{{target_message}}": truncate(msg.Text, 4000),
		"{{username}}":       msg.Username,
		"{{user_id}}":        fmt.Sprintf("%d", msg.UserID),
		"{{chat_title}}":     msg.ChatTitle,
		"{{chat_id}}":        fmt.Sprintf("%d", msg.ChatID),
		"{{recent_context}}": recent,
		"{{level_results}}":  string(priorBytes),
	}
	out := tpl
	for k, v := range repl {
		out = strings.ReplaceAll(out, k, v)
	}
	return out
}

func finalDecision(levels []models.ModerationChatLevel, verdicts []models.ModerationVerdict) (models.ModerationVerdict, models.ModerationChatLevel) {
	var final models.ModerationVerdict
	var actionLevel models.ModerationChatLevel
	for _, v := range verdicts {
		if !v.Toxic {
			continue
		}
		if severityRank(v.Severity) > severityRank(final.Severity) || v.Confidence > final.Confidence {
			final = v
		}
	}
	for _, l := range levels {
		if !l.Enabled {
			continue
		}
		for _, v := range verdicts {
			if v.Toxic && passesThreshold(v, l) && severityRank(v.Severity) >= severityRank(l.TriggerSeverity) {
				if severityRank(l.TriggerSeverity) >= severityRank(actionLevel.TriggerSeverity) {
					actionLevel = l
				}
			}
		}
	}
	if actionLevel.Action == "" {
		actionLevel.Action = "alert"
	}
	return final, actionLevel
}

func passesThreshold(v models.ModerationVerdict, l models.ModerationChatLevel) bool {
	return v.Toxic && v.Confidence >= l.MinConfidence && severityRank(v.Severity) >= severityRank(l.TriggerSeverity)
}

func severityRank(s string) int {
	switch strings.ToLower(s) {
	case "low":
		return 1
	case "medium":
		return 2
	case "high":
		return 3
	default:
		return 0
	}
}

func (s *Service) applyAction(cfg *models.ModerationChatConfig, msg Message, event *models.ModerationEvent, level models.ModerationChatLevel, results []models.ModerationLevelResult, bot ActionBot) error {
	if bot == nil {
		return fmt.Errorf("managed bot is not available")
	}
	action := level.Action
	if action == "" || action == "none" {
		event.ActionResult = "no action"
		return nil
	}
	if cfg.AlertChatID != 0 {
		alertID, err := bot.SendMessageGetID(cfg.AlertChatID, formatAlert(cfg, msg, event, level, results))
		if err != nil {
			event.ActionError = "alert failed: " + err.Error()
		} else {
			event.AlertSent = true
			event.AlertMessageID = alertID
		}
	}
	if action == "alert" {
		event.ActionResult = "alert sent"
		return nil
	}
	if msg.UserID == 0 {
		return fmt.Errorf("cannot apply %s without user id", action)
	}
	if isAdmin(bot, msg.ChatID, msg.UserID) {
		return fmt.Errorf("refusing to apply %s to chat administrator", action)
	}
	switch action {
	case "mute":
		duration := NormalizeMuteDurationSeconds(level.DurationSeconds)
		until := time.Now().Add(time.Duration(duration) * time.Second).Unix()
		if err := bot.MuteUser(msg.ChatID, msg.UserID, until); err != nil {
			return err
		}
		event.ActionDurationSeconds = duration
		event.ActionResult = fmt.Sprintf("muted in source chat for %ds", duration)
	case "ban":
		return fmt.Errorf("ban action is disabled in minimal moderation mode")
	}
	return nil
}

func NormalizeMuteDurationSeconds(seconds int64) int64 {
	if seconds <= 0 {
		return 300
	}
	if seconds < 300 {
		return 300
	}
	if seconds > 3600 {
		return 3600
	}
	return seconds
}

func isAdmin(bot ActionBot, chatID, userID int64) bool {
	admins, err := bot.GetAdmins(chatID)
	if err != nil {
		log.Printf("[moderation] cannot check admins chat=%d user=%d: %v", chatID, userID, err)
		return false
	}
	for _, a := range admins {
		if a.UserID == userID {
			return true
		}
	}
	return false
}

func formatAlert(cfg *models.ModerationChatConfig, msg Message, event *models.ModerationEvent, level models.ModerationChatLevel, results []models.ModerationLevelResult) string {
	var b strings.Builder
	b.WriteString("🚨 <b>Moderation alert</b>\n\n")
	b.WriteString(fmt.Sprintf("Severity: <b>%s</b>\n", html.EscapeString(strings.ToUpper(event.FinalSeverity))))
	b.WriteString(fmt.Sprintf("Category: %s\n", html.EscapeString(event.FinalCategory)))
	b.WriteString(fmt.Sprintf("Action: %s\n", html.EscapeString(level.Action)))
	b.WriteString(fmt.Sprintf("Confidence: %.2f\n", event.FinalConfidence))
	b.WriteString(fmt.Sprintf("Reason: %s\n\n", html.EscapeString(event.FinalReason)))
	b.WriteString(fmt.Sprintf("Source chat: %s (%d)\n", html.EscapeString(msg.ChatTitle), msg.ChatID))
	b.WriteString(fmt.Sprintf("Alert chat: %d\n", cfg.AlertChatID))
	b.WriteString(fmt.Sprintf("User: %s (%d)\n", html.EscapeString(msg.Username), msg.UserID))
	b.WriteString(fmt.Sprintf("Message ID: %d\n\n", msg.MessageID))
	b.WriteString("Message:\n<pre>")
	b.WriteString(html.EscapeString(truncate(msg.Text, 2500)))
	b.WriteString("</pre>\n\nLevel results:\n")
	for _, r := range results {
		if r.Error != "" {
			b.WriteString(fmt.Sprintf("L%d %s: error=%s\n", r.Level, html.EscapeString(r.Model), html.EscapeString(r.Error)))
			continue
		}
		b.WriteString(fmt.Sprintf("L%d %s: toxic=%v severity=%s confidence=%.2f\n", r.Level, html.EscapeString(r.Model), r.Toxic, html.EscapeString(r.Severity), r.Confidence))
	}
	return truncate(b.String(), 3900)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 20 {
		return s[:max]
	}
	return s[:max-15] + "...[truncated]"
}
