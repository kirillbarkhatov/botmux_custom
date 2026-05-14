package models

// OnMessageSentFunc is called when a bot sends a message (for bridge outgoing notifications)
type OnMessageSentFunc func(botID int64, chatID int64, text string, msgID int, replyToMsgID int)

// BotConfig represents a bot in the unified bots table
type BotConfig struct {
	ID               int64  `json:"id"`
	Name             string `json:"name"`
	Token            string `json:"token"`
	BotUsername      string `json:"bot_username"`
	ManageEnabled    bool   `json:"manage_enabled"`
	ProxyEnabled     bool   `json:"proxy_enabled"`
	BackendURL       string `json:"backend_url"`
	SecretToken      string `json:"secret_token"`
	PollingTimeout   int    `json:"polling_timeout"`
	Offset           int64  `json:"offset"`
	LastError        string `json:"last_error,omitempty"`
	LastActivity     string `json:"last_activity,omitempty"`
	UpdatesForwarded int64  `json:"updates_forwarded"`
	Source           string `json:"source"` // "cli" or "web"
	BackendStatus    string `json:"backend_status"`
	BackendCheckedAt string `json:"backend_checked_at"`
	LongPollEnabled  bool   `json:"long_poll_enabled"`
	Disabled         bool   `json:"disabled"`
}

type Chat struct {
	ID          int64  `json:"id"`
	Type        string `json:"type"`
	Title       string `json:"title"`
	Username    string `json:"username"`
	MemberCount int    `json:"member_count"`
	Description string `json:"description"`
	IsAdmin     bool   `json:"is_admin"`
	UpdatedAt   string `json:"updated_at"`
	LastMsgText string `json:"last_msg_text,omitempty"`
	LastMsgFrom string `json:"last_msg_from,omitempty"`
	LastMsgDate int64  `json:"last_msg_date,omitempty"`
}

type Message struct {
	ID        int    `json:"id"`
	BotID     int64  `json:"bot_id"`
	ChatID    int64  `json:"chat_id"`
	FromUser  string `json:"from_user"`
	FromID    int64  `json:"from_id"`
	Text      string `json:"text"`
	Date      int64  `json:"date"`
	DateStr   string `json:"date_str"`
	ReplyToID int    `json:"reply_to_id,omitempty"`
	Deleted   bool   `json:"deleted"`
	MediaType string `json:"media_type,omitempty"`
	FileID    string `json:"file_id,omitempty"`
	FromIsBot bool   `json:"from_is_bot,omitempty"`
	SenderTag string `json:"sender_tag,omitempty"`
}

type ChatStats struct {
	ChatID        int64          `json:"chat_id"`
	Title         string         `json:"title"`
	TotalMessages int            `json:"total_messages"`
	TodayMessages int            `json:"today_messages"`
	ActiveUsers   int            `json:"active_users"`
	TopUsers      []UserActivity `json:"top_users"`
	HourlyStats   []HourlyStat   `json:"hourly_stats"`
}

type UserActivity struct {
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
	Count    int    `json:"count"`
}

type HourlyStat struct {
	Hour  int `json:"hour"`
	Count int `json:"count"`
}

type AdminLog struct {
	ID         int64  `json:"id"`
	ChatID     int64  `json:"chat_id"`
	Action     string `json:"action"`
	ActorName  string `json:"actor_name"`
	TargetID   int64  `json:"target_id,omitempty"`
	TargetName string `json:"target_name,omitempty"`
	Details    string `json:"details,omitempty"`
	CreatedAt  string `json:"created_at"`
}

type UserTag struct {
	ID       int64  `json:"id"`
	ChatID   int64  `json:"chat_id"`
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
	Tag      string `json:"tag"`
	Color    string `json:"color"`
}

type ChatUser struct {
	UserID       int64     `json:"user_id"`
	Username     string    `json:"username"`
	MessageCount int       `json:"message_count"`
	LastSeen     string    `json:"last_seen"`
	Tags         []UserTag `json:"tags"`
}

