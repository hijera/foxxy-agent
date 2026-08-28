//go:build http && miniapps

package httpserver

// mergeOpenAPIMiniAppsDoc adds the optional Mini Apps contract only when the
// binary is built with the feature tag. Keep this fragment deliberately close
// to the route registration so the lean HTTP build remains unchanged.
func mergeOpenAPIMiniAppsDoc(doc *map[string]interface{}) {
	if doc == nil {
		return
	}
	paths, _ := (*doc)["paths"].(map[string]interface{})
	if paths == nil {
		paths = make(map[string]interface{})
		(*doc)["paths"] = paths
	}
	jsonObject := func(ref string) map[string]interface{} {
		return map[string]interface{}{"$ref": "#/components/schemas/" + ref}
	}
	request := func(ref string) map[string]interface{} {
		return map[string]interface{}{"required": true, "content": map[string]interface{}{"application/json": map[string]interface{}{"schema": jsonObject(ref)}}}
	}
	response := func(description string, ref string) map[string]interface{} {
		value := map[string]interface{}{"description": description}
		if ref != "" {
			value["content"] = map[string]interface{}{"application/json": map[string]interface{}{"schema": jsonObject(ref)}}
		}
		return value
	}
	accepted := response("Asynchronous Mini App operation", "MiniAppJob")
	accepted["headers"] = map[string]interface{}{"Location": map[string]interface{}{"schema": map[string]string{"type": "string"}}}
	paths["/foxxycode/capabilities"] = map[string]interface{}{"get": map[string]interface{}{
		"summary": "Report optional server capabilities", "operationId": "getFoxxyCodeCapabilities",
		"responses": map[string]interface{}{"200": response("Capability map", "Capabilities")},
	}}
	paths["/foxxycode/sessions/{id}/miniapps/distill"] = map[string]interface{}{"post": map[string]interface{}{
		"summary": "Start Mini App distillation", "operationId": "startMiniAppDistillation",
		"parameters": []interface{}{miniAppsPathParameter("id", "Session id")}, "requestBody": request("MiniAppDistillRequest"),
		"responses": miniAppsResponses(accepted, response("Invalid or busy session", "ErrorEnvelope")),
	}}
	paths["/foxxycode/miniapp-distillations/{job_id}"] = map[string]interface{}{"get": map[string]interface{}{
		"summary": "Get distillation job", "operationId": "getMiniAppDistillation", "parameters": []interface{}{miniAppsPathParameter("job_id", "Distillation job id")},
		"responses": map[string]interface{}{"200": response("Distillation job", "MiniAppJob"), "404": errorResponseRef()},
	}}
	paths["/foxxycode/miniapp-distillations/{job_id}/events"] = miniAppsSSEPath("Stream distillation events", "streamMiniAppDistillationEvents", "job_id")
	paths["/foxxycode/miniapp-distillations/{job_id}/scenario"] = map[string]interface{}{"post": map[string]interface{}{
		"summary": "Confirm a distillation scenario", "operationId": "confirmMiniAppScenario", "parameters": []interface{}{miniAppsPathParameter("job_id", "Distillation job id")}, "requestBody": request("MiniAppScenarioRequest"),
		"responses": map[string]interface{}{"202": accepted, "400": errorResponseRef(), "404": errorResponseRef(), "409": errorResponseRef()},
	}}
	paths["/foxxycode/miniapp-distillations/{job_id}/cancel"] = miniAppsCancelPath("Cancel distillation", "cancelMiniAppDistillation", "job_id", "MiniAppJob")

	paths["/foxxycode/miniapps"] = map[string]interface{}{
		"get":  map[string]interface{}{"summary": "List Mini Apps", "operationId": "listMiniApps", "responses": map[string]interface{}{"200": response("Mini App catalog", "MiniAppCatalog")}},
		"post": map[string]interface{}{"summary": "Create a Mini App draft", "operationId": "createMiniApp", "requestBody": request("MiniApp"), "responses": map[string]interface{}{"201": response("Created draft", "MiniApp"), "400": errorResponseRef(), "422": errorResponseRef()}},
	}
	paths["/foxxycode/miniapps/{id}"] = map[string]interface{}{
		"get":   map[string]interface{}{"summary": "Get a Mini App draft", "operationId": "getMiniApp", "parameters": []interface{}{miniAppsPathParameter("id", "Mini App id")}, "responses": map[string]interface{}{"200": response("Mini App", "MiniApp"), "404": errorResponseRef()}},
		"patch": map[string]interface{}{"summary": "Patch Mini App metadata", "operationId": "patchMiniApp", "parameters": []interface{}{miniAppsPathParameter("id", "Mini App id")}, "requestBody": request("MiniAppMetadataPatch"), "responses": map[string]interface{}{"200": response("Updated draft", "MiniApp"), "409": errorResponseRef(), "422": errorResponseRef()}},
	}
	paths["/foxxycode/miniapps/{id}/versions/{version}"] = map[string]interface{}{"get": map[string]interface{}{"summary": "Get an immutable Mini App release", "operationId": "getMiniAppRelease", "parameters": []interface{}{miniAppsPathParameter("id", "Mini App id"), miniAppsPathParameter("version", "Release version")}, "responses": map[string]interface{}{"200": response("Released Mini App", "MiniApp"), "400": errorResponseRef(), "404": errorResponseRef()}}}
	paths["/foxxycode/miniapps/{id}/draft"] = map[string]interface{}{
		"get": map[string]interface{}{"summary": "Get the current draft", "operationId": "getMiniAppDraft", "parameters": []interface{}{miniAppsPathParameter("id", "Mini App id")}, "responses": map[string]interface{}{"200": response("Mini App draft", "MiniApp"), "404": errorResponseRef()}},
		"put": map[string]interface{}{"summary": "Replace the current draft", "operationId": "replaceMiniAppDraft", "parameters": []interface{}{miniAppsPathParameter("id", "Mini App id")}, "requestBody": request("MiniApp"), "responses": map[string]interface{}{"200": response("Updated draft", "MiniApp"), "409": errorResponseRef(), "422": errorResponseRef()}},
	}
	paths["/foxxycode/miniapps/{id}/assistant"] = map[string]interface{}{"post": map[string]interface{}{
		"summary": "Ask the Mini App editor assistant", "description": "Returns a validated draft proposal without saving it. The caller must review and replace the draft explicitly.",
		"operationId": "assistMiniAppDraft", "parameters": []interface{}{miniAppsPathParameter("id", "Mini App id")}, "requestBody": request("MiniAppAssistantRequest"),
		"responses": map[string]interface{}{"200": response("Assistant reply and proposed draft", "MiniAppAssistantResponse"), "400": errorResponseRef(), "404": errorResponseRef(), "409": errorResponseRef(), "422": errorResponseRef()},
	}}
	paths["/foxxycode/miniapps/{id}/authoring/source"] = map[string]interface{}{"get": map[string]interface{}{"summary": "Get private source evidence", "description": "Returns sanitized authoring evidence. Fixture file contents are omitted; `fixture_files` is a path, SHA-256, and size manifest.", "operationId": "getMiniAppSourceEvidence", "parameters": []interface{}{miniAppsPathParameter("id", "Mini App id")}, "responses": map[string]interface{}{"200": response("Source evidence", "MiniAppSourceEvidence"), "404": errorResponseRef()}}}
	paths["/foxxycode/miniapps/{id}/authoring/patches"] = map[string]interface{}{"post": map[string]interface{}{"summary": "Generate repair proposals", "operationId": "createMiniAppRepairProposals", "parameters": []interface{}{miniAppsPathParameter("id", "Mini App id")}, "requestBody": request("MiniAppPatchRequest"), "responses": map[string]interface{}{"200": response("Repair proposals", "MiniAppPatchList"), "404": errorResponseRef()}}}
	paths["/foxxycode/miniapps/{id}/authoring/patches/{patch_id}/accept"] = map[string]interface{}{"post": map[string]interface{}{"summary": "Accept a repair proposal", "operationId": "acceptMiniAppRepairProposal", "parameters": []interface{}{miniAppsPathParameter("id", "Mini App id"), miniAppsPathParameter("patch_id", "Repair proposal id")}, "responses": map[string]interface{}{"200": response("Updated draft", "MiniApp"), "404": errorResponseRef(), "409": errorResponseRef(), "422": errorResponseRef()}}}
	paths["/foxxycode/miniapps/{id}/validate"] = map[string]interface{}{"post": map[string]interface{}{"summary": "Validate a Mini App draft", "operationId": "validateMiniApp", "parameters": []interface{}{miniAppsPathParameter("id", "Mini App id")}, "responses": map[string]interface{}{"200": response("Validation report", "MiniAppValidationReport"), "404": errorResponseRef()}}}
	paths["/foxxycode/miniapps/{id}/sanitize"] = map[string]interface{}{"post": map[string]interface{}{"summary": "Sanitize a Mini App draft", "operationId": "sanitizeMiniApp", "parameters": []interface{}{miniAppsPathParameter("id", "Mini App id")}, "responses": map[string]interface{}{"200": response("Sanitization report", "MiniAppSanitizationReport"), "404": errorResponseRef()}}}
	paths["/foxxycode/miniapps/{id}/release"] = map[string]interface{}{"post": map[string]interface{}{"summary": "Release a tested Mini App", "operationId": "releaseMiniApp", "parameters": []interface{}{miniAppsPathParameter("id", "Mini App id")}, "requestBody": request("MiniAppReleaseRequest"), "responses": map[string]interface{}{"201": response("Released Mini App", "MiniApp"), "409": errorResponseRef(), "422": errorResponseRef()}}}
	paths["/foxxycode/miniapps/{id}/runs"] = map[string]interface{}{"get": map[string]interface{}{"summary": "List Mini App run history", "operationId": "listMiniAppRuns", "parameters": []interface{}{miniAppsPathParameter("id", "Mini App id")}, "responses": map[string]interface{}{"200": response("Run history", "MiniAppRunList"), "404": errorResponseRef()}}}
	paths["/foxxycode/miniapps/{id}/test-runs"] = map[string]interface{}{"post": map[string]interface{}{"summary": "Run the current draft for verification", "operationId": "startMiniAppTestRun", "parameters": []interface{}{miniAppsPathParameter("id", "Mini App id")}, "requestBody": request("MiniAppRunRequest"), "responses": miniAppsResponses(accepted, response("Invalid draft or input", "ErrorEnvelope"))}}
	paths["/foxxycode/miniapps/{id}/versions/{version}/runs"] = map[string]interface{}{"post": map[string]interface{}{"summary": "Run an immutable Mini App release", "operationId": "startMiniAppReleaseRun", "parameters": []interface{}{miniAppsPathParameter("id", "Mini App id"), miniAppsPathParameter("version", "Release version")}, "requestBody": request("MiniAppRunRequest"), "responses": miniAppsResponses(accepted, response("Invalid release or input", "ErrorEnvelope"))}}
	paths["/foxxycode/miniapp-runs/{run_id}"] = map[string]interface{}{"get": map[string]interface{}{"summary": "Get a Mini App run or run job", "description": "Accepts either the persisted run_id or the async job id returned by a test/release run request. Job-shaped responses are returned until the run is persisted; use run_id for the persisted record.", "operationId": "getMiniAppRun", "parameters": []interface{}{miniAppsPathParameter("run_id", "Run or job id")}, "responses": map[string]interface{}{"200": map[string]interface{}{"description": "Run or asynchronous job status", "content": map[string]interface{}{"application/json": map[string]interface{}{"schema": map[string]interface{}{"oneOf": []interface{}{jsonObject("MiniAppRun"), jsonObject("MiniAppJob")}}}}}, "404": errorResponseRef()}}}
	runEventsPath := miniAppsSSEPath("Stream Mini App run events", "streamMiniAppRunEvents", "run_id")
	if operation, ok := runEventsPath["get"].(map[string]interface{}); ok {
		operation["description"] = "Accepts either a persisted run_id or the async job id returned by a test/release run request."
	}
	paths["/foxxycode/miniapp-runs/{run_id}/events"] = runEventsPath
	paths["/foxxycode/miniapp-runs/{run_id}/confirmation"] = map[string]interface{}{"post": map[string]interface{}{"summary": "Answer a Mini App confirmation", "operationId": "confirmMiniAppRun", "parameters": []interface{}{miniAppsPathParameter("run_id", "Run id")}, "requestBody": request("MiniAppConfirmationRequest"), "responses": map[string]interface{}{"202": accepted, "200": response("Cancelled run", "MiniAppJob"), "400": errorResponseRef(), "404": errorResponseRef(), "409": errorResponseRef(), "422": errorResponseRef()}}}
	paths["/foxxycode/miniapp-runs/{run_id}/cancel"] = miniAppsCancelPath("Cancel Mini App run", "cancelMiniAppRun", "run_id", "MiniAppJob")

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
	for name, schema := range miniAppsSchemas() {
		schemas[name] = schema
	}
}

