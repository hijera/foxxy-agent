package web

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/kuhahalong/ddgsearch"
)

// ---- parseGoogleResults unit tests ----

func TestParseGoogleResults_DirectHTTPSLink(t *testing.T) {
	body := []byte(`<html><body>
<div class="g">
  <a href="https://example.com/page">
    <h3>Example Title</h3>
  </a>
</div>
</body></html>`)
	got, err := parseGoogleResults(body, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 result, got %d", len(got))
	}
	if got[0].Title != "Example Title" {
		t.Errorf("title: %q", got[0].Title)
	}
	if got[0].URL != "https://example.com/page" {
		t.Errorf("url: %q", got[0].URL)
	}
}

func TestParseGoogleResults_DecodesRedirectURL(t *testing.T) {
	body := []byte(`<html><body>
<div class="g">
  <a href="/url?q=https://example.com/path%3Fkey%3Dval&amp;sa=U&amp;ved=abc">
    <h3>Title Two</h3>
  </a>
</div>
</body></html>`)
	got, err := parseGoogleResults(body, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(got), got)
	}
	if got[0].URL != "https://example.com/path?key=val" {
		t.Errorf("url: %q", got[0].URL)
	}
	if got[0].Title != "Title Two" {
		t.Errorf("title: %q", got[0].Title)
	}
}

func TestParseGoogleResults_SkipsGoogleOwnURLs(t *testing.T) {
	body := []byte(`<html><body>
<div class="g">
  <a href="https://www.google.com/search?q=foo">
    <h3>Google's own page</h3>
  </a>
</div>
<div class="g">
  <a href="https://example.com/real-result">
    <h3>Real Result</h3>
  </a>
</div>
</body></html>`)
	got, err := parseGoogleResults(body, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 result, got %d", len(got))
	}
	if got[0].URL != "https://example.com/real-result" {
		t.Errorf("url: %q", got[0].URL)
	}
}

func TestParseGoogleResults_MultipleResults(t *testing.T) {
	body := []byte(`<html><body>
<div class="g"><a href="https://a.com"><h3>A</h3></a></div>
<div class="g"><a href="https://b.com"><h3>B</h3></a></div>
<div class="g"><a href="https://c.com"><h3>C</h3></a></div>
</body></html>`)
	got, err := parseGoogleResults(body, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 results, got %d", len(got))
	}
}

func TestParseGoogleResults_RespectsMaxResults(t *testing.T) {
	body := []byte(`<html><body>
<div class="g"><a href="https://a.com"><h3>A</h3></a></div>
<div class="g"><a href="https://b.com"><h3>B</h3></a></div>
<div class="g"><a href="https://c.com"><h3>C</h3></a></div>
</body></html>`)
	got, err := parseGoogleResults(body, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 results (maxResults cap), got %d", len(got))
	}
}

func TestParseGoogleResults_EmptyHTML(t *testing.T) {
	body := []byte(`<html><body><div>No results found</div></body></html>`)
	got, err := parseGoogleResults(body, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 results, got %d", len(got))
	}
}

// ---- decodeGoogleHref unit tests ----

func TestDecodeGoogleHref_DirectHTTPS(t *testing.T) {
	got := decodeGoogleHref("https://example.com/page")
	if got != "https://example.com/page" {
		t.Errorf("got %q", got)
	}
}

func TestDecodeGoogleHref_RedirectURL(t *testing.T) {
	got := decodeGoogleHref("/url?q=https://example.com/result&sa=U")
	if got != "https://example.com/result" {
		t.Errorf("got %q", got)
	}
}

func TestDecodeGoogleHref_NonHTTP(t *testing.T) {
	got := decodeGoogleHref("/maps/place/foo")
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestDecodeGoogleHref_RedirectToNonHTTP(t *testing.T) {
	got := decodeGoogleHref("/url?q=/relative/path")
	if got != "" {
		t.Errorf("expected empty for non-http redirect target, got %q", got)
	}
}

// ---- isGoogleDomain unit tests ----

func TestIsGoogleDomain(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		{"https://www.google.com/search?q=foo", true},
		{"https://google.com/", true},
		{"https://google.co.uk/", true},
		{"https://translate.google.com/", true},
		{"https://example.com/", false},
		{"https://notgoogle.com/", false},
		{"https://mygoogle.com/", false},
	}
	for _, tc := range cases {
		t.Run(tc.url, func(t *testing.T) {
			if got := isGoogleDomain(tc.url); got != tc.want {
				t.Errorf("isGoogleDomain(%q) = %v, want %v", tc.url, got, tc.want)
			}
		})
	}
}

