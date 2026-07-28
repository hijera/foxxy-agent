//go:build http

package httpserver

func mergeOpenAPICapabilitiesDoc(doc *map[string]interface{}) {
	paths := (*doc)["paths"].(map[string]interface{})
	paths["/foxxycode/capabilities"] = map[string]interface{}{
		"get": map[string]interface{}{
			"summary":     "Discover optional FoxxyCode HTTP capabilities",
			"operationId": "getFoxxyCodeCapabilities",
			"responses": map[string]interface{}{
				"200": map[string]interface{}{
					"description": "Build capabilities used for hard feature gating in clients.",
					"content": map[string]interface{}{
						"application/json": map[string]interface{}{
							"schema": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"miniapps": map[string]interface{}{"type": "boolean"},
								},
								"required": []string{"miniapps"},
							},
						},
					},
				},
			},
		},
	}
}
