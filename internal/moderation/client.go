package moderation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/skrashevich/botmux/internal/models"
)

type Client struct{}

type LLMResult struct {
	Verdict   models.ModerationVerdict
	Raw       string
	LatencyMS int64
}

func (c *Client) Classify(ctx context.Context, p models.ModerationProvider, systemPrompt, userPrompt string) (*LLMResult, error) {
	timeout := time.Duration(p.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result, err := c.call(ctx, p, systemPrompt, userPrompt, true)
	if err == nil {
		return result, nil
	}
	if strings.Contains(err.Error(), "response_format") || strings.Contains(err.Error(), "400") {
		return c.call(ctx, p, systemPrompt, userPrompt, false)
	}
	return nil, err
}

func (c *Client) call(ctx context.Context, p models.ModerationProvider, systemPrompt, userPrompt string, responseFormat bool) (*LLMResult, error) {
	apiURL := strings.TrimRight(p.APIURL, "/") + "/chat/completions"
	reqBody := map[string]any{
		"model": p.Model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
		"temperature": 0,
	}
	if responseFormat {
		reqBody["response_format"] = map[string]string{"type": "json_object"}
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if p.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.APIKey)
	}
	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("moderation provider status %d: %s", resp.StatusCode, string(body))
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("moderation provider returned no choices")
	}
	content := parsed.Choices[0].Message.Content
	jsonText, err := extractJSONObject(content)
	if err != nil {
		return nil, err
	}
	var verdict models.ModerationVerdict
	if err := json.Unmarshal([]byte(jsonText), &verdict); err != nil {
		return nil, fmt.Errorf("parse moderation JSON %q: %w", jsonText, err)
	}
	normalizeVerdict(&verdict)
	return &LLMResult{Verdict: verdict, Raw: content, LatencyMS: latency}, nil
}

func extractJSONObject(s string) (string, error) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```json")
		s = strings.TrimPrefix(s, "```")
		s = strings.TrimSuffix(s, "```")
		s = strings.TrimSpace(s)
	}
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end < start {
		return "", fmt.Errorf("no JSON object in response")
	}
	return s[start : end+1], nil
}

func normalizeVerdict(v *models.ModerationVerdict) {
	if v.Severity == "" {
		v.Severity = "none"
	}
	if v.Category == "" {
		v.Category = "none"
	}
	if v.Confidence < 0 {
		v.Confidence = 0
	}
	if v.Confidence > 1 {
		v.Confidence = 1
	}
}
