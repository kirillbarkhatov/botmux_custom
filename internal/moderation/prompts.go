package moderation

const Level1SystemPrompt = `You are a Telegram chat moderation classifier.

Classify ONLY the target message.
Detect direct insults, harassment, threats, hate, humiliation, discriminatory attacks, sexual harassment, spam, and abusive behavior.

Do NOT mark as toxic:
- neutral disagreement
- criticism without personal attack
- profanity not aimed at a person
- quoted toxic text
- jokes without a clear victim
- moderation/admin messages

Return JSON only with this schema:
{
  "toxic": true/false,
  "severity": "none|low|medium|high",
  "category": "none|insult|harassment|threat|hate|sexual|spam|other",
  "confidence": 0.0-1.0,
  "needs_context": true/false,
  "context_dependent": false,
  "needs_human_review": true/false,
  "reason": "short explanation"
}`

const Level1UserPrompt = `Target message:
{{message_text}}

Sender:
{{username}} ({{user_id}})

Moderated chat:
{{chat_title}} ({{chat_id}})`

const Level2SystemPrompt = `You are a Telegram chat moderation classifier.

Analyze the TARGET message using RECENT CONTEXT.
The target may be toxic only because of previous messages.
Look for repeated targeting, escalation, veiled insults, dogpiling, threats, harassment, discrimination, or coordinated abuse.

Do NOT mark as toxic if the user is:
- defending themselves without abuse
- quoting someone else's insult
- making a neutral factual statement
- joking mutually without a clear victim
- responding to moderation instructions

Return JSON only with this schema:
{
  "toxic": true/false,
  "severity": "none|low|medium|high",
  "category": "none|insult|harassment|threat|hate|sexual|spam|other",
  "confidence": 0.0-1.0,
  "needs_context": false,
  "context_dependent": true/false,
  "needs_human_review": true/false,
  "reason": "short explanation"
}`

const Level2UserPrompt = `Recent context:
{{recent_context}}

Target message:
{{target_message}}

Sender:
{{username}} ({{user_id}})

Moderated chat:
{{chat_title}} ({{chat_id}})`

const Level3SystemPrompt = `You are a careful second-opinion moderation reviewer.

You receive previous moderation verdicts and chat context.
Your job is to resolve uncertain cases.
Be conservative with automatic punishments.
Prefer needs_human_review=true when evidence is ambiguous.

Return JSON only with this schema:
{
  "toxic": true/false,
  "severity": "none|low|medium|high",
  "category": "none|insult|harassment|threat|hate|sexual|spam|other",
  "confidence": 0.0-1.0,
  "needs_context": false,
  "context_dependent": true/false,
  "needs_human_review": true/false,
  "reason": "short explanation"
}`

const Level3UserPrompt = `Previous verdicts:
{{level_results}}

Recent context:
{{recent_context}}

Target message:
{{target_message}}

Sender:
{{username}} ({{user_id}})

Moderated chat:
{{chat_title}} ({{chat_id}})`

func DefaultLevel(level int, botID, chatID, providerID int64, openAIProviderID int64) (l LevelDefaults) {
	switch level {
	case 1:
		return LevelDefaults{Level: 1, Name: "Level 1: single message", Enabled: true, Required: true, ProviderID: providerID, SystemPrompt: Level1SystemPrompt, UserPrompt: Level1UserPrompt, MinConfidence: 0.70, TriggerSeverity: "medium", Action: "alert"}
	case 2:
		return LevelDefaults{Level: 2, Name: "Level 2: context", Enabled: true, Required: true, ProviderID: providerID, SystemPrompt: Level2SystemPrompt, UserPrompt: Level2UserPrompt, MinConfidence: 0.70, TriggerSeverity: "medium", Action: "alert"}
	default:
		enabled := openAIProviderID != 0
		return LevelDefaults{Level: 3, Name: "Level 3: external review", Enabled: enabled, Required: false, OnlyIfUncertain: true, ProviderID: openAIProviderID, SystemPrompt: Level3SystemPrompt, UserPrompt: Level3UserPrompt, MinConfidence: 0.70, TriggerSeverity: "medium", Action: "alert"}
	}
}

type LevelDefaults struct {
	Level           int
	Name            string
	Enabled         bool
	Required        bool
	OnlyIfUncertain bool
	ProviderID      int64
	SystemPrompt    string
	UserPrompt      string
	MinConfidence   float64
	TriggerSeverity string
	Action          string
}
