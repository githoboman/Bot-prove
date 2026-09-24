package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestWebhookRegisterAndFireDelivery(t *testing.T) {
	// Receiver counts hits and verifies signature.
	var hits int64
	var lastSig string
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		lastSig = r.Header.Get("X-CP-Signature")
		body, _ := io.ReadAll(r.Body)
		if !VerifyWebhookSignature("s3cret", body, lastSig) {
			t.Errorf("signature verification failed: got %q body %q", lastSig, string(body))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer receiver.Close()

	store := newWebhookStore()
	sub, err := store.register("owner-hash", receiver.URL, "s3cret", []string{"proof.verified"})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"id": "42"})
	added := store.enqueue("proof.verified", body)
	if added != 1 {
		t.Fatalf("enqueue count: got %d want 1", added)
	}
	// Deliver.
	attempted := store.deliverOnce(context.Background())
	if attempted != 1 {
		t.Fatalf("deliverOnce: got %d want 1", attempted)
	}
	if atomic.LoadInt64(&hits) != 1 {
		t.Fatalf("receiver hits: got %d want 1", hits)
	}
	// Stats mutated correctly.
	subs := store.list("owner-hash")
	if len(subs) != 1 {
		t.Fatalf("list: got %d want 1", len(subs))
	}
	if subs[0].Deliveries != 1 || subs[0].Attempts != 1 || subs[0].Failures != 0 {
		t.Fatalf("stats: %+v", subs[0])
	}
	if subs[0].ID != sub.ID {
		t.Fatalf("id mismatch")
	}
}

func TestWebhookRetryOnFailure(t *testing.T) {
	// Receiver returns 500 twice, then 200.
	var hits int64
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt64(&hits, 1)
		if n < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer receiver.Close()

	store := newWebhookStore()
	fixedNow := time.Now()
	store.now = func() time.Time { return fixedNow }
	if _, err := store.register("h", receiver.URL, "s", []string{"proof.verified"}); err != nil {
		t.Fatal(err)
	}
	store.enqueue("proof.verified", []byte("{}"))
	// First attempt fails.
	store.deliverOnce(context.Background())
	// Fast-forward past the 1s backoff.
	store.now = func() time.Time { return fixedNow.Add(2 * time.Second) }
	store.deliverOnce(context.Background())
	// Fast-forward past the 2s backoff.
	store.now = func() time.Time { return fixedNow.Add(10 * time.Second) }
	store.deliverOnce(context.Background())
	if atomic.LoadInt64(&hits) != 3 {
		t.Fatalf("expected 3 hits, got %d", hits)
	}
	subs := store.list("h")
	if subs[0].Deliveries != 1 || subs[0].Failures != 2 {
		t.Fatalf("stats: %+v", subs[0])
	}
}

func TestWebhookDeadLetter(t *testing.T) {
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer receiver.Close()

	store := newWebhookStore()
	fixedNow := time.Now()
	store.now = func() time.Time { return fixedNow }
	if _, err := store.register("h", receiver.URL, "s", []string{"proof.verified"}); err != nil {
		t.Fatal(err)
	}
	store.enqueue("proof.verified", []byte("{}"))
	// 8 attempts; between each, warp past the backoff so the event
	// is ready to redeliver.
	for i := 0; i < webhookMaxAttempts; i++ {
		store.now = func() time.Time { return fixedNow.Add(time.Duration(i+1) * time.Hour) }
		store.deliverOnce(context.Background())
	}
	dl := store.deadLetters()
	if len(dl) != 1 {
		t.Fatalf("dead letters: got %d want 1", len(dl))
	}
	if dl[0].Event.Attempts != webhookMaxAttempts {
		t.Fatalf("attempts on dl: got %d want %d", dl[0].Event.Attempts, webhookMaxAttempts)
	}
}

