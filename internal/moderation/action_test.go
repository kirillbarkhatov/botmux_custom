package moderation

import (
	"testing"
	"time"

	"github.com/skrashevich/botmux/internal/models"
)

type actionBotStub struct {
	muteUntil int64
}

func (b *actionBotStub) SendMessageGetID(chatID int64, text string) (int, error) { return 0, nil }
func (b *actionBotStub) DeleteMessage(chatID int64, messageID int) error         { return nil }
func (b *actionBotStub) MuteUser(chatID int64, userID int64, untilUnix int64) error {
	b.muteUntil = untilUnix
	return nil
}
func (b *actionBotStub) BanUserUntil(chatID int64, userID int64, untilUnix int64) error {
	return nil
}
func (b *actionBotStub) GetAdmins(chatID int64) ([]models.AdminInfo, error) { return nil, nil }
func (b *actionBotStub) GetSelfID() int64                                   { return 999 }

func TestApplyActionMuteDurationNormalization(t *testing.T) {
	tests := []struct {
		name     string
		input    int64
		expected int64
	}{
		{name: "zero means indefinite", input: 0, expected: 0},
		{name: "below telegram temporary limit means indefinite", input: 30, expected: 0},
		{name: "one minute preserved", input: 60, expected: 60},
		{name: "long duration preserved", input: 7200, expected: 7200},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bot := &actionBotStub{}
			event := &models.ModerationEvent{}
			before := time.Now().Unix()
			err := (&Service{}).applyAction(
				&models.ModerationChatConfig{},
				Message{ChatID: -100, UserID: 42},
				event,
				models.ModerationChatLevel{Action: "mute", DurationSeconds: tc.input},
				nil,
				bot,
			)
			if err != nil {
				t.Fatalf("applyAction: %v", err)
			}
			gotDuration := int64(0)
			if bot.muteUntil > 0 {
				gotDuration = bot.muteUntil - before
			}
			if tc.expected == 0 && bot.muteUntil != 0 {
				t.Fatalf("mute until = %d, want indefinite without until_date", bot.muteUntil)
			}
			if tc.expected > 0 && (gotDuration < tc.expected || gotDuration > tc.expected+2) {
				t.Fatalf("mute until duration = %d, want about %d", gotDuration, tc.expected)
			}
			if event.ActionDurationSeconds != tc.expected {
				t.Fatalf("event duration = %d, want %d", event.ActionDurationSeconds, tc.expected)
			}
			wantResult := "muted in source chat"
			if tc.expected > 0 {
				wantResult = "muted in source chat for " + itoa(tc.expected) + "s"
			}
			if event.ActionResult != wantResult {
				t.Fatalf("unexpected action result: %q want %q", event.ActionResult, wantResult)
			}
		})
	}
}

func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
