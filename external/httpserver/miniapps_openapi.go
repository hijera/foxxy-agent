//go:build http && miniapps

package httpserver

func mergeOpenAPIMiniAppsDoc(doc *map[string]interface{}) {
	mergeOpenAPICapabilitiesDoc(doc)
	paths := (*doc)["paths"].(map[string]interface{})

	paths["/foxxycode/sessions/{id}/miniapps/distill"] = map[string]interface{}{
		"post": miniAppOpenAPIOperation("Distill a saved session into an editable mini-app draft", "distillSessionMiniApp", "MiniAppDistillation", httpAccepted(), sessionIDParameter()),
	}
	paths["/foxxycode/miniapp-distillations/{job_id}"] = map[string]interface{}{
		"get": miniAppOpenAPIOperation("Read asynchronous distillation status", "getMiniAppDistillation", "MiniAppDistillation", httpOK(), miniAppPathParameter("job_id")),
	}
	paths["/foxxycode/miniapps"] = map[string]interface{}{
		"get":  miniAppOpenAPIOperation("List and search mini apps", "listMiniApps", "MiniAppList", httpOK()),
		"post": miniAppOpenAPIOperationWithBody("Import a mini-app JSON document as a draft", "createMiniApp", "MiniApp", "MiniApp", httpCreated()),
	}
	paths["/foxxycode/miniapps/import"] = map[string]interface{}{
		"post": miniAppOpenAPIOperationWithBody("Import a mini-app JSON document as a draft", "importMiniApp", "MiniApp", "MiniApp", httpCreated()),
	}
	paths["/foxxycode/miniapps/{id}"] = map[string]interface{}{
		"get":   miniAppOpenAPIOperation("Read mini-app catalog metadata", "getMiniApp", "MiniAppCatalogEntry", httpOK(), miniAppPathParameter("id")),
		"patch": miniAppOpenAPIOperationWithBody("Update editable mini-app metadata", "updateMiniAppMetadata", "MiniAppMetadataPatch", "MiniApp", httpOK(), miniAppPathParameter("id")),
	}
	paths["/foxxycode/miniapps/{id}/draft"] = map[string]interface{}{
		"get": miniAppOpenAPIOperation("Read the current editable draft", "getMiniAppDraft", "MiniApp", httpOK(), miniAppPathParameter("id")),
		"put": miniAppOpenAPIOperationWithBody("Replace the current editable draft", "putMiniAppDraft", "MiniApp", "MiniApp", httpOK(), miniAppPathParameter("id")),
	}
	paths["/foxxycode/miniapps/{id}/authoring/source"] = map[string]interface{}{
		"get": miniAppOpenAPIOperation("Read private sanitized source-session evidence", "getMiniAppSourceEvidence", "MiniAppSourceEvidence", httpOK(), miniAppPathParameter("id")),
	}
	paths["/foxxycode/miniapps/{id}/model-binding"] = map[string]interface{}{
		"post": miniAppOpenAPIOperationWithBody("Fix a configured logical model for every model-driven draft operation", "setMiniAppModelBinding", "MiniAppModelBindingRequest", "MiniApp", httpOK(), miniAppPathParameter("id")),
	}
	paths["/foxxycode/miniapps/{id}/authoring/chat"] = map[string]interface{}{
		"post": miniAppOpenAPIOperationWithBody("Edit a draft through the bounded LLM authoring tool loop", "editMiniAppWithAssistant", "MiniAppAuthoringRequest", "MiniAppAuthoringResult", httpOK(), miniAppPathParameter("id")),
	}
	paths["/foxxycode/miniapps/{id}/expected-result"] = map[string]interface{}{
		"post": miniAppOpenAPIOperationWithBody("Generate and store an LLM-authored expected-result contract", "generateMiniAppExpectedResult", "MiniAppExpectedResultRequest", "MiniAppExpectedResultGeneration", httpOK(), miniAppPathParameter("id")),
	}
	paths["/foxxycode/miniapps/{id}/validate"] = map[string]interface{}{
		"post": miniAppOpenAPIOperation("Validate the current draft", "validateMiniApp", "MiniAppValidationReport", httpOK(), miniAppPathParameter("id")),
	}
	paths["/foxxycode/miniapps/{id}/sanitize"] = map[string]interface{}{
		"post": miniAppOpenAPIOperation("Run the mandatory release sanitization screen", "sanitizeMiniApp", "MiniAppSanitizationReport", httpOK(), miniAppPathParameter("id")),
	}
	paths["/foxxycode/miniapps/{id}/test-runs"] = map[string]interface{}{
		"post": miniAppOpenAPIOperationWithBody("Test the current draft with operator inputs", "testMiniApp", "MiniAppRunRequest", "MiniAppRun", httpOK(), miniAppPathParameter("id")),
	}
	paths["/foxxycode/miniapps/{id}/release"] = map[string]interface{}{
		"post": miniAppOpenAPIOperationWithBody("Release a tested and sanitized draft as an immutable version", "releaseMiniApp", "MiniAppReleaseRequest", "MiniApp", httpCreated(), miniAppPathParameter("id")),
	}
	paths["/foxxycode/miniapps/{id}/export"] = map[string]interface{}{
		"get": miniAppOpenAPIOperation("Export draft or exact released JSON", "exportMiniApp", "MiniApp", httpOK(), miniAppPathParameter("id")),
	}
	paths["/foxxycode/miniapps/{id}/versions/{version}/runs"] = map[string]interface{}{
		"post": miniAppOpenAPIOperationWithBody("Run an exact immutable mini-app version", "runMiniAppVersion", "MiniAppRunRequest", "MiniAppRun", httpOK(), miniAppPathParameter("id"), miniAppPathParameter("version")),
	}
	paths["/foxxycode/miniapp-runs/{run_id}"] = map[string]interface{}{
		"get": miniAppOpenAPIOperation("Read safe run state and declared outputs", "getMiniAppRun", "MiniAppRun", httpOK(), miniAppPathParameter("run_id")),
	}

	components := (*doc)["components"].(map[string]interface{})
	schemas := components["schemas"].(map[string]interface{})
	schemas["MiniApp"] = map[string]interface{}{
		"type":                 "object",
		"additionalProperties": true,
		"required":             []string{"schema_version", "kind", "id", "state", "metadata", "workflow", "success", "runtime"},
		"properties": map[string]interface{}{
			"schema_version": map[string]interface{}{"type": "string", "enum": []string{"1.0.0"}},
			"kind":           map[string]interface{}{"type": "string", "enum": []string{"foxxycode.miniapp"}},
			"id":             map[string]interface{}{"type": "string"},
			"version":        map[string]interface{}{"type": "string"},
			"state":          map[string]interface{}{"type": "string", "enum": []string{"draft", "released"}},
			"revision":       map[string]interface{}{"type": "string"},
			"metadata":       map[string]interface{}{"type": "object", "additionalProperties": true},
			"requirements":   map[string]interface{}{"type": "object", "additionalProperties": true},
			"permissions":    map[string]interface{}{"type": "object", "additionalProperties": true},
			"inputs":         map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "object", "additionalProperties": true}},
			"workflow":       map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "object", "additionalProperties": true}},
			"success":        map[string]interface{}{"type": "object", "additionalProperties": true},
			"outputs":        map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "object", "additionalProperties": true}},
			"display":        map[string]interface{}{"type": "object", "additionalProperties": true},
			"runtime":        map[string]interface{}{"type": "object", "additionalProperties": true},
		},
	}
	schemas["MiniAppCatalogEntry"] = miniAppFreeObjectSchema()
	schemas["MiniAppMetadataPatch"] = miniAppFreeObjectSchema()
	schemas["MiniAppSourceEvidence"] = miniAppFreeObjectSchema()
	schemas["MiniAppValidationReport"] = miniAppFreeObjectSchema()
	schemas["MiniAppSanitizationReport"] = miniAppFreeObjectSchema()
	schemas["MiniAppDistillation"] = miniAppFreeObjectSchema()
	schemas["MiniAppExpectedResultGeneration"] = miniAppFreeObjectSchema()
	schemas["MiniAppExpectedResultRequest"] = map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"expectations": map[string]interface{}{"type": "string"},
			"draft":        map[string]interface{}{"$ref": "#/components/schemas/MiniApp"},
		},
		"required": []string{"expectations"},
	}
	schemas["MiniAppModelBindingRequest"] = map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"model_ref": map[string]interface{}{"type": "string"},
			"draft":     map[string]interface{}{"$ref": "#/components/schemas/MiniApp"},
		},
		"required": []string{"model_ref"},
	}
	schemas["MiniAppAuthoringRequest"] = map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"message": map[string]interface{}{"type": "string"},
			"history": map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"role":    map[string]interface{}{"type": "string", "enum": []string{"user", "assistant"}},
						"content": map[string]interface{}{"type": "string"},
					},
					"required": []string{"role", "content"},
				},
			},
			"draft": map[string]interface{}{"$ref": "#/components/schemas/MiniApp"},
		},
		"required": []string{"message"},
	}
	schemas["MiniAppAuthoringResult"] = map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"app":           map[string]interface{}{"$ref": "#/components/schemas/MiniApp"},
			"message":       map[string]interface{}{"type": "string"},
			"operations":    map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
			"model_binding": map[string]interface{}{"type": "string"},
		},
		"required": []string{"app", "message", "operations", "model_binding"},
	}
	schemas["MiniAppRun"] = miniAppFreeObjectSchema()
	schemas["MiniAppRunRequest"] = map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"inputs":        miniAppFreeObjectSchema(),
			"confirmations": miniAppFreeObjectSchema(),
		},
		"required": []string{"inputs"},
	}
	schemas["MiniAppReleaseRequest"] = map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{"version": map[string]interface{}{"type": "string", "example": "1.0.0"}},
		"required":   []string{"version"},
	}
	schemas["MiniAppList"] = map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"object": map[string]interface{}{"type": "string"},
			"items": map[string]interface{}{
				"type":  "array",
				"items": map[string]interface{}{"$ref": "#/components/schemas/MiniAppCatalogEntry"},
			},
		},
	}
}

