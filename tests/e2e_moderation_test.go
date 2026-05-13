package tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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

func TestE2E_Moderation(t *testing.T) {
	t.Run("enabled chat sends alert with same bot and deduplicates", func(t *testing.T) {
		h := setupE2E(t)
		llm := newFakeLLM(t)
		llm.SetNextRoute(toxicVerdict())
		botID := registerAndManage(h, "mod01:token", "modbot01")
		saveModerationProvider(t, h, llm.URL(), "local")
		_ = h.store.UpsertChat(botID, models.Chat{ID: 100, Type: "supergroup", Title: "Source", UpdatedAt: time.Now().Format(time.RFC3339)})
		_ = h.store.UpsertChat(botID, models.Chat{ID: 200, Type: "supergroup", Title: "Alerts", UpdatedAt: time.Now().Format(time.RFC3339)})
		if err := h.store.SaveModerationChatConfig(models.ModerationChatConfig{BotID: botID, ChatID: 100, Enabled: true, AlertChatID: 200, SkipBotMessages: true, IncludeContext: true, ContextMessagesLimit: 30, ContextMinutes: 30}); err != nil {
			t.Fatalf("SaveModerationChatConfig: %v", err)
		}

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
		if cnt := llm.RequestsCountFor("/chat/completions"); cnt != 2 {
			t.Fatalf("expected L1+L2 only for enabled chat, got %d LLM calls", cnt)
		}
		eventsA, _ := h.store.ListModerationEvents(botID, 10, 0, "", "", "", "", "", 10, 0)
		eventsB, _ := h.store.ListModerationEvents(botID, 11, 0, "", "", "", "", "", 10, 0)
		if len(eventsA) != 1 || len(eventsB) != 0 {
			t.Fatalf("unexpected events: A=%d B=%d", len(eventsA), len(eventsB))
		}
	})

	t.Run("provider API masks key and alert chats are current bot only", func(t *testing.T) {
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
		var providers []models.ModerationProvider
		_ = json.NewDecoder(resp.Body).Decode(&providers)
		if len(providers) == 0 || providers[0].APIKey != "" || providers[0].APIKeyMasked == "" {
			t.Fatalf("provider key was not masked: %+v", providers)
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
		providerID := saveModerationProvider(t, h, llm.URL(), "local")
		if err := h.store.SaveModerationChatConfig(models.ModerationChatConfig{BotID: botID, ChatID: 100, Enabled: true, AlertChatID: 200, SkipBotMessages: true, IncludeContext: true}); err != nil {
			t.Fatalf("SaveModerationChatConfig: %v", err)
		}
		for _, level := range []int{1, 2} {
			if err := h.store.SaveModerationLevel(models.ModerationChatLevel{
				BotID: botID, ChatID: 100, Level: level, Name: "Level", Enabled: true, ProviderID: providerID, Required: true,
				SystemPrompt: "Return JSON", UserPromptTemplate: "{{message_text}}", MinConfidence: 0.7, TriggerSeverity: "medium", Action: "mute", DurationSeconds: 3600,
			}); err != nil {
				t.Fatalf("SaveModerationLevel: %v", err)
			}
		}
		h.InjectUpdate(botID, makeTextUpdate(5, 100, 55, "mute_me", "bad"))
		reqs := h.fake.RequestsFor("restrictChatMember")
		if len(reqs) != 1 || !strings.Contains(string(reqs[0].body), `"chat_id":100`) && !strings.Contains(string(reqs[0].body), "chat_id=100") {
			events, _ := h.store.ListModerationEvents(botID, 100, 0, "", "", "", "", "", 10, 0)
			t.Fatalf("expected mute in source chat 100, got %+v events=%+v", reqs, events)
		}
	})

	t.Run("ban action targets source chat", func(t *testing.T) {
		h := setupE2E(t)
		llm := newFakeLLM(t)
		llm.SetNextRoute(toxicVerdict())
		botID := registerAndManage(h, "mod06:token", "modbot06")
		providerID := saveModerationProvider(t, h, llm.URL(), "local")
		if err := h.store.SaveModerationChatConfig(models.ModerationChatConfig{BotID: botID, ChatID: 300, Enabled: true, AlertChatID: 400, SkipBotMessages: true, IncludeContext: true}); err != nil {
			t.Fatalf("SaveModerationChatConfig: %v", err)
		}
		for _, level := range []int{1, 2} {
			if err := h.store.SaveModerationLevel(models.ModerationChatLevel{
				BotID: botID, ChatID: 300, Level: level, Name: "Level", Enabled: true, ProviderID: providerID, Required: true,
				SystemPrompt: "Return JSON", UserPromptTemplate: "{{message_text}}", MinConfidence: 0.7, TriggerSeverity: "medium", Action: "ban", DurationSeconds: 3600,
			}); err != nil {
				t.Fatalf("SaveModerationLevel: %v", err)
			}
		}
		h.InjectUpdate(botID, makeTextUpdate(6, 300, 66, "ban_me", "bad"))
		reqs := h.fake.RequestsFor("banChatMember")
		if len(reqs) != 1 || !strings.Contains(string(reqs[0].body), `"chat_id":300`) && !strings.Contains(string(reqs[0].body), "chat_id=300") {
			t.Fatalf("expected ban in source chat 300, got %+v", reqs)
		}
	})
}

func jsonNumber(v int64) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func TestE2E_ModerationUncertainCallsLevel3(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		n := calls
		mu.Unlock()
		verdict := cleanVerdict()
		if n == 1 || n == 3 {
			verdict = toxicVerdict()
		}
		data, _ := json.Marshal(verdict)
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"message": map[string]string{"content": string(data)}}}})
	}))
	defer srv.Close()

	h := setupE2E(t)
	botID := registerAndManage(h, "mod04:token", "modbot04")
	saveModerationProvider(t, h, srv.URL, "local")
	saveModerationProvider(t, h, srv.URL, "openai")
	if err := h.store.SaveModerationChatConfig(models.ModerationChatConfig{BotID: botID, ChatID: 100, Enabled: true, AlertChatID: 0, SkipBotMessages: true, IncludeContext: true}); err != nil {
		t.Fatalf("SaveModerationChatConfig: %v", err)
	}
	h.InjectUpdate(botID, makeTextUpdate(4, 100, 88, "maybe", "ambiguous"))
	mu.Lock()
	got := calls
	mu.Unlock()
	if got != 3 {
		t.Fatalf("expected L1, L2, and uncertain L3 calls, got %d", got)
	}
}