func TestWebhookReplayDeadLetter(t *testing.T) {
	// Receiver fails until we replay, then succeeds. Verifies that:
	//   • 8 failing attempts push the event to dead-letters
	//   • The dead letter has a stable delivery id and OwnerKey
	//   • replay() re-enqueues it and removes it from dead-letters
	//   • The re-enqueued event delivers with byte-identical body
	//     (signature must still verify — no re-marshal)
	var allowSuccess int64
	var hits int64
	var lastSigOK int64
	var lastBody []byte
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		body, _ := io.ReadAll(r.Body)
		lastBody = body
		if VerifyWebhookSignature("s", body, r.Header.Get("X-CP-Signature")) {
			atomic.StoreInt64(&lastSigOK, 1)
		}
		if atomic.LoadInt64(&allowSuccess) == 1 {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer receiver.Close()

	store := newWebhookStore()
	fixedNow := time.Now()
	store.now = func() time.Time { return fixedNow }
	if _, err := store.register("owner-a", receiver.URL, "s", []string{"proof.verified"}); err != nil {
		t.Fatal(err)
	}
	originalBody := []byte(`{"id":"replay-me"}`)
	store.enqueue("proof.verified", originalBody)

	// Exhaust the retry budget.
	for i := 0; i < webhookMaxAttempts; i++ {
		store.now = func() time.Time { return fixedNow.Add(time.Duration(i+1) * time.Hour) }
		store.deliverOnce(context.Background())
	}
	dls := store.deadLetters()
	if len(dls) != 1 {
		t.Fatalf("expected 1 dead letter, got %d", len(dls))
	}
	dl := dls[0]
	if dl.DeliveryID == "" || !strings.HasPrefix(dl.DeliveryID, "dl_") {
		t.Fatalf("delivery id: %q", dl.DeliveryID)
	}
	if dl.OwnerKey != "owner-a" {
		t.Fatalf("owner: %q", dl.OwnerKey)
	}

	// Wrong owner cannot replay.
	if _, err := store.replay(dl.DeliveryID, "owner-b"); err == nil {
		t.Fatal("replay from wrong owner must fail")
	}

	// Right owner replays — flip the receiver to success first.
	atomic.StoreInt64(&allowSuccess, 1)
	replayedID, err := store.replay(dl.DeliveryID, "owner-a")
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replayedID != dl.DeliveryID {
		t.Fatalf("replay id: got %q want %q", replayedID, dl.DeliveryID)
	}
	if remaining := store.deadLetters(); len(remaining) != 0 {
		t.Fatalf("expected dead letter to be removed, got %d", len(remaining))
	}

	hitsBeforeReplay := atomic.LoadInt64(&hits)
	store.now = func() time.Time { return fixedNow.Add(100 * time.Hour) }
	store.deliverOnce(context.Background())
	if atomic.LoadInt64(&hits) != hitsBeforeReplay+1 {
		t.Fatalf("expected one more receiver hit after replay")
	}
	if !bytes.Equal(lastBody, originalBody) {
		t.Fatalf("replay must send byte-identical body: got %q want %q", lastBody, originalBody)
	}
	if atomic.LoadInt64(&lastSigOK) != 1 {
		t.Fatal("HMAC signature must still verify on replay")
	}

	// Second replay of the same id must fail — it is gone now.
	if _, err := store.replay(dl.DeliveryID, "owner-a"); err == nil {
		t.Fatal("replaying a already-replayed id must fail")
	}
}

func TestWebhookReplayUnregisteredSubscription(t *testing.T) {
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer receiver.Close()

	store := newWebhookStore()
	fixedNow := time.Now()
	store.now = func() time.Time { return fixedNow }
	sub, err := store.register("owner-a", receiver.URL, "s", []string{"proof.verified"})
	if err != nil {
		t.Fatal(err)
	}
	store.enqueue("proof.verified", []byte("{}"))
	for i := 0; i < webhookMaxAttempts; i++ {
		store.now = func() time.Time { return fixedNow.Add(time.Duration(i+1) * time.Hour) }
		store.deliverOnce(context.Background())
	}
	if err := store.unregister(sub.ID, "owner-a"); err != nil {
		t.Fatal(err)
	}
	dls := store.deadLetters()
	if len(dls) != 1 {
		t.Fatalf("dl count: %d", len(dls))
	}
	if _, err := store.replay(dls[0].DeliveryID, "owner-a"); err == nil {
		t.Fatal("replaying event for unregistered subscription must fail")
	}
}

func TestWebhookReplayHTTPHandler(t *testing.T) {
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer receiver.Close()

	srv := newTestServer("")
	srv.webhooks = newWebhookStore()
	ownerReq := httptest.NewRequest(http.MethodGet, "/", nil)
	ownerReq.Header.Set("X-API-Key", "key-a")
	ownerA := callerHash(ownerReq)
	if _, err := srv.webhooks.register(ownerA, receiver.URL, "s", []string{"proof.verified"}); err != nil {
		t.Fatal(err)
	}
	srv.webhooks.enqueue("proof.verified", []byte("{}"))
	fixedNow := time.Now()
	for i := 0; i < webhookMaxAttempts; i++ {
		srv.webhooks.now = func() time.Time { return fixedNow.Add(time.Duration(i+1) * time.Hour) }
		srv.webhooks.deliverOnce(context.Background())
	}
	dls := srv.webhooks.deadLetters()
	if len(dls) != 1 {
		t.Fatalf("dl count: %d", len(dls))
	}
	did := dls[0].DeliveryID

	// Caller B cannot replay caller A's dead letter.
	req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/dead-letters/"+did+"/replay", nil)
	req.Header.Set("X-API-Key", "key-b")
	req.SetPathValue("delivery_id", did)
	rec := httptest.NewRecorder()
	srv.webhooksReplayDeadLetter(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-owner replay must 404, got %d", rec.Code)
	}

	// Caller A can.
	req = httptest.NewRequest(http.MethodPost, "/v1/webhooks/dead-letters/"+did+"/replay", nil)
	req.Header.Set("X-API-Key", "key-a")
	req.SetPathValue("delivery_id", did)
	rec = httptest.NewRecorder()
	srv.webhooksReplayDeadLetter(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("replay: got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp webhookReplayResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.DeliveryID != did || resp.Status != "enqueued" {
		t.Fatalf("replay body: %+v", resp)
	}
	if remaining := srv.webhooks.deadLetters(); len(remaining) != 0 {
		t.Fatalf("dead letter should be gone, got %d", len(remaining))
	}
}

func TestWebhookOwnerIsolation(t *testing.T) {
	store := newWebhookStore()
	if _, err := store.register("owner-a", "http://a.example", "", []string{"proof.verified"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.register("owner-b", "http://b.example", "", []string{"proof.verified"}); err != nil {
		t.Fatal(err)
	}
	if got := len(store.list("owner-a")); got != 1 {
		t.Fatalf("owner-a: got %d want 1", got)
	}
	if got := len(store.list("owner-b")); got != 1 {
		t.Fatalf("owner-b: got %d want 1", got)
	}
	if got := len(store.list("owner-c")); got != 0 {
		t.Fatalf("owner-c: got %d want 0", got)
	}
}

func TestWebhookUnknownEventRejected(t *testing.T) {
	store := newWebhookStore()
	_, err := store.register("h", "http://a.example", "", []string{"totally.made.up"})
	if err == nil {
		t.Fatal("expected error for unknown event")
	}
	if !strings.Contains(err.Error(), "unknown event") {
		t.Fatalf("wrong error: %v", err)
	}
}

func TestWebhookSignatureConstantTime(t *testing.T) {
	body := []byte("hello world")
	sig := signWebhook("secret", body)
	if !VerifyWebhookSignature("secret", body, sig) {
		t.Fatal("valid sig rejected")
	}
	if VerifyWebhookSignature("secret", body, "sha256=00") {
		t.Fatal("bad sig accepted")
	}
	if VerifyWebhookSignature("", body, sig) {
		t.Fatal("empty secret accepted")
	}
}

func TestWebhookHTTPHandlers(t *testing.T) {
	// Directly exercise the handlers — the mux is wired identically
	// in Start(); we just need the handler methods themselves to
	// produce the right side effects on the store.
	srv := newTestServer("")
	srv.webhooks = newWebhookStore()

	// Register.
	body, _ := json.Marshal(webhookRegisterRequest{
		URL:    "https://example.invalid/hook",
		Secret: "sh",
		Events: []string{"proof.verified"},
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/webhooks", bytes.NewReader(body))
	req.Header.Set("X-API-Key", "test-key")
	rec := httptest.NewRecorder()
	srv.webhooksCreate(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register: got %d body=%s", rec.Code, rec.Body.String())
	}
	var regResp webhookRegisterResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &regResp); err != nil {
		t.Fatal(err)
	}
	subID := regResp.Subscription.ID

	// List.
	req = httptest.NewRequest(http.MethodGet, "/v1/webhooks", nil)
	req.Header.Set("X-API-Key", "test-key")
	rec = httptest.NewRecorder()
	srv.webhooksList(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: got %d", rec.Code)
	}
	var listResp webhookListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &listResp); err != nil {
		t.Fatal(err)
	}
	if listResp.Count != 1 || listResp.Subscriptions[0].ID != subID {
		t.Fatalf("list mismatch: %+v", listResp)
	}

	// Delete — need to fake path value.
	req = httptest.NewRequest(http.MethodDelete, "/v1/webhooks/"+subID, nil)
	req.Header.Set("X-API-Key", "test-key")
	req.SetPathValue("id", subID)
	rec = httptest.NewRecorder()
	srv.webhooksDelete(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: got %d body=%s", rec.Code, rec.Body.String())
	}

	// Owner scoping — list from a different key must return zero.
	req = httptest.NewRequest(http.MethodGet, "/v1/webhooks", nil)
	req.Header.Set("X-API-Key", "other-key")
	rec = httptest.NewRecorder()
	srv.webhooksList(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list other: got %d", rec.Code)
	}
	var l2 webhookListResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &l2)
	if l2.Count != 0 {
		t.Fatalf("expected owner isolation: %+v", l2)
	}
}
