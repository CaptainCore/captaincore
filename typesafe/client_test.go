package typesafe

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestSystemOneSendsDocumentedShape(t *testing.T) {
	var got map[string]interface{}
	var auth, ctype string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/systemone" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		auth, ctype = r.Header.Get("Authorization"), r.Header.Get("Content-Type")
		json.NewDecoder(r.Body).Decode(&got)
		w.Write([]byte(`{"model":"jev-1.13.0","answers":{
			"urgent":{"type":"noul","noul":0.999},
			"dept":{"type":"choice","choice":"billing","probabilities":{"billing":0.84,"technical":0.16},"confidence":0.596},
			"mood":{"type":"score","score":1.035,"legend":{"0":"Calm","1":"Frustrated"},"confidence":0.842}
		},"usage":{"input_tokens":312,"output_tokens":48}}`))
	}))
	defer srv.Close()

	c := NewClient("k-test")
	c.BaseURL = srv.URL
	resp, err := c.SystemOne(context.Background(), "Help ASAP", Questions{
		"urgent": Noul("Is this urgent?", "Time-sensitive", "Not urgent"),
		"dept":   Choice("Which team?", map[string]string{"billing": "Payments", "technical": "Bugs"}),
		"mood":   Score("How upset?", []string{"Calm", "Frustrated"}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if auth != "Bearer k-test" || ctype != "application/json" {
		t.Errorf("headers: %q %q", auth, ctype)
	}
	if got["model"] != DefaultModel || got["state"] != "Help ASAP" {
		t.Errorf("request body: %v", got)
	}
	qs := got["questions"].(map[string]interface{})
	if qs["urgent"].(map[string]interface{})["criteria"].(map[string]interface{})["true"] != "Time-sensitive" {
		t.Errorf("noul criteria not sent: %v", qs["urgent"])
	}
	if _, ok := qs["mood"].(map[string]interface{})["criteria"].([]interface{}); !ok {
		t.Errorf("score criteria should be an array: %v", qs["mood"])
	}
	if resp.Model != "jev-1.13.0" || resp.Usage.InputTokens != 312 {
		t.Errorf("response meta: %+v", resp)
	}
	if resp.Answers["urgent"].Yes() != 0.999 {
		t.Errorf("noul: %v", resp.Answers["urgent"])
	}
	if d := resp.Answers["dept"]; d.Choice != "billing" || d.Conf() != 0.596 || d.Probabilities["technical"] != 0.16 {
		t.Errorf("choice: %+v", d)
	}
	if m := resp.Answers["mood"]; m.Value() != 1.035 || m.Legend["1"] != "Frustrated" {
		t.Errorf("score: %+v", m)
	}
}

func TestRetriesOn429ThenSucceeds(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			w.Header().Set("Retry-After", "0.01")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":"rate limited"}`))
			return
		}
		w.Write([]byte(`{"model":"jev-1.13.0","answers":{"q":{"type":"noul","noul":0.5}},"usage":{"input_tokens":1}}`))
	}))
	defer srv.Close()

	c := NewClient("k")
	c.BaseURL = srv.URL
	start := time.Now()
	resp, err := c.SystemOne(context.Background(), "x", Questions{"q": Noul("?", "", "")})
	if err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(&calls) != 3 || resp.Answers["q"].Yes() != 0.5 {
		t.Errorf("calls=%d resp=%+v", calls, resp)
	}
	if time.Since(start) > 2*time.Second {
		t.Errorf("Retry-After not honoured, took %s", time.Since(start))
	}
}

func TestGivesUpAfterMaxRetries(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Retry-After", "0.01")
		w.WriteHeader(529)
		w.Write([]byte(`{"error":{"message":"overloaded"}}`))
	}))
	defer srv.Close()

	c := NewClient("k")
	c.BaseURL = srv.URL
	c.MaxRetries = 2
	_, err := c.SystemOne(context.Background(), "x", Questions{"q": Noul("?", "", "")})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 529 {
		t.Fatalf("want APIError 529, got %v", err)
	}
	if atomic.LoadInt32(&calls) != 3 {
		t.Errorf("want 3 attempts, got %d", calls)
	}
	if apiErr.Error() != "typesafe: HTTP 529: overloaded" {
		t.Errorf("message: %q", apiErr.Error())
	}
}

func TestNonRetryableStatusReturnsAtOnce(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"invalid api key"}`))
	}))
	defer srv.Close()

	c := NewClient("bad")
	c.BaseURL = srv.URL
	_, err := c.Models(context.Background())
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 401 || calls != 1 {
		t.Fatalf("want one 401, got calls=%d err=%v", calls, err)
	}
}

func TestNoKeyAndNoQuestions(t *testing.T) {
	c := NewClient("")
	if _, err := c.SystemOne(context.Background(), "x", Questions{"q": Noul("?", "", "")}); !errors.Is(err, ErrNoAPIKey) {
		t.Errorf("want ErrNoAPIKey, got %v", err)
	}
	c = NewClient("k")
	if _, err := c.SystemOne(context.Background(), "x", nil); err == nil {
		t.Error("want error for empty questions")
	}
}

func TestModelsAcceptsWrappedAndBareLists(t *testing.T) {
	for _, body := range []string{
		`{"models":[{"name":"jev-latest","description":"d","release_date":"2026-01-01"}]}`,
		`[{"name":"jev-latest"}]`,
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/models" || r.Method != http.MethodGet {
				t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			}
			w.Write([]byte(body))
		}))
		c := NewClient("k")
		c.BaseURL = srv.URL
		models, err := c.Models(context.Background())
		srv.Close()
		if err != nil || len(models) != 1 || models[0].Name != "jev-latest" {
			t.Errorf("body %s: models=%v err=%v", body, models, err)
		}
	}
}

func TestBackoff(t *testing.T) {
	if d := backoff(0, ""); d != 500*time.Millisecond {
		t.Errorf("attempt 0: %s", d)
	}
	if d := backoff(3, ""); d != 4*time.Second {
		t.Errorf("attempt 3: %s", d)
	}
	if d := backoff(10, ""); d != 20*time.Second {
		t.Errorf("cap: %s", d)
	}
	if d := backoff(0, "2"); d != 2*time.Second {
		t.Errorf("retry-after: %s", d)
	}
	if d := backoff(0, "999"); d != 60*time.Second {
		t.Errorf("retry-after cap: %s", d)
	}
}
