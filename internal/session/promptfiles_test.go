package session_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/EvilFreelancer/coddy-agent/internal/acp"
	"github.com/EvilFreelancer/coddy-agent/internal/session"
)

func TestBuildHydratedComposerPromptAttachment(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "hello world.txt")
	if err := os.WriteFile(p, []byte("hi there"), 0o644); err != nil {
		t.Fatal(err)
	}
	blocks, err := session.BuildHydratedComposerPrompt(root, "see @", []session.PromptFileAttachment{
		{Path: "hello world.txt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 2 || blocks[0].Type != "text" || blocks[0].Text != "see @" {
		t.Fatalf("unexpected first blocks: %+v", blocks)
	}
	if blocks[1].Type != "resource" || blocks[1].Resource == nil || blocks[1].Resource.Text != "hi there" {
		t.Fatalf("unexpected resource: %+v", blocks[1])
	}
}

func TestHydratePromptContentBlocksExpandsAtInText(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "secret.txt")
	if err := os.WriteFile(p, []byte("z9"), 0o644); err != nil {
		t.Fatal(err)
	}
	in := []acp.ContentBlock{{Type: "text", Text: `please read @secret.txt`}}
	out, err := session.HydratePromptContentBlocks(root, in)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("got %d blocks", len(out))
	}
	if out[1].Type != "resource" || out[1].Resource == nil || out[1].Resource.Text != "z9" {
		t.Fatalf("resource %+v", out[1])
	}
}

func TestHydratePromptContentBlocksReadsResourceURI(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "a.txt")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	in := []acp.ContentBlock{
		{Type: "text", Text: "x"},
		{Type: "resource", Resource: &acp.Resource{URI: "a.txt"}},
	}
	out, err := session.HydratePromptContentBlocks(root, in)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || out[1].Resource == nil || out[1].Resource.Text != "x" {
		t.Fatalf("got %+v", out)
	}
}
