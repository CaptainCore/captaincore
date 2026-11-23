package providers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestKinstaGetRetries429(t *testing.T) {
	var attempts int32
	var sleeps []time.Duration

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		if n == 1 {
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprint(w, `{"message":"rate limited"}`)
			return
		}
		if n == 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprint(w, `{"message":"rate limited"}`)
			return
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer srv.Close()

	origURL := kinstaBaseURL
	origSleep := kinstaSleep
	kinstaBaseURL = srv.URL + "/"
	kinstaSleep = func(d time.Duration) { sleeps = append(sleeps, d) }
	defer func() {
		kinstaBaseURL = origURL
		kinstaSleep = origSleep
	}()

	body, err := kinstaGet("test-key", "sites/123")
	if err != nil {
		t.Fatalf("expected success after 429 retries, got err: %v", err)
	}
	if string(body) != `{"ok":true}` {
		t.Fatalf("unexpected body: %s", string(body))
	}
	if got := atomic.LoadInt32(&attempts); got != 3 {
		t.Fatalf("expected 3 attempts, got %d", got)
	}
	if len(sleeps) != 2 || sleeps[0] != 2*time.Second || sleeps[1] != 1*time.Second {
		t.Fatalf("unexpected sleep durations: %v", sleeps)
	}
}

func TestKinstaGetExhausts429(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"message":"still limited"}`)
	}))
	defer srv.Close()

	origURL := kinstaBaseURL
	origSleep := kinstaSleep
	kinstaBaseURL = srv.URL + "/"
	kinstaSleep = func(d time.Duration) {}
	defer func() {
		kinstaBaseURL = origURL
		kinstaSleep = origSleep
	}()

	_, err := kinstaGet("test-key", "sites/123")
	if err == nil || !strings.Contains(err.Error(), "429") {
		t.Fatalf("expected 429 error after max attempts, got: %v", err)
	}
	if got := atomic.LoadInt32(&attempts); got != 5 {
		t.Fatalf("expected 5 attempts, got %d", got)
	}
}

func TestKinstaEnrichSiteEnvSelectionAndPassword(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/sites/site-uuid":
			fmt.Fprint(w, `{"site":{"name":"exampleuser"}}`)
		case r.URL.Path == "/sites/site-uuid/environments":
			// Staging intentionally listed BEFORE live to verify exact matching.
			fmt.Fprint(w, `{"site":{"environments":[
				{"id":"env-staging","name":"staging","is_premium":false,"ssh_connection":{"ssh_ip":{"external_ip":"10.0.0.2"},"ssh_port":15188},"primaryDomain":{"name":"staging.example.com"},"web_root":"/www/example/public"},
				{"id":"env-live","name":"live","is_premium":false,"ssh_connection":{"ssh_ip":{"external_ip":"10.0.0.1"},"ssh_port":54980},"primaryDomain":{"name":"example.com"},"web_root":"/www/example/public"}
			]}}`)
		case r.URL.Path == "/sites/environments/env-live/ssh/password":
			fmt.Fprint(w, `{"environment":{"sftp_password":"live-secret"}}`)
		case r.URL.Path == "/sites/environments/env-staging/ssh/password":
			fmt.Fprint(w, `{"environment":{"sftp_password":"staging-secret"}}`)
		case strings.HasPrefix(r.URL.Path, "/sites/environments/") && strings.HasSuffix(r.URL.Path, "/analytics/visits"):
			fmt.Fprint(w, `{"app":{"analytics":{"visits":1234}}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	origURL := kinstaBaseURL
	origSleep := kinstaSleep
	kinstaBaseURL = srv.URL + "/"
	kinstaSleep = func(d time.Duration) {}
	defer func() {
		kinstaBaseURL = origURL
		kinstaSleep = origSleep
	}()

	kp := &kinstaProvider{}
	creds := map[string]string{"api": "k", "company_id": "c"}

	prod, err := kp.EnrichSite(creds, RemoteSite{RemoteID: "site-uuid", Environment: "production"})
	if err != nil {
		t.Fatalf("EnrichSite prod failed: %v", err)
	}
	if prod.SSHAddress != "10.0.0.1" || prod.SSHPort != "54980" || prod.SSHUsername != "exampleuser" || prod.SSHPassword != "live-secret" {
		t.Fatalf("unexpected prod enriched: %+v", prod)
	}

	stg, err := kp.EnrichSite(creds, RemoteSite{RemoteID: "site-uuid", Environment: "staging"})
	if err != nil {
		t.Fatalf("EnrichSite staging failed: %v", err)
	}
	if stg.SSHAddress != "10.0.0.2" || stg.SSHPort != "15188" || stg.SSHPassword != "staging-secret" {
		t.Fatalf("unexpected staging enriched: %+v", stg)
	}
}

func TestKinstaEnrichSiteMissingLiveDoesNotFallbackToStaging(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sites/site-uuid":
			fmt.Fprint(w, `{"site":{"name":"exampleuser"}}`)
		case "/sites/site-uuid/environments":
			fmt.Fprint(w, `{"site":{"environments":[
				{"id":"env-staging","name":"staging","is_premium":false,"ssh_connection":{"ssh_ip":{"external_ip":"10.0.0.2"},"ssh_port":15188}}
			]}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	origURL := kinstaBaseURL
	kinstaBaseURL = srv.URL + "/"
	defer func() { kinstaBaseURL = origURL }()

	kp := &kinstaProvider{}
	prod, err := kp.EnrichSite(map[string]string{"api": "k"}, RemoteSite{RemoteID: "site-uuid", Environment: "production"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if prod.SSHAddress != "" || prod.SSHPort != "" {
		t.Fatalf("expected empty prod SSH when live env missing, got %s:%s", prod.SSHAddress, prod.SSHPort)
	}
}