func miniAppsPathParameter(name, description string) map[string]interface{} {
	return map[string]interface{}{"name": name, "in": "path", "required": true, "schema": map[string]string{"type": "string"}, "description": description}
}

func miniAppsResponses(accepted, invalid map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{"202": accepted, "400": invalid, "404": errorResponseRef(), "409": errorResponseRef(), "422": errorResponseRef(), "500": errorResponseRef()}
}

func miniAppsCancelPath(summary, operationID, param, ref string) map[string]interface{} {
	return map[string]interface{}{"post": map[string]interface{}{"summary": summary, "operationId": operationID, "parameters": []interface{}{miniAppsPathParameter(param, "Identifier")}, "responses": map[string]interface{}{"200": map[string]interface{}{"description": "Cancellation accepted", "content": map[string]interface{}{"application/json": map[string]interface{}{"schema": map[string]interface{}{"$ref": "#/components/schemas/" + ref}}}}, "404": errorResponseRef()}}}
}

func miniAppsSSEPath(summary, operationID, param string) map[string]interface{} {
	return map[string]interface{}{"get": map[string]interface{}{"summary": summary, "operationId": operationID, "parameters": []interface{}{miniAppsPathParameter(param, "Identifier")}, "responses": map[string]interface{}{"200": map[string]interface{}{"description": "Server-Sent Events stream", "content": map[string]interface{}{"text/event-stream": map[string]interface{}{"schema": map[string]string{"type": "string"}}}}, "404": errorResponseRef()}}}
}

