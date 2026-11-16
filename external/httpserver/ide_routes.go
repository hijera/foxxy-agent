//go:build http

package httpserver

func (s *Server) registerIDERoutes() {
	s.mux.HandleFunc("GET /foxxycode/ide/events", s.foxxycodeIdeEvents)
	s.mux.HandleFunc("POST /foxxycode/ide/editor-state", s.foxxycodeIdeEditorState)
	s.mux.HandleFunc("POST /foxxycode/ide/terminal-state", s.foxxycodeIdeTerminalStatePost)
	s.mux.HandleFunc("GET /foxxycode/ide/terminal-state", s.foxxycodeIdeTerminalStateGet)
	s.mux.HandleFunc("POST /foxxycode/ide/copy-buffer", s.foxxycodeIdeCopyBufferPost)
	s.mux.HandleFunc("POST /foxxycode/ide/paste-classify", s.foxxycodeIdePasteClassifyPost)
	s.mux.HandleFunc("POST /foxxycode/ide/tool-result", foxxycodeIdeToolResult)
}
