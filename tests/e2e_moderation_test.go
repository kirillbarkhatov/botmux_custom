package tests

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/skrashevich/botmux/internal/models"
)

func toxicVerdict() map[string]any {
	return map[string]any{
		"toxic":              true,
		"severity":           "high",
		"category":           "harassment",
		"confidence":         0.92,
		"needs_context":      false,
		"context_dependent":  false,
		"needs_human_review": false,
		"reason":             "Targeted insult",
	}
}

func cleanVerdict() map[string]any {
	return map[string]any{
		"toxic":              false,
		"severity":           "none",
		"category":           "none",
		"confidence":         0.95,
		"needs_context":      false,
		"context_dependent":  false,
		"needs_human_review": false,
		"reason":             "Not abusive",
	}
}

func saveModerationProvider(t *testing.T, h *e2eHarness, url string, kind string) int64 {
	t.Helper()
	if err := h.store.SaveModerationProvider(models.ModerationProvider{
		Name: "test " + kind, Kind: kind, APIURL: url, APIKey: "secret-key", Model: "test-model", Enabled: true, TimeoutSeconds: 5,
	}); err != nil {
		t.Fatalf("SaveModerationProvider: %v", err)
	}
	providers, err := h.store.ListModerationProviders(false)
	if err != nil || len(providers) == 0 {
		t.Fatalf("ListModerationProviders: %v", err)
	}
	return providers[len(providers)-1].ID
}

func saveModerationRule(t *testing.T, h *e2eHarness, rule models.ModerationRule) {
	t.Helper()
	if err := h.store.SaveModerationRule(rule); err != nil {
		t.Fatalf("SaveModerationRule: %v", err)
	}
}

func makeNewMemberUpdate(updateID int, chatID int64, members ...map[string]any) map[string]any {
	list := make([]any, 0, len(members))
	for _, m := range members {
		list = append(list, m)
	}
	return map[string]any{
		"update_id": float64(updateID),
		"message": map[string]any{
			"message_id": float64(updateID * 10),
			"date":       float64(1700000000 + updateID),
			"chat":       map[string]any{"id": float64(chatID), "type": "supergroup", "title": "Source"},
			"from":       map[string]any{"id": float64(1), "is_bot": false, "username": "inviter"},
			"new_chat_members": list,
		},
	}
}

func requestJSONInt(body []byte, key string) int64 {
	var m map[string]any
	_ = json.Unmarshal(body, &m)
	switch v := m[key].(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case int:
		return int64(v)
	default:
		return 0
	}
}