// RouteMapping tracks source↔target message pairs for reverse routing (Source-NAT)
type RouteMapping struct {
	ID           int64  `json:"id"`
	RouteID      int64  `json:"route_id"`
	SourceBotID  int64  `json:"source_bot_id"`
	SourceChatID int64  `json:"source_chat_id"`
	SourceMsgID  int    `json:"source_msg_id"`
	TargetBotID  int64  `json:"target_bot_id"`
	TargetChatID int64  `json:"target_chat_id"`
	TargetMsgID  int    `json:"target_msg_id"`
	CreatedAt    string `json:"created_at"`
}

// Route defines a routing rule: updates matching conditions on source bot get forwarded to target bot
type Route struct {
	ID             int64  `json:"id"`
	SourceBotID    int64  `json:"source_bot_id"`
	TargetBotID    int64  `json:"target_bot_id"`
	SourceChatID   int64  `json:"source_chat_id"`
	ConditionType  string `json:"condition_type"`
	ConditionValue string `json:"condition_value"`
	Action         string `json:"action"`
	TargetChatID   int64  `json:"target_chat_id"`
	Enabled        bool   `json:"enabled"`
	Description    string `json:"description"`
	CreatedAt      string `json:"created_at"`
}

type AdminInfo struct {
	UserID             int64  `json:"user_id"`
	Username           string `json:"username"`
	Status             string `json:"status"`
	CustomTitle        string `json:"custom_title"`
	CanDeleteMessages  bool   `json:"can_delete_messages"`
	CanRestrictMembers bool   `json:"can_restrict_members"`
	CanPromoteMembers  bool   `json:"can_promote_members"`
	CanChangeInfo      bool   `json:"can_change_info"`
	CanInviteUsers     bool   `json:"can_invite_users"`
	CanPinMessages     bool   `json:"can_pin_messages"`
	CanManageChat      bool   `json:"can_manage_chat"`
}

// AuthUser represents an authenticated user
type AuthUser struct {
	ID                 int64  `json:"id"`
	Username           string `json:"username"`
	PasswordHash       string `json:"-"`
	DisplayName        string `json:"display_name"`
	Role               string `json:"role"` // "admin" or "user"
	MustChangePassword bool   `json:"must_change_password"`
	CreatedAt          string `json:"created_at"`
	LastLogin          string `json:"last_login"`
}

// LLMConfig holds configuration for the LLM routing service
type LLMConfig struct {
	ID           int64  `json:"id"`
	APIURL       string `json:"api_url"`
	APIKey       string `json:"api_key"`
	Model        string `json:"model"`
	SystemPrompt string `json:"system_prompt"`
	Enabled      bool   `json:"enabled"`
}

// LLMRouteResult holds the routing decision from the LLM
type LLMRouteResult struct {
	TargetBotID  int64  `json:"target_bot_id"`
	TargetChatID int64  `json:"target_chat_id"`
	Action       string `json:"action"`
	Reason       string `json:"reason"`
}

