// Package typesafe is a small client for the TypeSafe System One API
// (https://docs.typesafe.ai/api). Jev, the model behind it, is not a language
// model: it reads a state (text or JSON) once and answers a set of typed
// questions in parallel, returning probabilities rather than prose. Code owns
// the workflow; the model supplies a judgment where a regex cannot.
//
// The API key comes from system.typesafe_api_key in config.json, with the
// TYPESAFE_API_KEY environment variable as the fallback (the same variable the
// official SDKs read).
package typesafe

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"time"
)

const (
	// DefaultBaseURL is the production API origin.
	DefaultBaseURL = "https://api.typesafe.ai"
	// DefaultModel is the alias TypeSafe documents. Pin a versioned ID
	// (jev-1.13.0) once thresholds have been tuned against it.
	DefaultModel = "jev-latest"
	// EnvAPIKey is the environment variable consulted when config.json has no key.
	EnvAPIKey = "TYPESAFE_API_KEY"

	defaultTimeout = 30 * time.Second
	defaultRetries = 4
)

// Client calls the System One endpoint.
type Client struct {
	APIKey     string
	BaseURL    string
	Model      string
	HTTP       *http.Client
	MaxRetries int // retries on 429 and 529, with exponential backoff
}

// NewClient returns a client for apiKey with production defaults.
func NewClient(apiKey string) *Client {
	return &Client{
		APIKey:     apiKey,
		BaseURL:    DefaultBaseURL,
		Model:      DefaultModel,
		HTTP:       &http.Client{Timeout: defaultTimeout},
		MaxRetries: defaultRetries,
	}
}

// Question is one entry of the questions map. Instructions and Criteria may be
// strings or JSON objects/arrays; the API accepts both.
type Question struct {
	Type         string      `json:"type"`
	Instructions interface{} `json:"instructions"`
	Criteria     interface{} `json:"criteria,omitempty"`
}

// Questions is the map sent in a request, keyed by an id of the caller's choice.
type Questions map[string]Question

// Noul builds a yes/no question. yes and no describe what each pole means and
// may be empty.
func Noul(instructions string, yes, no string) Question {
	q := Question{Type: "noul", Instructions: instructions}
	if yes != "" || no != "" {
		c := map[string]string{}
		if yes != "" {
			c["true"] = yes
		}
		if no != "" {
			c["false"] = no
		}
		q.Criteria = c
	}
	return q
}

// Choice builds a pick-one question over options, each with a rubric description.
func Choice(instructions string, options map[string]string) Question {
	return Question{Type: "choice", Instructions: instructions, Criteria: options}
}

// Score builds a rubric question over ordered levels; the answer is a
// probability-weighted index into levels.
func Score(instructions string, levels []string) Question {
	return Question{Type: "score", Instructions: instructions, Criteria: levels}
}

