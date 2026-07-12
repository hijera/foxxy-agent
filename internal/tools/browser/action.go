//go:build browser

package browser

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

// capture takes a viewport screenshot of the current page.
func (b *Browser) capture() ([]byte, error) {
	var buf []byte
	if err := b.run(chromedp.CaptureScreenshot(&buf)); err != nil {
		return nil, err
	}
	return buf, nil
}

// currentURL returns the page's current location (best-effort).
func (b *Browser) currentURL() string {
	var u string
	_ = b.run(chromedp.Location(&u))
	return u
}

// finishAction captures a screenshot after an action, persists it to the session
// assets directory, hands it to the agent for vision injection, and returns a
// concise text result describing the outcome (URL, action, console output, and the
// saved screenshot path). The base64 image never goes into the text result — it is
// delivered to the model as a user-role vision block via env.AddToolImage.
func finishAction(b *Browser, env *tooling.Env, action string) (string, error) {
	url := b.currentURL()

	shot, shotErr := b.capture()
	var savedPath string
	if shotErr == nil && len(shot) > 0 {
		name := fmt.Sprintf("browser_%d.png", time.Now().UnixNano())
		dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(shot)
		parts := []llm.ImagePart{{DataURL: dataURL, Name: name}}
		if env != nil && strings.TrimSpace(env.SessionDir) != "" {
			if err := session.SavePartsToAssets(parts, env.SessionDir); err == nil {
				savedPath = parts[0].FilePath
			}
		}
		if env != nil && env.AddToolImage != nil {
			env.AddToolImage(dataURL, savedPath, name)
		}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "%s\n", action)
	if url != "" {
		fmt.Fprintf(&sb, "url: %s\n", url)
	}
	if savedPath != "" {
		fmt.Fprintf(&sb, "screenshot: %s\n", savedPath)
	} else if shotErr != nil {
		fmt.Fprintf(&sb, "screenshot: unavailable (%v)\n", shotErr)
	}
	if logs := b.drainConsole(); len(logs) > 0 {
		sb.WriteString("console:\n")
		for _, l := range logs {
			fmt.Fprintf(&sb, "  %s\n", l)
		}
	}
	return strings.TrimRight(sb.String(), "\n"), nil
}