// ModerationChatConfig stores moderation settings for exactly one bot/chat pair.
type ModerationChatConfig struct {
	ID                         int64  `json:"id"`
	BotID                      int64  `json:"bot_id"`
	ChatID                     int64  `json:"chat_id"`
	Enabled                    bool   `json:"enabled"`
	AlertChatID                int64  `json:"alert_chat_id"`
	SkipBotMessages            bool   `json:"skip_bot_messages"`
	IncludeContext             bool   `json:"include_context"`
	ContextMessagesLimit       int    `json:"context_messages_limit"`
	ContextMinutes             int    `json:"context_minutes"`
	RulesEnabled               bool   `json:"rules_enabled"`
	AILevel2Enabled            bool   `json:"ai_level2_enabled"`
	AILevel2ProviderID         int64  `json:"ai_level2_provider_id"`
	AILevel2MinIntervalSeconds int    `json:"ai_level2_min_interval_seconds"`
	AILevel2ContextMinutes     int    `json:"ai_level2_context_minutes"`
	AILevel2LastRunAt          string `json:"ai_level2_last_run_at"`
	LogCleanMessages           bool   `json:"log_clean_messages"`
	MaxTextLengthForAI         int    `json:"max_text_length_for_ai"`
	NewMemberMuteEnabled       bool   `json:"new_member_mute_enabled"`
	NewMemberMuteSeconds       int64  `json:"new_member_mute_seconds"`
	NewMemberCanSendMessages   bool   `json:"new_member_can_send_messages"`
	NewMemberCanSendAudios     bool   `json:"new_member_can_send_audios"`
	NewMemberCanSendDocuments  bool   `json:"new_member_can_send_documents"`
	NewMemberCanSendPhotos     bool   `json:"new_member_can_send_photos"`
	NewMemberCanSendVideos     bool   `json:"new_member_can_send_videos"`
	NewMemberCanSendVideoNotes bool   `json:"new_member_can_send_video_notes"`
	NewMemberCanSendVoiceNotes bool   `json:"new_member_can_send_voice_notes"`
	NewMemberCanSendOther      bool   `json:"new_member_can_send_other_messages"`
	NewMemberCanAddWebPreviews bool   `json:"new_member_can_add_web_page_previews"`
	NewMemberCanSendPolls      bool   `json:"new_member_can_send_polls"`
	NewMemberCanInviteUsers    bool   `json:"new_member_can_invite_users"`
	NewMemberCanPinMessages    bool   `json:"new_member_can_pin_messages"`
	NewMemberCanChangeInfo     bool   `json:"new_member_can_change_info"`
	NewMemberCanManageTopics   bool   `json:"new_member_can_manage_topics"`
	CreatedAt                  string `json:"created_at"`
	UpdatedAt                  string `json:"updated_at"`
}

type ModerationCategorySetting struct {
	ID              int64  `json:"id"`
	BotID           int64  `json:"bot_id"`
	ChatID          int64  `json:"chat_id"`
	CategoryKey     string `json:"category_key"`
	Enabled         bool   `json:"enabled"`
	AlertEnabled    bool   `json:"alert_enabled"`
	MuteMinutes     int64  `json:"mute_minutes"`
	BanHours        int64  `json:"ban_hours"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
	RulesCount      int    `json:"rules_count,omitempty"`
	EnabledRules    int    `json:"enabled_rules,omitempty"`
	DisplayName     string `json:"display_name,omitempty"`
	SeveritySummary string `json:"severity_summary,omitempty"`
}

type ModerationRule struct {
	ID              int64   `json:"id"`
	BotID           int64   `json:"bot_id"`
	ChatID          int64   `json:"chat_id"`
	Enabled         bool    `json:"enabled"`
	Language        string  `json:"language"`
	Kind            string  `json:"kind"`
	Pattern         string  `json:"pattern"`
	Category        string  `json:"category"`
	Severity        string  `json:"severity"`
	Confidence      float64 `json:"confidence"`
	Action          string  `json:"action"`
	DurationSeconds int64   `json:"duration_seconds"`
	Mode            string  `json:"mode"`
	Notes           string  `json:"notes"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
}

type ModerationRuleMatch struct {
	RuleID          int64   `json:"rule_id"`
	BotID           int64   `json:"bot_id"`
	ChatID          int64   `json:"chat_id"`
	Scope           string  `json:"scope"`
	Language        string  `json:"language"`
	Kind            string  `json:"kind"`
	Pattern         string  `json:"pattern"`
	Category        string  `json:"category"`
	Severity        string  `json:"severity"`
	Confidence      float64 `json:"confidence"`
	Action          string  `json:"action"`
	DurationSeconds int64   `json:"duration_seconds"`
	Mode            string  `json:"mode"`
	Notes           string  `json:"notes"`
	Error           string  `json:"error,omitempty"`
}