// ---- parallel merge integration tests ----

func TestSearchMerge_BothReturnResults_BothCalledAndMerged(t *testing.T) {
	ddgCalled, googleCalled := false, false

	oldDDG := ddgSearchFunc
	oldGoogle := googleSearchFunc
	defer func() {
		ddgSearchFunc = oldDDG
		googleSearchFunc = oldGoogle
	}()

	ddgSearchFunc = func(_ context.Context, _ *ddgsearch.SearchParams) (*ddgsearch.SearchResponse, error) {
		ddgCalled = true
		return &ddgsearch.SearchResponse{
			Results: []ddgsearch.SearchResult{
				{Title: "DDG Only", URL: "https://ddg-only.example.com", Description: "from ddg"},
				{Title: "Shared", URL: "https://shared.example.com", Description: "from ddg shared"},
			},
		}, nil
	}
	googleSearchFunc = func(_ context.Context, _ string, _, _ int) ([]googleResult, error) {
		googleCalled = true
		return []googleResult{
			{Title: "Shared (Google)", URL: "https://shared.example.com", Snippet: "from google shared"},
			{Title: "Google Only", URL: "https://google-only.example.com", Snippet: "from google"},
		}, nil
	}

	tool := WebSearchTool()
	out, err := tool.Execute(context.Background(), `{"query":"test","max_results":10}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !ddgCalled {
		t.Error("DDG should have been called")
	}
	if !googleCalled {
		t.Error("Google should have been called")
	}

	var parsed struct {
		Results []struct {
			Title string `json:"title"`
			URL   string `json:"url"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatal(err)
	}
	// expect 3 results: DDG Only, Shared (from DDG, first seen), Google Only
	if len(parsed.Results) != 3 {
		t.Fatalf("expected 3 results after dedup, got %d: %s", len(parsed.Results), out)
	}
	// DDG results come first in the merged output
	if parsed.Results[0].URL != "https://ddg-only.example.com" {
		t.Errorf("first result should be DDG Only, got %q", parsed.Results[0].URL)
	}
	if parsed.Results[1].URL != "https://shared.example.com" {
		t.Errorf("second result should be Shared (DDG), got %q", parsed.Results[1].URL)
	}
	if parsed.Results[2].URL != "https://google-only.example.com" {
		t.Errorf("third result should be Google Only, got %q", parsed.Results[2].URL)
	}
}

func TestSearchMerge_DDGEmptyGoogleHasResults(t *testing.T) {
	oldDDG := ddgSearchFunc
	oldGoogle := googleSearchFunc
	defer func() {
		ddgSearchFunc = oldDDG
		googleSearchFunc = oldGoogle
	}()

	ddgSearchFunc = func(_ context.Context, _ *ddgsearch.SearchParams) (*ddgsearch.SearchResponse, error) {
		return &ddgsearch.SearchResponse{NoResults: true}, nil
	}
	googleSearchFunc = func(_ context.Context, _ string, _, _ int) ([]googleResult, error) {
		return []googleResult{
			{Title: "Google Result", URL: "https://google-result.example.com", Snippet: "from google"},
		}, nil
	}

	tool := WebSearchTool()
	out, err := tool.Execute(context.Background(), `{"query":"test"}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Results []struct{ URL string } `json:"results"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Results) != 1 || parsed.Results[0].URL != "https://google-result.example.com" {
		t.Errorf("expected Google result, got: %s", out)
	}
}

