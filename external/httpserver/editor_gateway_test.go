//go:build http

package httpserver

import "testing"

func TestTelegramRelayDoesNotStartOutsideEditorPlugins(t *testing.T) {
	for _, surface := range []string{"", "desktop", "cli", "acp", "browser", "IntelliJ"} {
		t.Run(surface, func(t *testing.T) {
			// No manager, logger or home: an accidental start would panic or
			// touch the user's relay state before a standalone HTTP listener runs.
			st := &StartedHTTP{editor: surface}
			st.startEditorGateway()
			if st.gatewayCancel != nil || st.gatewayDone != nil {
				t.Fatal("non-editor surface started a Telegram relay")
			}
		})
	}
}