// Answer is one entry of the answers map. Only the fields for its Type are set.
type Answer struct {
	Type          string             `json:"type"`
	Noul          *float64           `json:"noul,omitempty"`
	Choice        string             `json:"choice,omitempty"`
	Score         *float64           `json:"score,omitempty"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
	Legend        map[string]string  `json:"legend,omitempty"`
	Confidence    *float64           `json:"confidence,omitempty"`
}

// Yes returns the noul probability, or 0 when the answer is not a noul.
func (a Answer) Yes() float64 {
	if a.Noul == nil {
		return 0
	}
	return *a.Noul
}

// Conf returns the confidence, or 0 when the answer carries none.
func (a Answer) Conf() float64 {
	if a.Confidence == nil {
		return 0
	}
	return *a.Confidence
}

// Value returns the score, or 0 when the answer is not a score.
func (a Answer) Value() float64 {
	if a.Score == nil {
		return 0
	}
	return *a.Score
}

// Usage is the token accounting the API returns. Output tokens are free.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// Response is the body of a successful System One call. Model is the versioned
// ID that answered, which is what to log when an alias was requested.
type Response struct {
	Model   string            `json:"model"`
	Answers map[string]Answer `json:"answers"`
	Usage   Usage             `json:"usage"`
}

// Model is one row of GET /v1/models.
type Model struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	ReleaseDate string `json:"release_date,omitempty"`
}

// APIError is a non-2xx response after retries are exhausted.
type APIError struct {
	Status int
	Body   string
}

func (e *APIError) Error() string {
	msg := e.Body
	var parsed struct {
		Error   interface{} `json:"error"`
		Message string      `json:"message"`
	}
	if json.Unmarshal([]byte(e.Body), &parsed) == nil {
		switch v := parsed.Error.(type) {
		case string:
			msg = v
		case map[string]interface{}:
			if m, ok := v["message"].(string); ok {
				msg = m
			}
		}
		if parsed.Message != "" && msg == e.Body {
			msg = parsed.Message
		}
	}
	if len(msg) > 300 {
		msg = msg[:300] + "…"
	}
	return fmt.Sprintf("typesafe: HTTP %d: %s", e.Status, msg)
}

// ErrNoAPIKey is returned when the client has no key to send.
var ErrNoAPIKey = errors.New("typesafe: no API key (set system.typesafe_api_key in config.json or " + EnvAPIKey + ")")

// SystemOne evaluates questions against state with the client's default model.
func (c *Client) SystemOne(ctx context.Context, state interface{}, questions Questions) (*Response, error) {
	return c.SystemOneWithModel(ctx, c.Model, state, questions)
}

// SystemOneWithModel is SystemOne with an explicit model name or alias.
func (c *Client) SystemOneWithModel(ctx context.Context, model string, state interface{}, questions Questions) (*Response, error) {
	if c.APIKey == "" {
		return nil, ErrNoAPIKey
	}
	if len(questions) == 0 {
		return nil, errors.New("typesafe: no questions")
	}
	if model == "" {
		model = DefaultModel
	}
	body, err := json.Marshal(map[string]interface{}{
		"state":     state,
		"model":     model,
		"questions": questions,
	})
	if err != nil {
		return nil, fmt.Errorf("typesafe: marshal request: %w", err)
	}
	raw, err := c.do(ctx, http.MethodPost, "/v1/systemone", body)
	if err != nil {
		return nil, err
	}
	var out Response
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("typesafe: decode response: %w", err)
	}
	return &out, nil
}

// Models lists the model names the account can send.
func (c *Client) Models(ctx context.Context) ([]Model, error) {
	if c.APIKey == "" {
		return nil, ErrNoAPIKey
	}
	raw, err := c.do(ctx, http.MethodGet, "/v1/models", nil)
	if err != nil {
		return nil, err
	}
	// The documented shape is {"models": [...]}; accept a bare array too.
	var wrapped struct {
		Models []Model `json:"models"`
	}
	if err := json.Unmarshal(raw, &wrapped); err == nil && wrapped.Models != nil {
		return wrapped.Models, nil
	}
	var list []Model
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("typesafe: decode models: %w", err)
	}
	return list, nil
}

// do performs one API call with retries on 429 and 529, honouring Retry-After.
func (c *Client) do(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultTimeout}
	}
	base := c.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	var last error
	for attempt := 0; attempt <= c.MaxRetries; attempt++ {
		var rdr io.Reader
		if body != nil {
			rdr = bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, base+path, rdr)
		if err != nil {
			return nil, fmt.Errorf("typesafe: build request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
		req.Header.Set("Accept", "application/json")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("typesafe: %w", err)
		}
		data, readErr := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("typesafe: read response: %w", readErr)
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return data, nil
		}
		last = &APIError{Status: resp.StatusCode, Body: string(data)}
		if resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode != 529 {
			return nil, last
		}
		if attempt == c.MaxRetries {
			break
		}
		wait := backoff(attempt, resp.Header.Get("Retry-After"))
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}
	return nil, last
}

// backoff is 500ms, 1s, 2s, 4s… capped at 20s, or the Retry-After seconds
// when the server sent one.
func backoff(attempt int, retryAfter string) time.Duration {
	if retryAfter != "" {
		if secs, err := strconv.ParseFloat(retryAfter, 64); err == nil && secs > 0 {
			return time.Duration(math.Min(secs, 60) * float64(time.Second))
		}
	}
	d := time.Duration(500*math.Pow(2, float64(attempt))) * time.Millisecond
	if d > 20*time.Second {
		d = 20 * time.Second
	}
	return d
}
