package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/EvilFreelancer/coddy-agent/internal/llm"
	"github.com/EvilFreelancer/coddy-agent/internal/tooling"
	"github.com/go-shiori/go-readability"
	"github.com/kuhahalong/ddgsearch"
)

// ddgSearchFunc is swapped in tests to avoid live DuckDuckGo calls.
var ddgSearchFunc func(ctx context.Context, params *ddgsearch.SearchParams) (*ddgsearch.SearchResponse, error)

func defaultDDGSearch(ctx context.Context, params *ddgsearch.SearchParams) (*ddgsearch.SearchResponse, error) {
	cfg := &ddgsearch.Config{
		Timeout:    25 * time.Second,
		MaxRetries: 2,
	}
	c, err := ddgsearch.New(cfg)
	if err != nil {
		return nil, err
	}
	if params.Region == "" {
		params.Region = ddgsearch.RegionUS
	}
	if params.SafeSearch == "" {
		params.SafeSearch = ddgsearch.SafeSearchModerate
	}
	return c.Search(ctx, params)
}

// WebSearchTool returns the websearch built-in tool (DuckDuckGo + Google HTML, parallel, merged).
func WebSearchTool() *tooling.Tool {
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name:        "websearch",
			Description: "Search the public web. Queries DuckDuckGo and Google simultaneously, merges results (DDG first, duplicates removed). Returns titles, URLs, and short snippets. Use page for pagination (about 10 results per page). If results are thin, rephrase the query 1-3 times before giving up.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{
						"type":        "string",
						"description": "Search query",
					},
					"page": map[string]interface{}{
						"type":        "integer",
						"description": "Result page number starting at 1 (default 1)",
					},
					"max_results": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum results to return for this page (default 15, cap 25)",
					},
				},
				"required": []string{"query"},
			},
		},
		RequiresPermission: false,
		Execute:            executeSearchWeb,
	}
}

type searchWebArgs struct {
	Query      string `json:"query"`
	Page       int    `json:"page"`
	MaxResults int    `json:"max_results"`
}

