package session_test

// Godog harness for features/prompt_attachment_encoding.feature: writes real
// workspace files in a legacy encoding and drives the two prompt hydration entry
// points (BuildHydratedComposerPrompt for explicit attachments,
// HydratePromptContentBlocks for bare @path tokens) against them.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/cucumber/godog"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/unicode"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/platform"
	"github.com/hijera/foxxycode-agent/internal/session"
)

type attachmentEncodingState struct {
	dir     string
	blocks  []acp.ContentBlock
	lastErr error
}

func (s *attachmentEncodingState) reset() error {
	s.cleanup()
	dir, err := os.MkdirTemp("", "foxxycode-bdd-attach-enc-*")
	if err != nil {
		return err
	}
	// The temp root may be a symlink (/var -> /private/var on macOS); the
	// hydration helpers resolve paths, so compare against the resolved root.
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	s.dir = dir
	s.blocks = nil
	return nil
}

func (s *attachmentEncodingState) cleanup() {
	if s.dir != "" {
		_ = os.RemoveAll(s.dir)
		s.dir = ""
	}
}

// encoderFor maps a spec keyword to an encoder that produces the file bytes.
// This is deliberately the encoding direction, so the test never reuses the
// decoder under test as its own oracle.
func encoderFor(name string) (*encoding.Encoder, error) {
	switch name {
	case "windows-1251":
		return charmap.Windows1251.NewEncoder(), nil
	case "utf-16le":
		return unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewEncoder(), nil
	case "utf-8", "utf-8-bom":
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported encoding in spec: %q", name)
	}
}

func (s *attachmentEncodingState) aWorkspaceFile(name, encName string, doc *godog.DocString) error {
	enc, err := encoderFor(encName)
	if err != nil {
		return err
	}
	data := []byte(doc.Content)
	if enc != nil {
		data, err = enc.Bytes(data)
		if err != nil {
			return err
		}
	}
	if encName == "utf-8-bom" {
		data = append([]byte{0xEF, 0xBB, 0xBF}, data...)
	}
	return os.WriteFile(filepath.Join(s.dir, name), data, 0o644)
}

// aBinaryWorkspaceFile writes something that has no text reading at all: a PNG
// header followed by bytes no charset would decode into sensible text.
func (s *attachmentEncodingState) aBinaryWorkspaceFile(name string) error {
	data := append([]byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A},
		bytes.Repeat([]byte{0x00, 0x01, 0x02, 0xFF}, 64)...)
	return os.WriteFile(filepath.Join(s.dir, name), data, 0o644)
}

// attachToPromptFails is the negative counterpart of attachToPrompt: an explicit
// attachment the operator asked for must report why it could not be inlined,
// rather than silently dropping it.
func (s *attachmentEncodingState) attachToPromptFails(name, input string) error {
	blocks, err := session.BuildHydratedComposerPrompt(s.dir, input, []session.PromptFileAttachment{{Path: name}})
	if err == nil {
		return fmt.Errorf("attaching %q unexpectedly succeeded: %+v", name, blocks)
	}
	s.blocks = nil
	s.lastErr = err
	return nil
}

func (s *attachmentEncodingState) failureSaysNotText() error {
	if s.lastErr == nil {
		return fmt.Errorf("no failure was recorded")
	}
	if !errors.Is(s.lastErr, session.ErrNotDecodableText) {
		return fmt.Errorf("failure is %v, want ErrNotDecodableText", s.lastErr)
	}
	return nil
}

func (s *attachmentEncodingState) noResourceBlocks() error {
	for _, b := range s.blocks {
		if b.Type == "resource" && b.Resource != nil {
			return fmt.Errorf("unexpected resource block for %q", b.Resource.URI)
		}
	}
	return nil
}

func (s *attachmentEncodingState) promptTextIs(want string) error {
	for _, b := range s.blocks {
		if b.Type == "text" || b.Type == acp.ContentTypeText {
			if b.Text != want {
				return fmt.Errorf("prompt text is %q, want %q", b.Text, want)
			}
			return nil
		}
	}
	return fmt.Errorf("no text block in %+v", s.blocks)
}

func (s *attachmentEncodingState) attachToPrompt(name, input string) error {
	blocks, err := session.BuildHydratedComposerPrompt(s.dir, input, []session.PromptFileAttachment{{Path: name}})
	if err != nil {
		return fmt.Errorf("attach %q: %w", name, err)
	}
	s.blocks = blocks
	return nil
}

func (s *attachmentEncodingState) hydratePromptText(input string) error {
	blocks, err := session.HydratePromptContentBlocks(s.dir, []acp.ContentBlock{{Type: acp.ContentTypeText, Text: input}})
	if err != nil {
		return fmt.Errorf("hydrate %q: %w", input, err)
	}
	s.blocks = blocks
	return nil
}