func TestSearchMerge_GoogleEmptyDDGHasResults(t *testing.T) {
	oldDDG := ddgSearchFunc
	oldGoogle := googleSearchFunc
	defer func() {
		ddgSearchFunc = oldDDG
		googleSearchFunc = oldGoogle
	}()

	ddgSearchFunc = func(_ context.Context, _ *ddgsearch.SearchParams) (*ddgsearch.SearchResponse, error) {
		return &ddgsearch.SearchResponse{
			Results: []ddgsearch.SearchResult{
				{Title: "DDG Result", URL: "https://ddg.example.com", Description: "from ddg"},
			},
		}, nil
	}
	googleSearchFunc = func(_ context.Context, _ string, _, _ int) ([]googleResult, error) {
		return nil, fmt.Errorf("google: blocked")
	}

	tool := WebSearchTool()
	out, err := tool.Execute(context.Background(), `{"query":"test"}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Results []struct{ URL string } `json:"results"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Results) != 1 || parsed.Results[0].URL != "https://ddg.example.com" {
		t.Errorf("expected DDG result, got: %s", out)
	}
}

func TestSearchMerge_BothFail_ReturnsError(t *testing.T) {
	oldDDG := ddgSearchFunc
	oldGoogle := googleSearchFunc
	defer func() {
		ddgSearchFunc = oldDDG
		googleSearchFunc = oldGoogle
	}()

	ddgSearchFunc = func(_ context.Context, _ *ddgsearch.SearchParams) (*ddgsearch.SearchResponse, error) {
		return nil, fmt.Errorf("ddg: unavailable")
	}
	googleSearchFunc = func(_ context.Context, _ string, _, _ int) ([]googleResult, error) {
		return nil, fmt.Errorf("google: blocked")
	}

	tool := WebSearchTool()
	_, err := tool.Execute(context.Background(), `{"query":"test"}`, nil)
	if err == nil {
		t.Error("expected error when both backends fail")
	}
}

func TestSearchMerge_BothEmpty_ReturnsEmptyWithHint(t *testing.T) {
	oldDDG := ddgSearchFunc
	oldGoogle := googleSearchFunc
	defer func() {
		ddgSearchFunc = oldDDG
		googleSearchFunc = oldGoogle
	}()

	ddgSearchFunc = func(_ context.Context, _ *ddgsearch.SearchParams) (*ddgsearch.SearchResponse, error) {
		return &ddgsearch.SearchResponse{NoResults: true}, nil
	}
	googleSearchFunc = func(_ context.Context, _ string, _, _ int) ([]googleResult, error) {
		return nil, nil
	}

	tool := WebSearchTool()
	out, err := tool.Execute(context.Background(), `{"query":"test"}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Results     []struct{ URL string } `json:"results"`
		HasMoreHint string                 `json:"has_more_hint"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Results) != 0 {
		t.Errorf("expected 0 results, got %d", len(parsed.Results))
	}
	if parsed.HasMoreHint == "" {
		t.Error("expected has_more_hint when no results")
	}
}

func TestSearchMerge_CapsAtMaxResults(t *testing.T) {
	oldDDG := ddgSearchFunc
	oldGoogle := googleSearchFunc
	defer func() {
		ddgSearchFunc = oldDDG
		googleSearchFunc = oldGoogle
	}()

	ddgSearchFunc = func(_ context.Context, _ *ddgsearch.SearchParams) (*ddgsearch.SearchResponse, error) {
		results := make([]ddgsearch.SearchResult, 8)
		for i := range results {
			results[i] = ddgsearch.SearchResult{
				Title: fmt.Sprintf("DDG %d", i),
				URL:   fmt.Sprintf("https://ddg-%d.example.com", i),
			}
		}
		return &ddgsearch.SearchResponse{Results: results}, nil
	}
	googleSearchFunc = func(_ context.Context, _ string, _, _ int) ([]googleResult, error) {
		results := make([]googleResult, 8)
		for i := range results {
			results[i] = googleResult{
				Title: fmt.Sprintf("Google %d", i),
				URL:   fmt.Sprintf("https://google-%d.example.com", i),
			}
		}
		return results, nil
	}

	tool := WebSearchTool()
	out, err := tool.Execute(context.Background(), `{"query":"test","max_results":10}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Results []struct{ URL string } `json:"results"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Results) > 10 {
		t.Errorf("expected at most 10 results, got %d", len(parsed.Results))
	}
}
