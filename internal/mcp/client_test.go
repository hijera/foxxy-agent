package mcp

import "testing"

func TestParseToolListResultPreservesReadOnlyHint(t *testing.T) {
	tools, err := parseToolListResult([]byte(`{
		"tools": [
			{
				"name": "lookup",
				"description": "Read records",
				"inputSchema": {"type": "object"},
				"annotations": {"readOnlyHint": true}
			},
			{
				"name": "update",
				"description": "Update records",
				"inputSchema": {"type": "object"},
				"annotations": {"readOnlyHint": false}
			},
			{
				"name": "legacy",
				"description": "No safety metadata",
				"inputSchema": {"type": "object"}
			}
		]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 3 {
		t.Fatalf("expected 3 tools, got %d", len(tools))
	}
	if !tools[0].ReadOnly {
		t.Fatal("readOnlyHint=true must be preserved")
	}
	if tools[1].ReadOnly || tools[2].ReadOnly {
		t.Fatal("false or absent readOnlyHint must not be treated as read-only")
	}
}
