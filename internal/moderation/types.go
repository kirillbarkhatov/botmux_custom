package moderation

import "github.com/skrashevich/botmux/internal/models"

type Message struct {
	BotID     int64
	ChatID    int64
	MessageID int
	UserID    int64
	Username  string
	Text      string
	Date      int64
	FromIsBot bool
	ChatTitle string
}

type ActionBot interface {
	SendMessageGetID(chatID int64, text string) (int, error)
	MuteUser(chatID int64, userID int64, untilUnix int64) error
	BanUserUntil(chatID int64, userID int64, untilUnix int64) error
	GetAdmins(chatID int64) ([]models.AdminInfo, error)
	GetSelfID() int64
}
