package desktop

import "strings"

// DesktopStartURL builds the WebView2 navigation URL for the embedded SPA.
// When locale is "en" or "ru", a ?lang= query is inserted before the hash route.
// A desktop=1 marker is always appended so the SPA can detect the desktop shell
// (see external/ui/src/ui/desktopShell.ts) and enable the guided tour.
func DesktopStartURL(listenAddr, locale string) string {
	loc := strings.TrimSpace(locale)
	if loc == "en" || loc == "ru" {
		return "http://" + listenAddr + "/?lang=" + loc + "&desktop=1#/chat"
	}
	return "http://" + listenAddr + "/?desktop=1#/chat"
}