type ModerationPrefilterResult struct {
	NormalizedText  string                `json:"normalized_text"`
	Decision        string                `json:"decision"`
	MatchedRules    []ModerationRuleMatch `json:"matched_rules"`
	Severity        string                `json:"severity"`
	Category        string                `json:"category"`
	Confidence      float64               `json:"confidence"`
	Action          string                `json:"action"`
	DurationSeconds int64                 `json:"duration_seconds"`
	Reason          string                `json:"reason"`
}

type ModerationProvider struct {
	ID             int64  `json:"id"`
	Name           string `json:"name"`
	Kind           string `json:"kind"`
	APIURL         string `json:"api_url"`
	APIKey         string `json:"api_key,omitempty"`
	APIKeyMasked   string `json:"api_key_masked,omitempty"`
	Model          string `json:"model"`
	Enabled        bool   `json:"enabled"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

type ModerationChatLevel struct {
	ID                 int64   `json:"id"`
	BotID              int64   `json:"bot_id"`
	ChatID             int64   `json:"chat_id"`
	Level              int     `json:"level"`
	Name               string  `json:"name"`
	Enabled            bool    `json:"enabled"`
	ProviderID         int64   `json:"provider_id"`
	Required           bool    `json:"required"`
	OnlyIfUncertain    bool    `json:"only_if_uncertain"`
	SystemPrompt       string  `json:"system_prompt"`
	UserPromptTemplate string  `json:"user_prompt_template"`
	MinConfidence      float64 `json:"min_confidence"`
	TriggerSeverity    string  `json:"trigger_severity"`
	Action             string  `json:"action"`
	DurationSeconds    int64   `json:"duration_seconds"`
	CreatedAt          string  `json:"created_at"`
	UpdatedAt          string  `json:"updated_at"`
}

type ModerationVerdict struct {
	Toxic            bool    `json:"toxic"`
	Severity         string  `json:"severity"`
	Category         string  `json:"category"`
	Confidence       float64 `json:"confidence"`
	NeedsContext     bool    `json:"needs_context"`
	ContextDependent bool    `json:"context_dependent"`
	NeedsHumanReview bool    `json:"needs_human_review"`
	Reason           string  `json:"reason"`
}

type ModerationEvent struct {
	ID                    int64                   `json:"id"`
	BotID                 int64                   `json:"bot_id"`
	ChatID                int64                   `json:"chat_id"`
	MessageID             int                     `json:"message_id"`
	UserID                int64                   `json:"user_id"`
	Username              string                  `json:"username"`
	MessageText           string                  `json:"message_text"`
	MessageDate           int64                   `json:"message_date"`
	Status                string                  `json:"status"`
	FinalToxic            bool                    `json:"final_toxic"`
	FinalSeverity         string                  `json:"final_severity"`
	FinalCategory         string                  `json:"final_category"`
	FinalConfidence       float64                 `json:"final_confidence"`
	FinalReason           string                  `json:"final_reason"`
	FinalAction           string                  `json:"final_action"`
	ActionDurationSeconds int64                   `json:"action_duration_seconds"`
	ActionResult          string                  `json:"action_result"`
	ActionError           string                  `json:"action_error"`
	AlertSent             bool                    `json:"alert_sent"`
	AlertChatID           int64                   `json:"alert_chat_id"`
	AlertMessageID        int                     `json:"alert_message_id"`
	CreatedAt             string                  `json:"created_at"`
	LevelResults          []ModerationLevelResult `json:"level_results,omitempty"`
}

type ModerationLevelResult struct {
	ID               int64   `json:"id"`
	EventID          int64   `json:"event_id"`
	Level            int     `json:"level"`
	ProviderID       int64   `json:"provider_id"`
	ProviderKind     string  `json:"provider_kind"`
	Model            string  `json:"model"`
	Toxic            bool    `json:"toxic"`
	Severity         string  `json:"severity"`
	Category         string  `json:"category"`
	Confidence       float64 `json:"confidence"`
	NeedsContext     bool    `json:"needs_context"`
	ContextDependent bool    `json:"context_dependent"`
	NeedsHumanReview bool    `json:"needs_human_review"`
	Reason           string  `json:"reason"`
	RawResponse      string  `json:"raw_response"`
	Error            string  `json:"error"`
	LatencyMS        int64   `json:"latency_ms"`
	CreatedAt        string  `json:"created_at"`
}

type ModerationTestRequest struct {
	BotID         int64  `json:"bot_id"`
	ChatID        int64  `json:"chat_id"`
	MessageText   string `json:"message_text"`
	RecentContext string `json:"recent_context"`
	UserID        int64  `json:"user_id"`
	Username      string `json:"username"`
}

type ModerationRuleTestRequest struct {
	BotID  int64  `json:"bot_id"`
	ChatID int64  `json:"chat_id"`
	Text   string `json:"text"`
}

type ModerationRuleTestResponse struct {
	NormalizedText   string                `json:"normalized_text"`
	Decision         string                `json:"decision"`
	MatchedRules     []ModerationRuleMatch `json:"matched_rules"`
	WouldRunAILevel2 bool                  `json:"would_run_ai_level2"`
	AIRateLimited    bool                  `json:"ai_rate_limited"`
	NextAIAllowedAt  string                `json:"next_ai_allowed_at"`
}

type ModerationTestResponse struct {
	FinalVerdict ModerationVerdict       `json:"final_verdict"`
	FinalAction  string                  `json:"final_action"`
	Uncertain    bool                    `json:"uncertain"`
	Results      []ModerationLevelResult `json:"results"`
}

// BridgeConfig represents a protocol bridge in the database
type BridgeConfig struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Protocol     string `json:"protocol"`
	LinkedBotID  int64  `json:"linked_bot_id"`
	Config       string `json:"config"`
	CallbackURL  string `json:"callback_url"`
	Enabled      bool   `json:"enabled"`
	CreatedAt    string `json:"created_at"`
	LastActivity string `json:"last_activity,omitempty"`
	LastError    string `json:"last_error,omitempty"`
}

// BridgeIncomingMessage is the simple format external sources POST to us
type BridgeIncomingMessage struct {
	ExternalChatID string `json:"chat_id"`
	ExternalUserID string `json:"user_id"`
	Username       string `json:"username"`
	Text           string `json:"text"`
	ExternalMsgID  string `json:"message_id"`
	ReplyToMsgID   string `json:"reply_to"`
}

// BridgeOutgoingMessage is what we POST back to the bridge callback
type BridgeOutgoingMessage struct {
	BridgeID       int64  `json:"bridge_id"`
	ExternalChatID string `json:"chat_id"`
	Text           string `json:"text"`
	TelegramMsgID  int    `json:"telegram_msg_id"`
	ReplyToExtID   string `json:"reply_to,omitempty"`
}

// BridgeChatMapping tracks external_chat_id <-> telegram_chat_id
type BridgeChatMapping struct {
	BridgeID       int64  `json:"bridge_id"`
	ExternalChatID string `json:"external_chat_id"`
	TelegramChatID int64  `json:"telegram_chat_id"`
}

// BridgeMsgMapping tracks external_msg_id <-> telegram_msg_id for reply threading
type BridgeMsgMapping struct {
	BridgeID       int64  `json:"bridge_id"`
	ExternalMsgID  string `json:"external_msg_id"`
	TelegramMsgID  int    `json:"telegram_msg_id"`
	TelegramChatID int64  `json:"telegram_chat_id"`
}

// VersionInfo holds build-time version information
type VersionInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"build_date"`
}

// UpdateCheck holds the result of a version update check
type UpdateCheck struct {
	Current         string `json:"current_version"`
	Latest          string `json:"latest_version,omitempty"`
	UpdateAvailable bool   `json:"update_available"`
	ReleaseURL      string `json:"release_url,omitempty"`
	CheckedAt       string `json:"checked_at,omitempty"`
	Error           string `json:"error,omitempty"`
}

// QueuedUpdate holds a single raw Telegram update with its update_id.
type QueuedUpdate struct {
	UpdateID int64
	Data     map[string]any
}