func (s *attachmentEncodingState) resourceFor(uri string) (*acp.Resource, error) {
	for _, b := range s.blocks {
		if b.Type != "resource" || b.Resource == nil {
			continue
		}
		if filepath.ToSlash(b.Resource.URI) == uri {
			return b.Resource, nil
		}
	}
	return nil, fmt.Errorf("no resource block for %q in %+v", uri, s.blocks)
}

func (s *attachmentEncodingState) hasResource(uri string) error {
	_, err := s.resourceFor(uri)
	return err
}

func (s *attachmentEncodingState) resourceMime(uri, mime string) error {
	res, err := s.resourceFor(uri)
	if err != nil {
		return err
	}
	if res.MimeType != mime {
		return fmt.Errorf("resource %q mime is %q, want %q", uri, res.MimeType, mime)
	}
	return nil
}

func (s *attachmentEncodingState) resourceText(uri string, doc *godog.DocString) error {
	res, err := s.resourceFor(uri)
	if err != nil {
		return err
	}
	if res.Text != doc.Content {
		return fmt.Errorf("resource %q text is %q, want %q", uri, res.Text, doc.Content)
	}
	return nil
}

func initializeAttachmentEncodingScenario(sc *godog.ScenarioContext) {
	s := &attachmentEncodingState{}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return ctx, s.reset()
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		s.cleanup()
		return ctx, nil
	})

	sc.Step(`^a workspace file "([^"]+)" encoded in (windows-1251|utf-8|utf-8-bom|utf-16le) with content:$`, s.aWorkspaceFile)
	sc.Step(`^a workspace file "([^"]+)" holding binary content$`, s.aBinaryWorkspaceFile)
	sc.Step(`^I attach "([^"]+)" to the prompt "([^"]*)"$`, s.attachToPrompt)
	sc.Step(`^I attach "([^"]+)" to the prompt "([^"]*)" and it fails$`, s.attachToPromptFails)
	sc.Step(`^I hydrate the prompt text "([^"]*)"$`, s.hydratePromptText)
	sc.Step(`^the prompt has a resource for "([^"]+)"$`, s.hasResource)
	sc.Step(`^the prompt has no resource blocks$`, s.noResourceBlocks)
	sc.Step(`^the prompt text is "([^"]*)"$`, s.promptTextIs)
	sc.Step(`^the failure says the file is not text$`, s.failureSaysNotText)
	sc.Step(`^the resource mime type is "([^"]*)"$`, func(mime string) error {
		return s.resourceMimeForSingle(mime)
	})
	sc.Step(`^the resource text is:$`, func(doc *godog.DocString) error {
		return s.resourceTextForSingle(doc)
	})
}

// resourceMimeForSingle / resourceTextForSingle assert against the only resource
// block in the prompt; every scenario attaches exactly one file.
func (s *attachmentEncodingState) resourceMimeForSingle(mime string) error {
	uri, err := s.soleResourceURI()
	if err != nil {
		return err
	}
	return s.resourceMime(uri, mime)
}

func (s *attachmentEncodingState) resourceTextForSingle(doc *godog.DocString) error {
	uri, err := s.soleResourceURI()
	if err != nil {
		return err
	}
	return s.resourceText(uri, doc)
}

func (s *attachmentEncodingState) soleResourceURI() (string, error) {
	uris := make([]string, 0, 1)
	for _, b := range s.blocks {
		if b.Type == "resource" && b.Resource != nil {
			uris = append(uris, filepath.ToSlash(b.Resource.URI))
		}
	}
	if len(uris) != 1 {
		return "", fmt.Errorf("expected exactly one resource block, got %d (%v)", len(uris), uris)
	}
	return uris[0], nil
}

// systemANSIReadsCyrillic reports whether the machine's ANSI code page reads
// single-byte Cyrillic. Scenarios tagged @windows lean on that code page to
// resolve a file too short for statistical detection, so they only hold on a
// Cyrillic Windows install; everywhere else - other platforms, a Western ANSI
// page - they are filtered out rather than failed.
func systemANSIReadsCyrillic() bool {
	text, _, ok := platform.DecodeANSI([]byte{0xCF, 0xF0, 0xE8})
	return ok && text == "При"
}

func TestPromptAttachmentEncodingFeature(t *testing.T) {
	opts := &godog.Options{
		Format:   "pretty",
		Paths:    []string{"../../features/prompt_attachment_encoding.feature"},
		TestingT: t,
		Strict:   true,
	}
	if !systemANSIReadsCyrillic() {
		opts.Tags = "~@windows"
	}
	suite := godog.TestSuite{
		Name:                "prompt-attachment-encoding",
		ScenarioInitializer: initializeAttachmentEncodingScenario,
		Options:             opts,
	}
	if suite.Run() != 0 {
		t.Fatal("prompt attachment encoding feature suite failed")
	}
}