func miniAppsSchemas() map[string]interface{} {
	anyObject := map[string]interface{}{"type": "object", "additionalProperties": true}
	capabilityMap := map[string]interface{}{"type": "object", "additionalProperties": map[string]string{"type": "boolean"}}
	return map[string]interface{}{
		"Capabilities":              map[string]interface{}{"type": "object", "properties": map[string]interface{}{"object": map[string]string{"type": "string"}, "capabilities": capabilityMap}},
		"MiniApp":                   anyObject,
		"MiniAppCatalog":            map[string]interface{}{"type": "object", "properties": map[string]interface{}{"object": map[string]string{"type": "string"}, "items": map[string]interface{}{"type": "array", "items": anyObject}, "apps": map[string]interface{}{"type": "array", "items": anyObject}}},
		"MiniAppJob":                anyObject,
		"MiniAppRun":                anyObject,
		"MiniAppRunList":            map[string]interface{}{"type": "object", "properties": map[string]interface{}{"items": map[string]interface{}{"type": "array", "items": anyObject}, "runs": map[string]interface{}{"type": "array", "items": anyObject}}},
		"MiniAppSourceEvidence":     anyObject,
		"MiniAppValidationReport":   anyObject,
		"MiniAppSanitizationReport": anyObject,
		"MiniAppPatchList":          map[string]interface{}{"type": "object", "properties": map[string]interface{}{"items": map[string]interface{}{"type": "array", "items": anyObject}, "patches": map[string]interface{}{"type": "array", "items": anyObject}}},
		"MiniAppDistillRequest":     anyObject, "MiniAppScenarioRequest": anyObject, "MiniAppMetadataPatch": anyObject,
		"MiniAppPatchRequest": anyObject, "MiniAppReleaseRequest": anyObject, "MiniAppRunRequest": anyObject, "MiniAppConfirmationRequest": anyObject,
		"MiniAppAssistantRequest": anyObject, "MiniAppAssistantResponse": map[string]interface{}{"type": "object", "properties": map[string]interface{}{
			"reply": map[string]string{"type": "string"}, "changes": map[string]interface{}{"type": "array", "items": map[string]string{"type": "string"}}, "draft": anyObject,
		}},
	}
}
