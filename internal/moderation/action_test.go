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
		{name: "zero uses default", input: 0, expected: 300},
		{name: "below min clamps up", input: 60, expected: 300},
		{name: "above max clamps down", input: 7200, expected: 3600},
		{name: "valid duration preserved", input: 1800, expected: 1800},
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
			gotDuration := bot.muteUntil - before
			if gotDuration < tc.expected || gotDuration > tc.expected+2 {
				t.Fatalf("mute until duration = %d, want about %d", gotDuration, tc.expected)
			}
			if event.ActionDurationSeconds != tc.expected {
				t.Fatalf("event duration = %d, want %d", event.ActionDurationSeconds, tc.expected)
			}
			if event.ActionResult != "muted in source chat for "+itoa(tc.expected)+"s" {
				t.Fatalf("unexpected action result: %q", event.ActionResult)
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
