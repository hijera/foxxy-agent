//go:build http && !miniapps

package httpserver

// mergeOpenAPIMiniAppsDoc keeps the common capability endpoint in the lean
// HTTP build while omitting all executable Mini Apps lifecycle routes.
func mergeOpenAPIMiniAppsDoc(doc *map[string]interface{}) {
	if doc == nil {
		return
	}
	paths, _ := (*doc)["paths"].(map[string]interface{})
	if paths == nil {
		paths = make(map[string]interface{})
		(*doc)["paths"] = paths
	}
	paths["/foxxycode/capabilities"] = map[string]interface{}{"get": map[string]interface{}{
		"summary": "Report optional server capabilities", "operationId": "getFoxxyCodeCapabilities",
		"responses": map[string]interface{}{"200": map[string]interface{}{"description": "Capability map", "content": map[string]interface{}{"application/json": map[string]interface{}{"schema": map[string]interface{}{"$ref": "#/components/schemas/Capabilities"}}}}},
	}}
	components, _ := (*doc)["components"].(map[string]interface{})
	if components == nil {
		components = make(map[string]interface{})
		(*doc)["components"] = components
	}
	schemas, _ := components["schemas"].(map[string]interface{})
	if schemas == nil {
		schemas = make(map[string]interface{})
		components["schemas"] = schemas
	}
	schemas["Capabilities"] = map[string]interface{}{"type": "object", "properties": map[string]interface{}{
		"object": map[string]string{"type": "string"}, "capabilities": map[string]interface{}{"type": "object", "additionalProperties": map[string]string{"type": "boolean"}},
	}}
}
