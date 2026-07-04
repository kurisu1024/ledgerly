package ledgerly

import (
	"strings"
	"testing"
)

func TestNewHandler_HTTPSEventsURL_Accepted(t *testing.T) {
	h, err := NewHandler(fallbackRules(), newSpyHandler(true),
		WithBufferDir(t.TempDir()),
		WithEventsURL("https://ledgerly.example.com/v1/events"),
	)
	if err != nil {
		t.Fatalf("expected an https events URL to be accepted, got %v", err)
	}
	_ = h
}

func TestNewHandler_PlainHTTPEventsURL_Rejected(t *testing.T) {
	_, err := NewHandler(fallbackRules(), newSpyHandler(true),
		WithBufferDir(t.TempDir()),
		WithEventsURL("http://ledgerly.example.com/v1/events"),
	)
	if err == nil {
		t.Fatal("expected a plain-http events URL to a non-loopback host to be rejected, got nil")
	}
	if !strings.Contains(err.Error(), "WithInsecureHTTP") {
		t.Fatalf("expected the rejection to point at the WithInsecureHTTP opt-in, got %q", err)
	}
}

func TestNewHandler_PlainHTTPRulesURL_Rejected(t *testing.T) {
	_, err := NewHandler(fallbackRules(), newSpyHandler(true),
		WithBufferDir(t.TempDir()),
		WithRulesURL("http://ledgerly.example.com/v1/rules"),
	)
	if err == nil {
		t.Fatal("expected a plain-http rules URL to a non-loopback host to be rejected, got nil")
	}
}

func TestNewHandler_PlainHTTPWithInsecureOptIn_Accepted(t *testing.T) {
	_, err := NewHandler(fallbackRules(), newSpyHandler(true),
		WithBufferDir(t.TempDir()),
		WithEventsURL("http://ledgerly.example.com/v1/events"),
		WithInsecureHTTP(),
	)
	if err != nil {
		t.Fatalf("expected a plain-http events URL with WithInsecureHTTP to be accepted, got %v", err)
	}
}

func TestNewHandler_PlainHTTPLoopback_AcceptedWithoutOptIn(t *testing.T) {
	for _, url := range []string{
		"http://localhost:8080/v1/events",
		"http://127.0.0.1:8080/v1/events",
		"http://[::1]:8080/v1/events",
	} {
		_, err := NewHandler(fallbackRules(), newSpyHandler(true),
			WithBufferDir(t.TempDir()),
			WithEventsURL(url),
		)
		if err != nil {
			t.Fatalf("expected the loopback dev-flow URL %q to be accepted without WithInsecureHTTP, got %v", url, err)
		}
	}
}

func TestNewHandler_NonHTTPScheme_Rejected(t *testing.T) {
	_, err := NewHandler(fallbackRules(), newSpyHandler(true),
		WithBufferDir(t.TempDir()),
		WithEventsURL("ftp://ledgerly.example.com/v1/events"),
	)
	if err == nil {
		t.Fatal("expected a non-http(s) events URL to be rejected, got nil")
	}
}
