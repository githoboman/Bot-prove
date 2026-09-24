package decision

import (
	"strings"
	"testing"
)

func TestBuildRecord_HashesRequestAndResponse(t *testing.T) {
	rec := BuildRecord(
		"rec-1", "agent-A", "gpt-x", "v1.2",
		[]byte("hello world"),
		[]byte("hi"),
		nil,
		VerdictAllow, "low", "policy-default", "",
		false,
	)
	if rec.RequestHash == "" || len(rec.RequestHash) != 64 {
		t.Fatalf("RequestHash wrong: %q", rec.RequestHash)
	}
	if rec.ResponseHash == "" || len(rec.ResponseHash) != 64 {
		t.Fatalf("ResponseHash wrong: %q", rec.ResponseHash)
	}
	if rec.TracePreview != "" {
		t.Fatalf("TracePreview must stay empty when PreviewOptIn=false")
	}
	if !VerifyRecord(rec) {
		t.Fatalf("chain root fails verification on freshly built record")
	}
}

func TestBuildRecord_PreviewOptIn(t *testing.T) {
	rec := BuildRecord(
		"rec-2", "agent-A", "m", "v",
		[]byte("this is a long enough prompt to survive truncation checks"),
		nil, nil,
		VerdictAllow, "low", "p", "",
		true,
	)
	if rec.TracePreview == "" {
		t.Fatalf("TracePreview must be populated when PreviewOptIn=true")
	}
}

func TestRedactMetadata_HidesSecrets(t *testing.T) {
	md := map[string]string{
		"model_provider": "openai",
		"API_KEY":        "sk-abcdef",
		"auth_token":     "xoxb-...",
		"session_id":     "keep-visible",
		"user_email":     "alex@example.com",
	}
	out := redactMetadata(md)
	if !strings.HasPrefix(out["API_KEY"], "<redacted:sha256:") {
		t.Fatalf("API_KEY not redacted: %q", out["API_KEY"])
	}
	if !strings.HasPrefix(out["auth_token"], "<redacted:sha256:") {
		t.Fatalf("auth_token not redacted (should match 'token' substring): %q", out["auth_token"])
	}
	if !strings.HasPrefix(out["user_email"], "<redacted:sha256:") {
		t.Fatalf("user_email not redacted (should match 'email' substring): %q", out["user_email"])
	}
	if out["model_provider"] != "openai" {
		t.Fatalf("non-sensitive key must survive verbatim, got %q", out["model_provider"])
	}
	if out["session_id"] != "keep-visible" {
		t.Fatalf("session_id must survive verbatim, got %q", out["session_id"])
	}
}

func TestChainRoot_DetectsTampering(t *testing.T) {
	rec := BuildRecord("r", "a", "m", "v", []byte("hi"), []byte("ok"), nil, VerdictAllow, "low", "p", "", false)
	if !VerifyRecord(rec) {
		t.Fatalf("baseline record should verify")
	}
	tampered := rec
	tampered.Verdict = VerdictReject
	if VerifyRecord(tampered) {
		t.Fatalf("mutated verdict must invalidate chain root")
	}
}

func TestInMemorySink_LineageWalksParents(t *testing.T) {
	sink := NewInMemorySink(0)
	root := BuildRecord("root", "a", "m", "v", []byte("q0"), []byte("a0"), nil, VerdictAllow, "low", "p", "", false)
	mid := BuildRecord("mid", "a", "m", "v", []byte("q1"), []byte("a1"), nil, VerdictAllow, "low", "p", "root", false)
	leaf := BuildRecord("leaf", "a", "m", "v", []byte("q2"), []byte("a2"), nil, VerdictHITL, "high", "p", "mid", false)
	for _, r := range []Record{root, mid, leaf} {
		if err := sink.Append(r); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	chain, err := sink.Lineage("leaf", 10)
	if err != nil {
		t.Fatalf("lineage: %v", err)
	}
	if len(chain) != 3 {
		t.Fatalf("expected chain of 3, got %d", len(chain))
	}
	if chain[0].ID != "root" || chain[1].ID != "mid" || chain[2].ID != "leaf" {
		t.Fatalf("chain order wrong: %+v", []string{chain[0].ID, chain[1].ID, chain[2].ID})
	}
}

func TestInMemorySink_RingBufferEviction(t *testing.T) {
	sink := NewInMemorySink(2)
	for i, id := range []string{"a", "b", "c"} {
		_ = i
		_ = sink.Append(BuildRecord(id, "ag", "m", "v", nil, nil, nil, VerdictAllow, "low", "p", "", false))
	}
	recent, _ := sink.Recent(10)
	if len(recent) != 2 {
		t.Fatalf("ring buffer should retain 2, got %d", len(recent))
	}
	if _, ok, _ := sink.Get("a"); ok {
		t.Fatalf("oldest record 'a' should have been evicted")
	}
}
