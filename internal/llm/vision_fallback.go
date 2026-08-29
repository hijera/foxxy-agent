package llm

import (
	"fmt"
	"regexp"
	"strings"
)

// imagesDroppedMarker opens the note that replaces images an endpoint refused, so
// the model is told the attachment exists but could not be shown to it rather than
// being left to reason about a picture it never received.
const imagesDroppedMarker = "[image not shown]"

// visionRejectionRE matches the ways an OpenAI-compatible endpoint says "this model
// cannot read pictures". The wording is not standardised: api.neuraldeep.ru (LiteLLM)
// answers "Model openai/gpt-oss-20b does not accept image input" with HTTP 405 and
// "No endpoints found that support image input" from its fallback chain, while OpenAI
// itself rejects the content part by name. Matching the message is the only portable
// signal — the status code varies (400/404/405) and collides with unrelated failures.
var visionRejectionRE = regexp.MustCompile(`(?i)` + strings.Join([]string{
	`does not accept image input`,
	`does not support image`,
	`support image input`,
	`image input is not supported`,
	`image_url is only supported`,
	`vision is not supported`,
	`no endpoints found that support image`,
}, "|"))

// messagesCarryImages reports whether stripping images would change anything.
func messagesCarryImages(messages []Message) bool {
	for _, m := range messages {
		if len(m.ImageParts) > 0 {
			return true
		}
	}
	return false
}

// isVisionRejection reports whether err is an endpoint refusing image input for a
// request that actually carried images. Both halves matter: the wording alone could
// appear in an unrelated error, and a text-only request has nothing to retry with.
func isVisionRejection(err error, messages []Message) bool {
	if err == nil || !messagesCarryImages(messages) {
		return false
	}
	return visionRejectionRE.MatchString(err.Error())
}

// messagesWithoutImages returns a copy of messages with every image replaced by a
// short note naming what was dropped. The caller's slice and the original messages
// are left untouched, so the transcript the agent persists keeps the real images.
func messagesWithoutImages(messages []Message) []Message {
	out := make([]Message, len(messages))
	copy(out, messages)
	for i, m := range out {
		if len(m.ImageParts) == 0 {
			continue
		}
		names := make([]string, 0, len(m.ImageParts))
		for _, ip := range m.ImageParts {
			name := strings.TrimSpace(ip.Name)
			if name == "" {
				name = "image"
			}
			names = append(names, name)
		}
		note := fmt.Sprintf("%s %s — this model or endpoint rejected image input, so %s not shown to you. Rely on the text results instead.",
			imagesDroppedMarker, strings.Join(names, ", "), pluralWere(len(names)))
		out[i].ImageParts = nil
		if strings.TrimSpace(m.Content) == "" {
			out[i].Content = note
		} else {
			out[i].Content = m.Content + "\n\n" + note
		}
	}
	return out
}

func pluralWere(n int) string {
	if n == 1 {
		return "it was"
	}
	return "they were"
}