func executeSearchWeb(ctx context.Context, argsJSON string, _ *tooling.Env) (string, error) {
	args, err := tooling.ParseArgs[searchWebArgs](argsJSON)
	if err != nil {
		return "", err
	}
	q := strings.TrimSpace(args.Query)
	if q == "" {
		return "", fmt.Errorf("query is required")
	}
	page := args.Page
	if page < 1 {
		page = 1
	}
	maxRes := args.MaxResults
	if maxRes <= 0 {
		maxRes = 15
	}
	if maxRes > 25 {
		maxRes = 25
	}

	// Fire both backends in parallel.
	type ddgOut struct {
		resp *ddgsearch.SearchResponse
		err  error
	}
	type gOut struct {
		rows []googleResult
		err  error
	}
	ddgCh := make(chan ddgOut, 1)
	gCh := make(chan gOut, 1)

	ddgFn := ddgSearchFunc
	if ddgFn == nil {
		ddgFn = defaultDDGSearch
	}
	go func() {
		resp, err := ddgFn(ctx, &ddgsearch.SearchParams{
			Query:      q,
			Page:       page,
			MaxResults: maxRes,
		})
		ddgCh <- ddgOut{resp, err}
	}()

	gFn := googleSearchFunc
	if gFn == nil {
		gFn = defaultGoogleSearch
	}
	go func() {
		rows, err := gFn(ctx, q, page, maxRes)
		gCh <- gOut{rows, err}
	}()

	ddgR := <-ddgCh
	gR := <-gCh

	// Both backends errored — nothing to return.
	if ddgR.err != nil && gR.err != nil {
		return "", fmt.Errorf("search failed (ddg: %w; google: %v)", ddgR.err, gR.err)
	}

	// Merge with URL deduplication; DDG results are listed first.
	type row struct {
		Title       string `json:"title"`
		URL         string `json:"url"`
		Description string `json:"description"`
	}
	seen := make(map[string]bool)
	var rows []row

	if ddgR.err == nil && ddgR.resp != nil {
		for _, r := range ddgR.resp.Results {
			u := strings.TrimSpace(r.URL)
			if u != "" && !seen[u] {
				seen[u] = true
				rows = append(rows, row{
					Title:       strings.TrimSpace(r.Title),
					URL:         u,
					Description: strings.TrimSpace(r.Description),
				})
			}
		}
	}
	if gR.err == nil {
		for _, r := range gR.rows {
			if r.URL != "" && !seen[r.URL] {
				seen[r.URL] = true
				rows = append(rows, row{
					Title:       r.Title,
					URL:         r.URL,
					Description: r.Snippet,
				})
			}
		}
	}

	if len(rows) > maxRes {
		rows = rows[:maxRes]
	}

	out := struct {
		Query       string `json:"query"`
		Page        int    `json:"page"`
		HasMoreHint string `json:"has_more_hint,omitempty"`
		Results     []row  `json:"results"`
	}{
		Query:   q,
		Page:    page,
		Results: rows,
	}
	if len(rows) == 0 {
		out.HasMoreHint = "No results; try rephrasing the query."
	} else if len(rows) >= maxRes {
		out.HasMoreHint = "If you need more links, call websearch again with page incremented or a refined query."
	}

	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

const maxFetchHTMLBytes = 4 << 20

// WebFetchTool returns the webfetch built-in tool (fetch URL as markdown).
func WebFetchTool() *tooling.Tool {
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name:        "webfetch",
			Description: "Download a public http(s) page and return main article text as Markdown (readability extraction). Respects size limits. Blocked for private networks and localhost (SSRF guard).",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"url": map[string]interface{}{
						"type":        "string",
						"description": "Absolute http or https URL",
					},
					"timeout_seconds": map[string]interface{}{
						"type":        "integer",
						"description": "HTTP timeout in seconds (default 30, max 120)",
					},
					"max_chars": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum markdown characters to return (default 120000)",
					},
				},
				"required": []string{"url"},
			},
		},
		RequiresPermission: false,
		Execute:            executeExtractPageContent,
	}
}

type extractPageArgs struct {
	URL            string `json:"url"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	MaxChars       int    `json:"max_chars"`
}

func executeExtractPageContent(ctx context.Context, argsJSON string, _ *tooling.Env) (string, error) {
	args, err := tooling.ParseArgs[extractPageArgs](argsJSON)
	if err != nil {
		return "", err
	}
	u, err := ValidateFetchURL(ctx, args.URL)
	if err != nil {
		return "", err
	}
	timeout := 30
	if args.TimeoutSeconds > 0 {
		timeout = args.TimeoutSeconds
	}
	if timeout > 120 {
		timeout = 120
	}
	maxChars := args.MaxChars
	if maxChars <= 0 {
		maxChars = 120_000
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "coddy-agent/1.0 (+https://github.com/EvilFreelancer/coddy-agent)")

	client := &http.Client{Timeout: time.Duration(timeout) * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("http %d", resp.StatusCode)
	}
	limited := io.LimitReader(resp.Body, maxFetchHTMLBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return "", err
	}
	if len(body) > maxFetchHTMLBytes {
		return "", fmt.Errorf("response body exceeds %d bytes", maxFetchHTMLBytes)
	}

	article, err := readability.FromReader(bytes.NewReader(body), u)
	if err != nil {
		return "", fmt.Errorf("readability: %w", err)
	}
	html := strings.TrimSpace(article.Content)
	if html == "" {
		html = strings.TrimSpace(article.TextContent)
	}
	md, err := HTMLToMarkdown(html)
	if err != nil {
		return "", err
	}
	title := strings.TrimSpace(article.Title)
	var b strings.Builder
	if title != "" {
		b.WriteString("# ")
		b.WriteString(title)
		b.WriteString("\n\n")
	}
	b.WriteString(strings.TrimSpace(md))
	out := b.String()
	if len(out) > maxChars {
		out = out[:maxChars] + "\n\n...truncated..."
	}
	return out, nil
}