func miniAppOpenAPIOperation(summary, operationID, responseSchema, responseCode string, parameters ...map[string]interface{}) map[string]interface{} {
	operation := map[string]interface{}{
		"summary": summary, "operationId": operationID, "tags": []string{"Mini apps"},
		"responses": map[string]interface{}{
			responseCode: map[string]interface{}{
				"description": "Success",
				"content": map[string]interface{}{
					"application/json": map[string]interface{}{
						"schema": map[string]interface{}{"$ref": "#/components/schemas/" + responseSchema},
					},
				},
			},
			"default": errorResponseRef(),
		},
	}
	if len(parameters) > 0 {
		values := make([]interface{}, len(parameters))
		for index, parameter := range parameters {
			values[index] = parameter
		}
		operation["parameters"] = values
	}
	return operation
}

func miniAppOpenAPIOperationWithBody(summary, operationID, requestSchema, responseSchema, responseCode string, parameters ...map[string]interface{}) map[string]interface{} {
	operation := miniAppOpenAPIOperation(summary, operationID, responseSchema, responseCode, parameters...)
	operation["requestBody"] = map[string]interface{}{
		"required": true,
		"content": map[string]interface{}{
			"application/json": map[string]interface{}{
				"schema": map[string]interface{}{"$ref": "#/components/schemas/" + requestSchema},
			},
		},
	}
	return operation
}

func miniAppPathParameter(name string) map[string]interface{} {
	return map[string]interface{}{
		"name": name, "in": "path", "required": true,
		"schema": map[string]interface{}{"type": "string"},
	}
}

func sessionIDParameter() map[string]interface{} { return miniAppPathParameter("id") }
func miniAppFreeObjectSchema() map[string]interface{} {
	return map[string]interface{}{"type": "object", "additionalProperties": true}
}
func httpOK() string       { return "200" }
func httpCreated() string  { return "201" }
func httpAccepted() string { return "202" }