func TestE2E_Moderation(t *testing.T) {
	t.Run("enabled chat sends alert with same bot and deduplicates", func(t *testing.T) {
		h := setupE2E(t)
		llm := newFakeLLM(t)
		llm.SetNextRoute(toxicVerdict())
		botID := registerAndManage(h, "mod01:token", "modbot01")
		_ = saveModerationProvider(t, h, llm.URL(), "local")
		_ = h.store.UpsertChat(botID, models.Chat{ID: 100, Type: "supergroup", Title: "Source", UpdatedAt: time.Now().Format(time.RFC3339)})
		_ = h.store.UpsertChat(botID, models.Chat{ID: 200, Type: "supergroup", Title: "Alerts", UpdatedAt: time.Now().Format(time.RFC3339)})
		if err := h.store.SaveModerationChatConfig(models.ModerationChatConfig{BotID: botID, ChatID: 100, Enabled: true, AlertChatID: 200, SkipBotMessages: true, IncludeContext: true, ContextMessagesLimit: 30, ContextMinutes: 30}); err != nil {
			t.Fatalf("SaveModerationChatConfig: %v", err)
		}
		saveModerationRule(t, h, models.ModerationRule{BotID: botID, ChatID: 100, Enabled: true, Kind: "phrase", Pattern: "you are awful", Category: "harassment", Severity: "high", Confidence: 0.9, Mode: "hard", Action: "alert"})

		update := makeTextUpdate(1, 100, 501, "baduser", "you are awful")
		h.InjectUpdate(botID, update)
		h.InjectUpdate(botID, update)

		events, err := h.store.ListModerationEvents(botID, 100, 0, "", "", "", "", "", 10, 0)
		if err != nil {
			t.Fatalf("ListModerationEvents: %v", err)
		}
		if len(events) != 1 {
			t.Fatalf("expected 1 moderation event, got %d", len(events))
		}
		if events[0].FinalAction != "alert" || !events[0].AlertSent || events[0].AlertChatID != 200 {
			t.Fatalf("unexpected event: %+v", events[0])
		}
		if cnt := h.fake.RequestsCountFor("sendMessage"); cnt != 1 {
			t.Fatalf("expected exactly one alert sendMessage, got %d", cnt)
		}
		req := h.fake.RequestsFor("sendMessage")[0]
		if req.token != "mod01:token" || !strings.Contains(string(req.body), "chat_id=200") {
			t.Fatalf("alert used wrong bot/chat: token=%s body=%s", req.token, string(req.body))
		}
	})

	t.Run("enabled chat A disabled chat B", func(t *testing.T) {
		h := setupE2E(t)
		llm := newFakeLLM(t)
		llm.SetNextRoute(cleanVerdict())
		botID := registerAndManage(h, "mod02:token", "modbot02")
		saveModerationProvider(t, h, llm.URL(), "local")
		if err := h.store.SaveModerationChatConfig(models.ModerationChatConfig{BotID: botID, ChatID: 10, Enabled: true, SkipBotMessages: true, IncludeContext: true}); err != nil {
			t.Fatalf("SaveModerationChatConfig: %v", err)
		}
		h.InjectUpdate(botID, makeTextUpdate(2, 10, 7, "u", "hello"))
		h.InjectUpdate(botID, makeTextUpdate(3, 11, 7, "u", "hello"))
		if cnt := llm.RequestsCountFor("/chat/completions"); cnt != 0 {
			t.Fatalf("clean messages must not call moderation LLM, got %d calls", cnt)
		}
		eventsA, _ := h.store.ListModerationEvents(botID, 10, 0, "", "", "", "", "", 10, 0)
		eventsB, _ := h.store.ListModerationEvents(botID, 11, 0, "", "", "", "", "", 10, 0)
		if len(eventsA) != 1 || len(eventsB) != 0 {
			t.Fatalf("unexpected events: A=%d B=%d", len(eventsA), len(eventsB))
		}
	})

	t.Run("provider API disabled and alert chats are current bot only", func(t *testing.T) {
		h := setupE2E(t, withHTTPServer())
		botA := registerAndManage(h, "mod03a:token", "modbot03a")
		botB := registerAndManage(h, "mod03b:token", "modbot03b")
		_ = h.store.UpsertChat(botA, models.Chat{ID: 100, Type: "group", Title: "A"})
		_ = h.store.UpsertChat(botB, models.Chat{ID: 200, Type: "group", Title: "B"})
		saveModerationProvider(t, h, "http://127.0.0.1:1/v1", "openai")

		req, _ := http.NewRequest(http.MethodGet, h.ts.URL+"/api/moderation/providers", nil)
		req.AddCookie(&http.Cookie{Name: "botmux_session", Value: h.session})
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("providers request: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusGone {
			t.Fatalf("expected providers API disabled with 410, got %d", resp.StatusCode)
		}

		req, _ = http.NewRequest(http.MethodGet, h.ts.URL+"/api/moderation/alert-chats?bot_id="+jsonNumber(botA), nil)
		req.AddCookie(&http.Cookie{Name: "botmux_session", Value: h.session})
		resp, err = http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("alert chats request: %v", err)
		}
		defer resp.Body.Close()
		var chats []models.Chat
		_ = json.NewDecoder(resp.Body).Decode(&chats)
		if len(chats) != 1 || chats[0].ID != 100 {
			t.Fatalf("expected only bot A chat, got %+v", chats)
		}
	})

	t.Run("mute action targets source chat", func(t *testing.T) {
		h := setupE2E(t)
		llm := newFakeLLM(t)
		llm.SetNextRoute(toxicVerdict())
		botID := registerAndManage(h, "mod05:token", "modbot05")
		_ = saveModerationProvider(t, h, llm.URL(), "local")
		if err := h.store.SaveModerationChatConfig(models.ModerationChatConfig{BotID: botID, ChatID: 100, Enabled: true, AlertChatID: 200, SkipBotMessages: true, IncludeContext: true}); err != nil {
			t.Fatalf("SaveModerationChatConfig: %v", err)
		}
		saveModerationRule(t, h, models.ModerationRule{BotID: botID, ChatID: 100, Enabled: true, Kind: "keyword", Pattern: "bad", Category: "harassment", Severity: "high", Confidence: 0.9, Mode: "hard", Action: "mute", DurationSeconds: 3600})
		h.InjectUpdate(botID, makeTextUpdate(5, 100, 55, "mute_me", "bad"))
		reqs := h.fake.RequestsFor("restrictChatMember")
		if len(reqs) != 1 || !strings.Contains(string(reqs[0].body), `"chat_id":100`) && !strings.Contains(string(reqs[0].body), "chat_id=100") {
			events, _ := h.store.ListModerationEvents(botID, 100, 0, "", "", "", "", "", 10, 0)
			t.Fatalf("expected mute in source chat 100, got %+v events=%+v", reqs, events)
		}
	})

	t.Run("ban action disabled in minimal mode", func(t *testing.T) {
		h := setupE2E(t)
		llm := newFakeLLM(t)
		llm.SetNextRoute(toxicVerdict())
		botID := registerAndManage(h, "mod06:token", "modbot06")
		_ = saveModerationProvider(t, h, llm.URL(), "local")
		if err := h.store.SaveModerationChatConfig(models.ModerationChatConfig{BotID: botID, ChatID: 300, Enabled: true, AlertChatID: 400, SkipBotMessages: true, IncludeContext: true}); err != nil {
			t.Fatalf("SaveModerationChatConfig: %v", err)
		}
		saveModerationRule(t, h, models.ModerationRule{BotID: botID, ChatID: 300, Enabled: true, Kind: "keyword", Pattern: "bad", Category: "harassment", Severity: "high", Confidence: 0.9, Mode: "hard", Action: "ban", DurationSeconds: 3600})
		h.InjectUpdate(botID, makeTextUpdate(6, 300, 66, "ban_me", "bad"))
		if got := len(h.fake.RequestsFor("banChatMember")); got != 0 {
			t.Fatalf("expected no ban request in minimal mode, got %d", got)
		}
	})

	t.Run("new member mute clamps duration and skips bots and self", func(t *testing.T) {
		h := setupE2E(t)
		botID := registerAndManage(h, "mod07:token", "modbot07")
		if err := h.store.SaveModerationChatConfig(models.ModerationChatConfig{BotID: botID, ChatID: 700, Enabled: true, NewMemberMuteEnabled: true, NewMemberMuteSeconds: 60}); err != nil {
			t.Fatalf("SaveModerationChatConfig: %v", err)
		}
		before := time.Now().Unix()
		h.InjectUpdate(botID, makeNewMemberUpdate(7, 700,
			map[string]any{"id": float64(701), "is_bot": false, "username": "newbie"},
			map[string]any{"id": float64(702), "is_bot": true, "username": "helperbot"},
			map[string]any{"id": float64(100), "is_bot": true, "username": "modbot07"},
		))
		reqs := h.fake.RequestsFor("restrictChatMember")
		if len(reqs) != 1 {
			t.Fatalf("expected one new member mute, got %d", len(reqs))
		}
		if got := requestJSONInt(reqs[0].body, "user_id"); got != 701 {
			t.Fatalf("expected user 701 muted, got %d body=%s", got, string(reqs[0].body))
		}
		until := requestJSONInt(reqs[0].body, "until_date")
		if got := until - before; got < 300 || got > 302 {
			t.Fatalf("expected clamped 300s mute, got until delta %d body=%s", got, string(reqs[0].body))
		}
	})

	t.Run("new member mute disabled config does nothing", func(t *testing.T) {
		h := setupE2E(t)
		botID := registerAndManage(h, "mod08:token", "modbot08")
		if err := h.store.SaveModerationChatConfig(models.ModerationChatConfig{BotID: botID, ChatID: 800, Enabled: true, NewMemberMuteEnabled: false, NewMemberMuteSeconds: 300}); err != nil {
			t.Fatalf("SaveModerationChatConfig: %v", err)
		}
		h.InjectUpdate(botID, makeNewMemberUpdate(8, 800, map[string]any{"id": float64(801), "is_bot": false, "username": "newbie"}))
		if got := len(h.fake.RequestsFor("restrictChatMember")); got != 0 {
			t.Fatalf("expected no mute with disabled config, got %d", got)
		}
	})

	t.Run("new member mute skips admins", func(t *testing.T) {
		h := setupE2E(t)
		h.fake.SetHandler("getChatAdministrators", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true,"result":[{"status":"administrator","user":{"id":901,"is_bot":false,"first_name":"Admin","username":"admin"},"can_restrict_members":true}]}`))
		})
		botID := registerAndManage(h, "mod09:token", "modbot09")
		if err := h.store.SaveModerationChatConfig(models.ModerationChatConfig{BotID: botID, ChatID: 900, Enabled: true, NewMemberMuteEnabled: true, NewMemberMuteSeconds: 300}); err != nil {
			t.Fatalf("SaveModerationChatConfig: %v", err)
		}
		h.InjectUpdate(botID, makeNewMemberUpdate(9, 900,
			map[string]any{"id": float64(901), "is_bot": false, "username": "admin"},
			map[string]any{"id": float64(902), "is_bot": false, "username": "member"},
		))
		reqs := h.fake.RequestsFor("restrictChatMember")
		if len(reqs) != 1 {
			t.Fatalf("expected one non-admin mute, got %d", len(reqs))
		}
		if got := requestJSONInt(reqs[0].body, "user_id"); got != 902 {
			t.Fatalf("expected non-admin user 902 muted, got %d body=%s", got, string(reqs[0].body))
		}
	})
}

func jsonNumber(v int64) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func TestE2E_ModerationAILevel2RateLimit(t *testing.T) {
	h := setupE2E(t)
	llm := newFakeLLM(t)
	llm.SetNextRoute(toxicVerdict())
	botID := registerAndManage(h, "mod04:token", "modbot04")
	providerID := saveModerationProvider(t, h, llm.URL(), "local")
	if err := h.store.SaveModerationChatConfig(models.ModerationChatConfig{BotID: botID, ChatID: 100, Enabled: true, AlertChatID: 0, SkipBotMessages: true, IncludeContext: true, RulesEnabled: true, AILevel2Enabled: true, AILevel2ProviderID: providerID, AILevel2MinIntervalSeconds: 3600, AILevel2ContextMinutes: 60, MaxTextLengthForAI: 4000}); err != nil {
		t.Fatalf("SaveModerationChatConfig: %v", err)
	}
	saveModerationRule(t, h, models.ModerationRule{BotID: botID, ChatID: 100, Enabled: true, Kind: "keyword", Pattern: "ambiguous", Category: "other", Severity: "medium", Confidence: 0.8, Mode: "soft", Action: "none"})
	h.InjectUpdate(botID, makeTextUpdate(4, 100, 88, "maybe", "ambiguous"))
	h.InjectUpdate(botID, makeTextUpdate(5, 100, 88, "maybe", "ambiguous"))
	if got := llm.RequestsCountFor("/chat/completions"); got != 0 {
		t.Fatalf("expected AI Level 2 disabled in minimal mode, got %d calls", got)
	}
}
