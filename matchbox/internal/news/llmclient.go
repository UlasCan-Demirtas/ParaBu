package news

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type LLMClient struct {
	apiKey string
	apiURL string
	client *http.Client
}

func NewLLMClient(apiKey string) *LLMClient {
	return &LLMClient{
		apiKey: apiKey,
		// Using Gemini 3.8 Flash via REST API
		apiURL: fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-3.8-flash:generateContent?key=%s", apiKey),
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

type SentimentResult struct {
	Sentiment  string  `json:"sentiment"`
	Confidence float64 `json:"confidence"`
	Impact     int     `json:"impact_score"`
}

func (c *LLMClient) Analyze(ctx context.Context, title, content string) (*SentimentResult, error) {
	prompt := fmt.Sprintf(`Analyze the following cryptocurrency news and return a strict JSON response.
Format: {"sentiment": "BULLISH"|"BEARISH"|"NEUTRAL", "confidence": 0.0-1.0, "impact_score": 1-5}
Rules:
- Only return the JSON. No markdown tags like ` + "```json" + `.
- If it's unrelated to crypto/finance, set sentiment to NEUTRAL and impact_score to 1.
- Confidence must be a float between 0.0 and 1.0.

Title: %s
Content: %s
`, title, content)

	reqBody := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]interface{}{
					{"text": prompt},
				},
			},
		},
		"generationConfig": map[string]interface{}{
			"temperature":      0.1,
			"responseMimeType": "application/json",
		},
	}

	b, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, "POST", c.apiURL, bytes.NewBuffer(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		return nil, fmt.Errorf("rate limited (429)")
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("api error %d: %s", resp.StatusCode, string(body))
	}

	var resData struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&resData); err != nil {
		return nil, err
	}

	if len(resData.Candidates) == 0 || len(resData.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("empty response from llm")
	}

	text := resData.Candidates[0].Content.Parts[0].Text

	// Clean markdown block if LLM ignored instructions
	text = strings.TrimPrefix(strings.TrimSpace(text), "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")

	var sr SentimentResult
	if err := json.Unmarshal([]byte(text), &sr); err != nil {
		return nil, fmt.Errorf("failed to parse json: %w. text: %s", err, text)
	}

	// Validation Fallbacks (Graceful Degradation)
	sr.Sentiment = strings.ToUpper(sr.Sentiment)
	if sr.Sentiment != "BULLISH" && sr.Sentiment != "BEARISH" && sr.Sentiment != "NEUTRAL" {
		sr.Sentiment = "NEUTRAL"
	}
	if sr.Impact < 1 {
		sr.Impact = 1
	}
	if sr.Impact > 5 {
		sr.Impact = 5
	}
	if sr.Confidence < 0.0 || sr.Confidence > 1.0 {
		sr.Confidence = 0.5
	}

	return &sr, nil
}
