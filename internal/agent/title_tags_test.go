package agent

import (
	"context"
	"reflect"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/session"
)

// Upstream files a new chat with labels through POST /describe, which its web
// client calls for a title. The fork's title comes from the backend title pass
// instead, so the same answer has to carry the labels here.
func TestMaybeGenerateTitleFilesTheSessionWithTags(t *testing.T) {
	cfg := titleConfig(t)
	st := &session.State{ID: "s", CWD: t.TempDir(), Mode: session.ModeAgent}
	st.ReplaceMessagesWithoutPersist(firstExchange())
	prov := &summarizeProvider{summary: "Postgres API connection\ntags: Postgres, api, postgres"}
	sender := &titleSender{}
	a := NewAgent(cfg, st, sender, nil)

	a.maybeGenerateTitle(context.Background(), prov)

	if got := st.GetTitleAuto(); got != "Postgres API connection" {
		t.Fatalf("TitleAuto = %q, want the phrase without the tag line", got)
	}
	if got, want := st.GetTags(), session.NormalizeTags([]string{"Postgres", "api"}); !reflect.DeepEqual(got, want) {
		t.Fatalf("tags = %v, want %v", got, want)
	}
	if got := sender.last(); got != "Postgres API connection" {
		t.Fatalf("broadcast title = %q", got)
	}
}

func TestMaybeGenerateTitleKeepsTagsTheSessionAlreadyCarries(t *testing.T) {
	cfg := titleConfig(t)
	st := &session.State{ID: "s", CWD: t.TempDir(), Mode: session.ModeAgent}
	st.ReplaceMessagesWithoutPersist(firstExchange())
	st.ReplaceTags([]string{"mine"})
	prov := &summarizeProvider{summary: "Postgres API connection\ntags: postgres, api"}
	a := NewAgent(cfg, st, &titleSender{}, nil)

	a.maybeGenerateTitle(context.Background(), prov)

	if got := st.GetTitleAuto(); got != "Postgres API connection" {
		t.Fatalf("TitleAuto = %q", got)
	}
	if got := st.GetTags(); !reflect.DeepEqual(got, []string{"mine"}) {
		t.Fatalf("tags = %v: a label the operator or the model filed must survive the title pass", got)
	}
}

func TestSplitTitleTags(t *testing.T) {
	cases := []struct {
		name, raw, title string
		tags             []string
	}{
		{"no tag line", "Rate limiting", "Rate limiting", nil},
		{"tag line first", "Tags: a, b\nRate limiting", "Rate limiting", []string{"a", "b"}},
		{"after reasoning", "<think>tags: x</think>\nRate limiting\ntags: go", "Rate limiting", []string{"go"}},
		{"bulleted and bold", "Rate limiting\n- **tags:** go, http", "Rate limiting", []string{"go", "http"}},
		{"only a tag line", "tags: go", "", []string{"go"}},
		{"non ascii head", "Ⱥ\ntags: go", "Ⱥ", []string{"go"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rest, tags := splitTitleTags(tc.raw)
			if got := cleanTitle(rest); got != tc.title {
				t.Errorf("title = %q, want %q", got, tc.title)
			}
			if want := session.NormalizeTags(tc.tags); !reflect.DeepEqual(tags, want) {
				t.Errorf("tags = %v, want %v", tags, want)
			}
		})
	}
}
