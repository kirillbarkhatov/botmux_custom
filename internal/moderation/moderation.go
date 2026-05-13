package moderation

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"sort"
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
	levels, err := s.store.GetModerationLevels(botID, chatID)
	if err != nil {
		return err
	}
	if len(levels) >= 3 {
		return nil
	}
	providers, _ := s.store.ListModerationProviders(false)
	var localID, openAIID int64
	for _, p := range providers {
		if !p.Enabled {
			continue
		}
		if localID == 0 && (p.Kind == "local" || p.Kind == "openai_compatible" || p.Kind == "openai-compatible") {
			localID = p.ID
		}
		if openAIID == 0 && p.Kind == "openai" {
			openAIID = p.ID
		}
	}
	exists := map[int]bool{}
	for _, l := range levels {
		exists[l.Level] = true
	}
	for i := 1; i <= 3; i++ {
		if exists[i] {
			continue
		}
		d := DefaultLevel(i, botID, chatID, localID, openAIID)
		if err := s.store.SaveModerationLevel(models.ModerationChatLevel{
			BotID: botID, ChatID: chatID, Level: d.Level, Name: d.Name, Enabled: d.Enabled,
			ProviderID: d.ProviderID, Required: d.Required, OnlyIfUncertain: d.OnlyIfUncertain,
			SystemPrompt: d.SystemPrompt, UserPromptTemplate: d.UserPrompt, MinConfidence: d.MinConfidence,
			TriggerSeverity: d.TriggerSeverity, Action: d.Action,
		}); err != nil {
			return err
		}
	}
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
	if err := s.EnsureDefaults(msg.BotID, msg.ChatID); err != nil {
		log.Printf("[moderation] ensure defaults failed: %v", err)
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
	final, actionLevel, uncertain, results := s.evaluate(ctx, cfg, msg, eventID)
	event.FinalToxic = final.Toxic
	event.FinalSeverity = final.Severity
	event.FinalCategory = final.Category
	event.FinalConfidence = final.Confidence
	event.FinalReason = final.Reason
	if len(results) == 0 {
		event.Status = "error"
		event.ActionError = "no moderation levels produced a result"
		_ = s.store.UpdateModerationEvent(event)
		return
	}
	if uncertain && !final.Toxic {
		event.Status = "flagged"
		event.FinalAction = "none"
		_ = s.store.UpdateModerationEvent(event)
		return
	}
	if !final.Toxic {
		event.Status = "ok"
		event.FinalAction = "none"
		_ = s.store.UpdateModerationEvent(event)
		return
	}
	action := actionLevel.Action
	if actionLevel.Level == 0 {
		event.FinalAction = "none"
		event.Status = "flagged"
		event.ActionResult = "below action threshold"
		_ = s.store.UpdateModerationEvent(event)
		return
	}
	if action == "" {
		action = "alert"
	}
	event.FinalAction = action
	event.ActionDurationSeconds = actionLevel.DurationSeconds
	event.Status = "flagged"
	if err := s.applyAction(cfg, msg, &event, actionLevel, results, bot); err != nil {
		event.ActionError = err.Error()
		event.Status = "error"
	} else if action == "mute" || action == "ban" {
		event.Status = "action_taken"
	} else {
		event.Status = "flagged"
	}
	_ = s.store.UpdateModerationEvent(event)
}

func (s *Service) TestClassify(ctx context.Context, req models.ModerationTestRequest) (*models.ModerationTestResponse, error) {
	if err := s.EnsureDefaults(req.BotID, req.ChatID); err != nil {
		return nil, err
	}
	cfg, err := s.store.GetModerationChatConfig(req.BotID, req.ChatID)
	if err != nil {
		cfg = &models.ModerationChatConfig{BotID: req.BotID, ChatID: req.ChatID, IncludeContext: true, ContextMessagesLimit: 30, ContextMinutes: 30}
	}
	msg := Message{BotID: req.BotID, ChatID: req.ChatID, MessageID: -1, UserID: req.UserID, Username: req.Username, Text: req.MessageText}
	final, actionLevel, uncertain, results := s.evaluate(ctx, cfg, msg, 0)
	return &models.ModerationTestResponse{FinalVerdict: final, FinalAction: actionLevel.Action, Uncertain: uncertain, Results: results}, nil
}

func (s *Service) evaluate(ctx context.Context, cfg *models.ModerationChatConfig, msg Message, eventID int64) (models.ModerationVerdict, models.ModerationChatLevel, bool, []models.ModerationLevelResult) {
	levels, _ := s.store.GetModerationLevels(msg.BotID, msg.ChatID)
	sort.Slice(levels, func(i, j int) bool { return levels[i].Level < levels[j].Level })
	recent := s.recentContext(msg, cfg)
	var results []models.ModerationLevelResult
	var verdicts []models.ModerationVerdict
	uncertain := false
	for _, level := range levels {
		if !level.Enabled {
			continue
		}
		if level.Level == 3 && !uncertain {
			continue
		}
		res := s.runLevel(ctx, level, msg, recent, results)
		if eventID != 0 {
			res.EventID = eventID
			_ = s.store.SaveModerationLevelResult(res)
		}
		results = append(results, res)
		if res.Error != "" {
			uncertain = true
			continue
		}
		v := models.ModerationVerdict{Toxic: res.Toxic, Severity: res.Severity, Category: res.Category, Confidence: res.Confidence, NeedsContext: res.NeedsContext, ContextDependent: res.ContextDependent, NeedsHumanReview: res.NeedsHumanReview, Reason: res.Reason}
		verdicts = append(verdicts, v)
		if level.Level <= 2 && (!passesThreshold(v, level) || v.NeedsContext || v.NeedsHumanReview) {
			uncertain = true
		}
		if len(verdicts) >= 2 && verdicts[0].Toxic != verdicts[1].Toxic {
			uncertain = true
		}
	}
	final, actionLevel := finalDecision(levels, verdicts)
	return final, actionLevel, uncertain, results
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
	until := time.Now().Add(time.Duration(level.DurationSeconds) * time.Second).Unix()
	if level.DurationSeconds <= 0 {
		until = 0
	}
	switch action {
	case "mute":
		if err := bot.MuteUser(msg.ChatID, msg.UserID, until); err != nil {
			return err
		}
		event.ActionResult = "muted in source chat"
	case "ban":
		if err := bot.BanUserUntil(msg.ChatID, msg.UserID, until); err != nil {
			return err
		}
		event.ActionResult = "banned in source chat"
	}
	return nil
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
