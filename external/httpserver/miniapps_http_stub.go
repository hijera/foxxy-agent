//go:build http && !miniapps

package httpserver

func (s *Server) registerMiniAppRoutes() {}

func miniAppsCapability() bool { return false }

func mergeOpenAPIMiniAppsDoc(doc *map[string]interface{}) {
	mergeOpenAPICapabilitiesDoc(doc)
}
