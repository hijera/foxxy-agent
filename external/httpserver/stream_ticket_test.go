//go:build http

package httpserver

import (
	"net/http"
	"sync"
	"testing"
	"time"
)

func TestStreamTicketExpires(t *testing.T) {
	store := newStreamTicketStore()
	now := time.Now()
	ticket, expires, err := store.mint(now)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	if !expires.After(now) {
		t.Fatalf("expiry %v is not after mint time %v", expires, now)
	}
	if store.consume(ticket, now.Add(streamTicketTTL)) {
		t.Fatal("a ticket was accepted exactly at its expiry")
	}
	if store.consume(ticket, now.Add(time.Second)) {
		t.Fatal("an expired ticket was still accepted after the pruning pass")
	}
}

func TestStreamTicketIsSingleUse(t *testing.T) {
	store := newStreamTicketStore()
	now := time.Now()
	ticket, _, err := store.mint(now)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	if !store.consume(ticket, now) {
		t.Fatal("first use rejected")
	}
	if store.consume(ticket, now) {
		t.Fatal("second use accepted; the ticket is not single-use")
	}
}

// Two subscriptions racing on the same ticket must not both get in.
func TestStreamTicketConcurrentConsumeAdmitsOne(t *testing.T) {
	store := newStreamTicketStore()
	now := time.Now()
	ticket, _, err := store.mint(now)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	const racers = 8
	var wg sync.WaitGroup
	results := make([]bool, racers)
	wg.Add(racers)
	for i := range racers {
		go func(idx int) {
			defer wg.Done()
			results[idx] = store.consume(ticket, now)
		}(i)
	}
	wg.Wait()
	accepted := 0
	for _, ok := range results {
		if ok {
			accepted++
		}
	}
	if accepted != 1 {
		t.Fatalf("%d racers admitted, want exactly 1", accepted)
	}
}

func TestStreamTicketUnknownValueRejected(t *testing.T) {
	store := newStreamTicketStore()
	now := time.Now()
	if _, _, err := store.mint(now); err != nil {
		t.Fatalf("mint: %v", err)
	}
	if store.consume("not-a-ticket", now) {
		t.Fatal("an unrelated value was accepted as a ticket")
	}
	if store.consume("", now) {
		t.Fatal("an empty credential was accepted as a ticket")
	}
}

func TestStreamTicketStoreEvictsOldestAtCap(t *testing.T) {
	store := newStreamTicketStore()
	now := time.Now()
	first, _, err := store.mint(now)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	// Every later ticket expires after the first, so the first is the eviction target.
	for i := 1; i <= streamTicketMax; i++ {
		if _, _, err := store.mint(now.Add(time.Duration(i) * time.Millisecond)); err != nil {
			t.Fatalf("mint %d: %v", i, err)
		}
	}
	if store.consume(first, now) {
		t.Fatal("the oldest ticket survived the cap; the store can grow without bound")
	}
	if len(store.tickets) > streamTicketMax {
		t.Fatalf("store holds %d tickets, cap is %d", len(store.tickets), streamTicketMax)
	}
}

// The endpoint is pointless without auth, and answering 200 there would suggest
// the stream is protected when every SSE route is in fact wide open.
func TestStreamTicketMintRequiresAuthEnabled(t *testing.T) {
	_, ts := authTestServer(t, cfgWithAuth("")) // no token configured => auth off
	res, err := http.Post(ts.URL+"/foxxycode/stream-tickets", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("mint with auth disabled: %d, want 409", res.StatusCode)
	}
}

func TestStreamTicketMintSetsNoStore(t *testing.T) {
	_, ts := authTestServer(t, cfgWithAuth("primary-secret"))
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/foxxycode/stream-tickets", nil)
	req.Header.Set("Authorization", "Bearer primary-secret")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if got := res.Header.Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store: a credential must not be cached", got)
	}
}

// With stream_tickets_only the durable token stops working in a URL, which is the
// whole point: nothing that reaches an access log is a lasting credential.
func TestStreamTicketsOnlyRejectsDurableTokenInQuery(t *testing.T) {
	_, ts := authTestServer(t, cfgWithAuthTicketsOnly("primary-secret"))
	sid := "sess_deadbeefdeadbeef"
	base := ts.URL + "/foxxycode/sessions/" + sid + "/composer-stream"
	if got := authGET(t, base+"?access_token=primary-secret", ""); got != http.StatusUnauthorized {
		t.Fatalf("durable token in query under stream_tickets_only: %d, want 401", got)
	}
	// The header remains the supported way for a non-EventSource client.
	if got := authGET(t, base, "primary-secret"); got == http.StatusUnauthorized {
		t.Fatal("Authorization header rejected under stream_tickets_only")
	}
}

// Default mode keeps the documented behaviour so existing EventSource clients
// are not broken by the upgrade.
func TestStreamTicketsDefaultStillAcceptsDurableToken(t *testing.T) {
	_, ts := authTestServer(t, cfgWithAuth("primary-secret"))
	sid := "sess_deadbeefdeadbeef"
	base := ts.URL + "/foxxycode/sessions/" + sid + "/composer-stream"
	if got := authGET(t, base+"?access_token=primary-secret", ""); got == http.StatusUnauthorized {
		t.Fatal("durable token in query rejected in the default mode")
	}
}
