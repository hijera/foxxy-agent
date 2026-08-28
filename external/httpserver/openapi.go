//go:build http

package httpserver

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/hijera/foxxycode-agent/internal/version"
	"gopkg.in/yaml.v3"
)

// openAPISpec builds the OpenAPI 3 document for the FoxxyCode HTTP gateway.
// Keep this in sync with routes registered in New.
func openAPISpec() map[string]interface{} {
	ver := version.Get()
	doc := map[string]interface{}{
		"openapi": "3.0.3",
		"info": map[string]interface{}{
			"title": "FoxxyCode HTTP API",
			"description": "OpenAI-compatible endpoints backed by FoxxyCode sessions and agents. **`GET /v1/models`** returns one list: **agent**, **plan**, **docs**, **ask**, and **debug** first (**`owned_by`**: **`foxxycode`**), then every configured **`models[].model`** row (**`id`** is the YAML selector, **`owned_by`** is the provider prefix). " +
				"Classify POST **model** values: **agent** / **plan** / **docs** / **ask** / **debug** run the ReAct agent; a selector with **provider/rest** form (see config) that appears in **`models`** triggers a single direct LLM completion (no tools). " +
				"**`metadata.model`** may appear only on agent/plan/docs/ask/debug requests to set the session **`SelectedModelID`**; it is **not** allowed on direct completion. " +
				"**`metadata.reasoning`** (optional, agent/plan/docs/ask/debug only) sets the reasoning level; it must be one of the effective model's **`reasoning_levels`** (or null/empty to clear). Levels map to provider controls (**`reasoning_effort`**; **`qwen3*`** models on OpenAI-compatible providers also pin **`chat_template_kwargs.enable_thinking`** on). " +
				"JSON and SSE responses include **`metadata`** with the effective YAML model selector (**`metadata.model`**); streamed runs emit a final **`event: foxxycode_meta`** JSON payload with the same map before **`data: [DONE]`**. " +
				"Optional header **X-FoxxyCode-Session-ID** continues an existing session; omit it to create one according to project docs.",
			"version": ver,
		},
		"servers": []interface{}{
			map[string]interface{}{
				"url":         "/",
				"description": "Server root (same host/port as foxxycode http). **`GET /`**, **`/index.html`**, **`/app.js`**, **`/styles.css`**, and favicon paths (**`/foxxycode-favicon.svg`**, **`/favicon-32.png`**, **`/favicon.ico`**, **`/apple-touch-icon.png`**) set **`Cache-Control: no-cache`**.",
			},
		},
		// Auth is optional: the empty requirement means "no auth" (default when no token is set);
		// bearerAuth applies when httpserver.auth_token / --auth-token / FOXXYCODE_HTTP_TOKEN is set.
		"security": []interface{}{
			map[string]interface{}{},
			map[string]interface{}{"bearerAuth": []interface{}{}},
		},
		"paths": map[string]interface{}{
			"/v1/models": map[string]interface{}{
				"get": map[string]interface{}{
					"summary": "List models (profiles and configured LLM backends)",
					"description": "Returns **agent**, then **plan**, then **docs**, then **ask**, then **debug** (**`owned_by`**: **`foxxycode`**), then each **`models[].model`** from configuration (**`owned_by`**: provider segment of **`id`**). " +
						"Optional **`default_agent_model`** echoes configured **`agent.model`** for clients that default **`metadata.model`** on profile requests. " +
						"Choose any returned **`id`** as the HTTP **`model`** on **`POST /v1/chat/completions`** or **`POST /v1/responses`**.",
					"operationId": "listModels",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Model list",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/ModelList",
									},
								},
							},
						},
					},
				},
			},
			"/v1/chat/completions": map[string]interface{}{
				"post": map[string]interface{}{
					"summary": "Create chat completion",
					"description": "Chat completion in OpenAI-compatible shape. **`model`** must match an **`id`** from **`GET /v1/models`**: **`agent`** / **`plan`** / **`docs`** / **`ask`** / **`debug`** (ReAct) or a configured **`models[].model`** YAML selector (single direct completion). " +
						"Optional **`metadata`** on agent/plan/docs/ask/debug only: **`metadata.model`** sets the backed LLM (**`models[].model`**); omit or omit the key to use session defaults. " +
						"**`metadata`** must not carry **`model`** for direct-completion **`model`** values. " +
						"When **stream** is true the response is **text/event-stream** (OpenAI-shaped chunks plus optional **`event: foxxycode_meta`** before **`[DONE]`**). Otherwise JSON. " +
						"A streamed response that has produced no frame for 15s sends an SSE comment keepalive, so an idle-timeout proxy does not drop a turn whose model is answering slowly. " +
						"This **`stream`** field selects the response shape for the client; **`models[].stream`** in **config.yaml** separately selects the transport FoxxyCode uses to reach the LLM. " +
						"Every **agent**/**plan**/**docs**/**ask**/**debug** turn is published to the session's composer relay whatever **`stream`** is set to, so other clients can watch it live over **GET /foxxycode/sessions/{id}/composer-stream**; with **`stream: false`** this response body is unchanged. A session already running a turn answers **409** for both shapes. " +
						"The last entry in **messages** must have role **user**.",
					"operationId": "createChatCompletion",
					"parameters": []interface{}{
						map[string]interface{}{
							"name": "X-FoxxyCode-Session-ID", "in": "header", "required": false,
							"schema":      map[string]string{"type": "string"},
							"description": "Existing session id. If absent, the server may create a new session.",
						},
					},
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"$ref": "#/components/schemas/ChatCompletionRequest",
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Completion or streamed events. SSE may include **`event: foxxycode_meta`** (final metadata map) before **`data: [DONE]`**.",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/ChatCompletionResponse",
									},
								},
								"text/event-stream": map[string]interface{}{
									"schema": map[string]interface{}{
										"type":        "string",
										"format":      "binary",
										"description": "Server-Sent Events stream (OpenAI-compatible chunk lines, optional foxxycode_meta).",
									},
								},
							},
						},
						"400": errorResponseRef(),
						"404": errorResponseRef(),
						"409": sessionBusyResponseRef(),
						"500": errorResponseRef(),
					},
				},
			},
			"/v1/responses": map[string]interface{}{
				"post": map[string]interface{}{
					"summary": "Create response",
					"description": "Responses-style call with **`model`**, **`input`** text, optional **`stream`** (SSE). **`model`** is any **`id`** from **`GET /v1/models`**. " +
						"**`metadata.model`** applies only when **`model`** is **`agent`**, **`plan`**, **`docs`**, **`ask`**, or **`debug`**. **`attachments`** (workspace-relative **`path`** rows) hydrate text file bodies from session **cwd** on **`agent`** / **`plan`** / **`docs`** / **`ask`** / **`debug`** only; a file stored in another detected encoding (Windows-1251 and other legacy charsets) is converted to UTF-8. Every **agent**/**plan**/**docs**/**ask**/**debug** turn is published to the session's composer relay whatever **`stream`** is set to, so other clients can watch it live over **GET /foxxycode/sessions/{id}/composer-stream**; with **`stream: false`** this response body is unchanged. A session already running a turn answers **409** for both shapes. A turn started with **`stream: false`** is cancelled when its HTTP request is dropped; a streamed one keeps running. A streamed response that has produced no frame for 15s sends an SSE comment keepalive, so an idle-timeout proxy does not drop a turn whose model is answering slowly. This **`stream`** field selects the response shape for the client; **`models[].stream`** in **config.yaml** separately selects the transport FoxxyCode uses to reach the LLM.",
					"operationId": "createResponse",
					"parameters": []interface{}{
						map[string]interface{}{
							"name": "X-FoxxyCode-Session-ID", "in": "header", "required": false,
							"schema":      map[string]string{"type": "string"},
							"description": "Existing session id. If absent, the server creates a session for this turn.",
						},
					},
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"$ref": "#/components/schemas/ResponsesCreateRequest",
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Completed JSON or streamed SSE (when **stream** is true). SSE default lines are OpenAI-style `data: { ... chat.completion.chunk ... }`. Named events: **tool_call**, **tool_call_update**, **plan**, **token_usage** (provider counters accumulated over the turn so far, so `inputTokens` + `outputTokens` == `totalTokens`), **usage_update** (`used` / `size` for the current context window), **mcp_phase** (`{\"phase\":\"connecting\"}` then `{\"phase\":\"ready\"}`, emitted only when the turn has to wait for the session's configured MCP servers to finish connecting — transient status, not a transcript row), **`foxxycode_meta`** (effective **`metadata`** map last; for agent/plan/docs/ask/debug turns it also carries **`stop_reason`** - `end_turn`, `cancelled`, `max_turns`, ... - so remote clients recover the ACP stop reason), then **`[DONE]`**.",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/ResponsesCreateResponse",
									},
								},
								"text/event-stream": map[string]interface{}{
									"schema": map[string]interface{}{
										"type":        "string",
										"format":      "binary",
										"description": "SSE including optional `event:` lines",
									},
								},
							},
						},
						"400": errorResponseRef(),
						"404": errorResponseRef(),
						"409": sessionBusyResponseRef(),
						"500": errorResponseRef(),
					},
				},
			},
			"/v1/responses/{id}": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Get response/session by id (MVP)",
					"description": "Returns metadata when **id** is an active session id in this process.",
					"operationId": "getResponse",
					"parameters": []interface{}{
						map[string]interface{}{
							"name": "id", "in": "path", "required": true,
							"schema":      map[string]string{"type": "string"},
							"description": "Session id (same as stored server-side for the conversation).",
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Response metadata",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/ResponsesGetResponse",
									},
								},
							},
						},
						"404": errorResponseRef(),
					},
				},
			},
			"/foxxycode/sessions": map[string]interface{}{
				"get": map[string]interface{}{
					"summary": "List persisted chat sessions",
					"description": "Rows are ordered by **session.json** **updatedAt** (newest first), then **id** when timestamps tie. " +
						"**updatedAt** advances when session state is persisted (messages, titles, etc.); loading a snapshot into memory for HTTP does not rewrite it. " +
						"Bundles created for **scheduler runs** (cron or manual) carry **schedulerRun** metadata and are **hidden** from this list unless **include_scheduler=true**.",
					"parameters": append(foxxycodePagingParams(), map[string]interface{}{
						"name":        "include_scheduler",
						"in":          "query",
						"schema":      map[string]string{"type": "boolean"},
						"description": "When true, include scheduler-run session directories in the list.",
					}, map[string]interface{}{
						"name":        "include_activity",
						"in":          "query",
						"schema":      map[string]string{"type": "boolean"},
						"description": "When true, each session row includes **turnActive**, **activitySeq**, **readActivitySeq**, and **unreadComplete** for composer UI.",
					}, map[string]interface{}{
						"name":   "cwd",
						"in":     "query",
						"schema": map[string]string{"type": "string"},
						"description": "Absolute directory. Keeps only sessions whose **cwd** is that directory or sits beneath it " +
							"(case-insensitive on Windows), applied before **q** and paging. Used by the IntelliJ / VS Code plugins " +
							"to scope History to the open project.",
					}),
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Paged session identifiers"},
						"503": errorResponseRef(),
					},
				},
			},
			"/foxxycode/describe": map[string]interface{}{
				"post": map[string]interface{}{
					"summary":     "Generate a short text description",
					"description": "Accepts arbitrary text and returns a short phrase describing what it is about. If the input is 3 words or fewer, the response echoes them.",
					"operationId": "foxxycodeDescribe",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type": "object",
									"properties": map[string]interface{}{
										"text": map[string]string{"type": "string"},
									},
									"required": []string{"text"},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Description payload",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"object": map[string]string{"type": "string", "example": "foxxycode.describe"},
											"short":  map[string]string{"type": "string"},
										},
										"required": []string{"object", "short"},
									},
								},
							},
						},
						"400": errorResponseRef(),
						"502": errorResponseRef(),
						"503": errorResponseRef(),
					},
				},
			},
			"/foxxycode/enhance-prompt": map[string]interface{}{
				"post": map[string]interface{}{
					"summary": "Enhance a draft prompt",
					"description": "Rewrites a user's draft prompt into a clearer, more specific, and more effective prompt. The draft is treated only as source text to improve, never as a request to answer. " +
						"The rewrite runs on the model the session in **X-FoxxyCode-Session-ID** currently has selected, so it matches the model the chat uses; without a usable session it falls back to **`agent.model`**, then to the first configured **`models[]`** row. " +
						"Returns **503** when no model is configured.",
					"operationId": "foxxycodeEnhancePrompt",
					"parameters": []interface{}{
						map[string]interface{}{
							"name": "X-FoxxyCode-Session-ID", "in": "header", "required": false,
							"schema":      map[string]string{"type": "string"},
							"description": "Existing session id, used to pick the rewrite model. Unknown or invalid ids fall back to the configured default; no session is created.",
						},
					},
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type": "object",
									"properties": map[string]interface{}{
										"text": map[string]string{"type": "string"},
									},
									"required": []string{"text"},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Enhanced prompt payload",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"object": map[string]string{"type": "string", "example": "foxxycode.enhance_prompt"},
											"text":   map[string]string{"type": "string"},
										},
										"required": []string{"object", "text"},
									},
								},
							},
						},
						"400": errorResponseRef(),
						"502": errorResponseRef(),
						"503": errorResponseRef(),
					},
				},
			},
			"/foxxycode/slash-commands": map[string]interface{}{
				"get": map[string]interface{}{
					"summary": "List slash commands from skills (paginated)",
					"description": "Returns skill-derived slash command **`name`** and **`description`** rows sorted by name. " +
						"**`page`** (1-based) and **`page_size`** (1 to 200) are required. Optional **`prefix`** filters by case-insensitive name prefix. " +
						"When **X-FoxxyCode-Session-ID** is set (existing session), listing uses that session **cwd** when resolving **`${CWD}`** in configured skill directories; otherwise the server default session cwd applies.",
					"operationId": "listSlashCommands",
					"parameters": []interface{}{
						map[string]interface{}{
							"name": "X-FoxxyCode-Session-ID", "in": "header", "required": false,
							"schema":      map[string]string{"type": "string"},
							"description": "Optional session whose cwd scopes skill path expansion.",
						},
						map[string]interface{}{
							"name": "page", "in": "query", "required": true,
							"schema":      map[string]interface{}{"type": "integer", "minimum": 1},
							"description": "Page index (1-based).",
						},
						map[string]interface{}{
							"name": "page_size", "in": "query", "required": true,
							"schema": map[string]interface{}{
								"type": "integer", "minimum": 1, "maximum": 200,
							},
							"description": "Rows per page.",
						},
						map[string]interface{}{
							"name": "prefix", "in": "query", "required": false,
							"schema":      map[string]string{"type": "string"},
							"description": "Case-insensitive filter on command name.",
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Paged slash command rows",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/FoxxyCodeSlashCommandsPage",
									},
								},
							},
						},
						"400": errorResponseRef(),
						"404": errorResponseRef(),
						"500": errorResponseRef(),
					},
				},
			},
			"/foxxycode/workspace/files": map[string]interface{}{
				"get": map[string]interface{}{
					"summary": "List workspace files under session cwd (paginated)",
					"description": "**`page`** (1-based) and **`page_size`** (1 to 200) are required. **Case-insensitive** **`prefix`** substring filter over **`path_rel`** (non-empty substring required; omit or blank **`prefix`** yields an empty **`items`** page without scanning). " +
						"Optional **`dirs=true`** adds **`kind`** **`dir`** rows with **`path_rel`** ending in **`/`** for navigation-only rows. Responses are sorted **`path_rel`** ascending. Paths never escape session **cwd**.",
					"operationId": "listWorkspaceFiles",
					"parameters": []interface{}{
						map[string]interface{}{
							"name": "X-FoxxyCode-Session-ID", "in": "header", "required": false,
							"schema":      map[string]string{"type": "string"},
							"description": "Session whose **cwd** is the listing root.",
						},
						map[string]interface{}{
							"name": "page", "in": "query", "required": true,
							"schema":      map[string]interface{}{"type": "integer", "minimum": 1},
							"description": "Page index (1-based).",
						},
						map[string]interface{}{
							"name": "page_size", "in": "query", "required": true,
							"schema": map[string]interface{}{
								"type": "integer", "minimum": 1, "maximum": 200,
							},
							"description": "Rows per page.",
						},
						map[string]interface{}{
							"name": "prefix", "in": "query", "required": false,
							"schema":      map[string]string{"type": "string"},
							"description": "Case-insensitive substring filter applied to **`path_rel`**. When empty, **`items`** is empty.",
						},
						map[string]interface{}{
							"name": "dirs", "in": "query", "required": false,
							"schema": map[string]interface{}{
								"type": "string",
								"enum": []interface{}{"", "true", "false", "1", "0", "yes"},
							},
							"description": "Include directory rows (**`dirs=true`** / **`yes`**). File-only attachments still require non-folder paths.",
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Paged workspace file rows relative to cwd",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/FoxxyCodeWorkspaceFilesPage",
									},
								},
							},
						},
						"400": errorResponseRef(),
						"404": errorResponseRef(),
						"500": errorResponseRef(),
					},
				},
			},
			"/foxxycode/workspace/relativize": map[string]interface{}{
				"post": map[string]interface{}{
					"summary":     "Relativize absolute paths to session cwd",
					"description": "Converts absolute filesystem **`paths`** and/or **`file://`** / **`vscode-file://`** **`uris`** into workspace-relative POSIX paths under the session **cwd**. Backs the IDE drag-and-drop flow (a dropped file becomes an **`@`**-mention). Each result carries **`ok`**; paths outside the workspace (or the cwd root itself) return **`ok:false`**. Session **cwd** is selected by **X-FoxxyCode-Session-ID** (default session cwd otherwise).",
					"operationId": "foxxycodeWorkspaceRelativize",
					"parameters": []interface{}{
						map[string]interface{}{
							"name": "X-FoxxyCode-Session-ID", "in": "header", "required": false,
							"schema":      map[string]string{"type": "string"},
							"description": "Session whose **cwd** is the relativization root.",
						},
					},
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type": "object",
									"properties": map[string]interface{}{
										"paths": map[string]interface{}{
											"type":        "array",
											"items":       map[string]string{"type": "string"},
											"description": "Absolute filesystem paths.",
										},
										"uris": map[string]interface{}{
											"type":        "array",
											"items":       map[string]string{"type": "string"},
											"description": "file:// / vscode-file:// URIs.",
										},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Relativized rows (order matches paths then uris)",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"object": map[string]string{"type": "string"},
											"items": map[string]interface{}{
												"type": "array",
												"items": map[string]interface{}{
													"type": "object",
													"properties": map[string]interface{}{
														"path_rel": map[string]string{"type": "string", "description": "POSIX path relative to cwd (empty when ok is false)."},
														"ok":       map[string]string{"type": "boolean", "description": "False when the path is outside the workspace or cannot be resolved."},
													},
												},
											},
										},
									},
								},
							},
						},
						"400": errorResponseRef(),
						"404": errorResponseRef(),
						"500": errorResponseRef(),
					},
				},
			},
			"/foxxycode/onboarding/status": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "First-run onboarding status",
					"description": "Reports whether the SPA should show the provider picker modal (missing config, providers, or agent model).",
					"operationId": "foxxycodeOnboardingStatusGet",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Onboarding status",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{"$ref": "#/components/schemas/FoxxyCodeOnboardingStatus"},
								},
							},
						},
						"500": errorResponseRef(),
					},
				},
			},
			"/foxxycode/project": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Current project folder",
					"description": "Returns the current project directory used as the working directory for new sessions. **`source`** is `project` when a project was opened explicitly, `default` when falling back to the process cwd. **`native_picker`** reports whether **POST** `/foxxycode/project/pick-folder` can open a native OS dialog (desktop app only).",
					"operationId": "foxxycodeProjectGet",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Current project",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{"$ref": "#/components/schemas/FoxxyCodeProject"},
								},
							},
						},
					},
				},
				"put": map[string]interface{}{
					"summary":     "Open a project folder",
					"description": "Sets the current project directory. New sessions created afterwards use it as their working directory; existing sessions keep their own cwd. The path must name an existing directory. Also bumps the recent-projects list.",
					"operationId": "foxxycodeProjectPut",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type":     "object",
									"required": []interface{}{"path"},
									"properties": map[string]interface{}{
										"path": map[string]string{"type": "string", "description": "Absolute path to an existing directory"},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Updated current project",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{"$ref": "#/components/schemas/FoxxyCodeProject"},
								},
							},
						},
						"400": errorResponseRef(),
						"503": errorResponseRef(),
					},
				},
			},
			"/foxxycode/projects/recent": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Recently opened project folders",
					"description": "Most recently opened first, capped at 15 entries. Entries whose directory no longer exists are kept with **`exists: false`** so clients can flag them.",
					"operationId": "foxxycodeProjectsRecentGet",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Recent projects",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{"$ref": "#/components/schemas/FoxxyCodeRecentProjects"},
								},
							},
						},
					},
				},
			},
			"/foxxycode/project/last-session": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Session to reopen for the current project",
					"description": "Returns the session the user last had open in the current project, for editor plugins that reopen their panel on a fresh random port each launch. **`session_id`** is empty when nothing was recorded, or when the recorded session was deleted, is a scheduler run, or no longer lives under **`path`**.",
					"operationId": "foxxycodeProjectLastSessionGet",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Last opened session for the current project",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{"$ref": "#/components/schemas/FoxxyCodeProjectLastSession"},
								},
							},
						},
					},
				},
				"put": map[string]interface{}{
					"summary":     "Record the session to reopen for the current project",
					"description": "Stores **`session_id`** against the current project in `~/.foxxycode/projects.json`. An empty **`session_id`** clears the record, so the next launch starts on a new chat. The recent-projects order and the current project are left unchanged.",
					"operationId": "foxxycodeProjectLastSessionPut",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type":     "object",
									"required": []interface{}{"session_id"},
									"properties": map[string]interface{}{
										"session_id": map[string]string{"type": "string", "description": "Session id, or empty to clear the record"},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Updated record",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{"$ref": "#/components/schemas/FoxxyCodeProjectLastSession"},
								},
							},
						},
						"400": errorResponseRef(),
						"503": errorResponseRef(),
					},
				},
			},
			"/foxxycode/project/pick-folder": map[string]interface{}{
				"post": map[string]interface{}{
					"summary":     "Open the native folder dialog (desktop only)",
					"description": "Opens the OS folder picker owned by the desktop window and returns the chosen path. Does **not** change the current project - confirm with **PUT** `/foxxycode/project`. Responds **501** outside the desktop app and **409** while another dialog is already open.",
					"operationId": "foxxycodeProjectPickFolder",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Dialog result",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{"$ref": "#/components/schemas/FoxxyCodeProjectPick"},
								},
							},
						},
						"409": errorResponseRef(),
						"500": errorResponseRef(),
						"501": errorResponseRef(),
					},
				},
			},
			"/foxxycode/workspace/context": map[string]interface{}{
				"get": map[string]interface{}{
					"summary": "Workspace context for the composer chips (folder, git branch, worktree, svn branch)",
					"description": "Describes the workspace of the session in **`X-FoxxyCode-Session-ID`** (or the server default cwd without the header). " +
						"With **`path`** the given folder is described instead (pre-session preview); a missing folder yields **400**. " +
						"Inside a git repository the payload adds **`repo_root`**, **`branch`**, **`branches`**, and **`worktrees`** (from `git worktree list`); **`is_worktree`** is true when the workspace is a linked (non-main) worktree. " +
						"Subversion is detected independently of git, so a branch folder that also holds a git repository reports both: **`is_svn_repo`** plus an **`svn`** object with **`available`**, **`wc_root`**, **`url`**, **`relative_url`**, **`repository_root`**, **`revision`**, **`branch`** (`trunk`, `branches/<name>`), **`branches`** (when `vcs.svn.branch_lookup` is on), and **`nested`** (the working copy root sits above the folder). " +
						"With **`vcs.svn.enabled: false`** or no svn client installed, **`is_svn_repo`** is false.",
					"operationId": "foxxycodeWorkspaceContextGet",
					"parameters": []interface{}{
						map[string]interface{}{
							"name": "X-FoxxyCode-Session-ID", "in": "header", "required": false,
							"schema":      map[string]string{"type": "string"},
							"description": "Session whose **cwd** is described (ignored when **`path`** is set).",
						},
						map[string]interface{}{
							"name": "path", "in": "query", "required": false,
							"schema":      map[string]string{"type": "string"},
							"description": "Absolute folder to describe instead of the session cwd.",
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Workspace context",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/FoxxyCodeWorkspaceContext",
									},
								},
							},
						},
						"400": errorResponseRef(),
						"404": errorResponseRef(),
						"500": errorResponseRef(),
					},
				},
			},
			"/foxxycode/workspace/folders": map[string]interface{}{
				"get": map[string]interface{}{
					"summary": "List subfolders for the workspace folder picker",
					"description": "Lists direct subfolders of **`path`** (default: session cwd via **`X-FoxxyCode-Session-ID`**, else the server default cwd). " +
						"Hidden folders and **`node_modules`** are skipped; rows are sorted by name. A missing folder yields **400**.",
					"operationId": "foxxycodeWorkspaceFoldersGet",
					"parameters": []interface{}{
						map[string]interface{}{
							"name": "X-FoxxyCode-Session-ID", "in": "header", "required": false,
							"schema":      map[string]string{"type": "string"},
							"description": "Session whose **cwd** is the default listing root.",
						},
						map[string]interface{}{
							"name": "path", "in": "query", "required": false,
							"schema":      map[string]string{"type": "string"},
							"description": "Absolute folder to list.",
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Folder listing",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"object": map[string]interface{}{"type": "string", "example": "foxxycode.workspace_folders"},
											"path":   map[string]interface{}{"type": "string"},
											"parent": map[string]interface{}{"type": "string"},
											"folders": map[string]interface{}{
												"type": "array",
												"items": map[string]interface{}{
													"type": "object",
													"properties": map[string]interface{}{
														"name": map[string]interface{}{"type": "string"},
														"path": map[string]interface{}{"type": "string"},
													},
												},
											},
										},
									},
								},
							},
							"400": errorResponseRef(),
							"404": errorResponseRef(),
							"500": errorResponseRef(),
						},
					},
				},
			},
			"/foxxycode/config/schema": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "JSON Schema for FoxxyCode YAML configuration (UI)",
					"description": "Returns a JSON Schema document describing the JSON shape accepted by **PUT** `/foxxycode/config` and returned by **GET** `/foxxycode/config`. Includes **`providers[].name`** pattern, optional **`x-foxxycode-provider-api-key-env-placeholder`** on **`providers[].api_key`**, and other UI hints. Exposes **api_key**, optional per-provider **proxy**, and other secrets when combined with **GET** - use only on trusted networks.",
					"operationId": "foxxycodeConfigSchemaGet",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "JSON Schema (draft 2020-12)",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{"type": "object"},
								},
							},
						},
						"500": errorResponseRef(),
					},
				},
			},
			"/foxxycode/config": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Get current configuration as JSON",
					"description": "Returns the active process configuration (including **api_key** and optional **proxy** fields on providers).",
					"operationId": "foxxycodeConfigGet",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Configuration JSON (ConfigJSON)",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{"$ref": "#/components/schemas/FoxxyCodeConfigJSON"},
								},
							},
						},
						"500": errorResponseRef(),
					},
				},
				"put": map[string]interface{}{
					"summary":     "Replace configuration from JSON",
					"description": "Validates the body, writes **config.yaml** atomically, and reloads in-process config. Changed **mcp_servers** are reconnected for active sessions, re-running the workspace trust gate so unapproved project declarations stay cold; a session with a turn in flight is reconnected when that turn ends, not mid-turn, while ACP client-provided session servers stay connected. On reload failure after write, restores **config.yaml.bak** to the primary path.",
					"operationId": "foxxycodeConfigPut",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{"$ref": "#/components/schemas/FoxxyCodeConfigJSON"},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "`{\"ok\":true}` on success",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{"$ref": "#/components/schemas/FoxxyCodeConfigValidateResponse"},
								},
							},
						},
						"400": errorResponseRef(),
						"500": errorResponseRef(),
					},
				},
			},
			"/foxxycode/config/validate": map[string]interface{}{
				"post": map[string]interface{}{
					"summary":     "Validate configuration JSON without writing",
					"description": "Runs the same validation as **PUT** `/foxxycode/config` without persisting.",
					"operationId": "foxxycodeConfigValidatePost",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{"$ref": "#/components/schemas/FoxxyCodeConfigJSON"},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "`{\"ok\":true}`",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{"$ref": "#/components/schemas/FoxxyCodeConfigValidateResponse"},
								},
							},
						},
						"400": map[string]interface{}{
							"description": "`{\"ok\":false,\"error\":\"...\"}`",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{"$ref": "#/components/schemas/FoxxyCodeConfigValidateResponse"},
								},
							},
						},
					},
				},
			},
			"/foxxycode/sessions/{id}/activity": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Composer activity for a session",
					"description": "Returns **turnActive** (turn in flight in this process or holding the exclusive turn lock), **activitySeq**, **readActivitySeq**, **unreadComplete**, and **permissionPending** (a persisted permission gate is awaiting the user) for multi-surface UI.",
					"parameters": []interface{}{
						map[string]interface{}{
							"name": "id", "in": "path", "required": true,
							"schema":      map[string]string{"type": "string"},
							"description": "Session id.",
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Activity payload"},
						"404": errorResponseRef(),
						"500": errorResponseRef(),
					},
				},
			},
			"/foxxycode/sessions/{id}/debug": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Debug trace for a session",
					"description": "Returns the persisted debug-trace events (**object** `foxxycode.session_debug`, **sessionId**, **events**) collected while **debug.enabled** is on: one record per turn start, LLM request/response, and tool start/finish boundary. Raw LLM HTTP bodies go to the process log; this endpoint surfaces the lightweight structured timeline. A session with no trace returns **events: null**.",
					"parameters": []interface{}{
						map[string]interface{}{
							"name": "id", "in": "path", "required": true,
							"schema":      map[string]string{"type": "string"},
							"description": "Session id.",
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Debug trace payload"},
						"404": errorResponseRef(),
						"503": errorResponseRef(),
					},
				},
			},
			"/foxxycode/sessions/{id}/stats": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Token and context statistics for a session",
					"description": "Returns **tokenUsageTotal** (provider tokens **cumulative for the session**, not for the running turn), **tokenUsageByTurn** (the most recent turns; whatever the cap drops is folded into **tokenUsageTrimmed** so the total stays whole) and **contextBreakdown** (live estimate of the current model window, overlaid from the in-memory session when it is loaded, so it reflects compaction immediately). A client that renders a running total adds the live **token_usage** SSE of the current turn on top of the **tokenUsageTotal** it read **before** that turn started; re-reading this route mid-turn to reseed that baseline counts the turn twice. **stats** is **null** when the session has no statistics file yet.",
					"parameters": []interface{}{
						map[string]interface{}{
							"name": "id", "in": "path", "required": true,
							"schema":      map[string]string{"type": "string"},
							"description": "Session id.",
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Session stats payload"},
						"404": errorResponseRef(),
						"500": errorResponseRef(),
						"503": errorResponseRef(),
					},
				},
			},
			"/foxxycode/sessions/{id}/assets/{name}": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Serve a session asset file",
					"description": "Returns the raw bytes of a file from the session assets directory (browser-tool screenshots, pasted images, etc.). **name** must be a bare file name; path separators and traversal segments are rejected. Referenced by the browser-tool transcript cards.",
					"parameters": []interface{}{
						map[string]interface{}{
							"name": "id", "in": "path", "required": true,
							"schema":      map[string]string{"type": "string"},
							"description": "Session id.",
						},
						map[string]interface{}{
							"name": "name", "in": "path", "required": true,
							"schema":      map[string]string{"type": "string"},
							"description": "Asset file name (no path separators).",
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Asset bytes"},
						"400": errorResponseRef(),
						"404": errorResponseRef(),
						"503": errorResponseRef(),
					},
				},
			},
			"/foxxycode/sessions/{id}/background-tasks": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Background tasks of a session",
					"description": "Lists commands the agent started with **run_command** **`background: true`**. Each row carries **id**, **label**, **command**, **status** (**queued**, **running**, **succeeded**, **failed**, **timed_out**, **stopped**, **orphaned**), **started_at**, **finished_at**, **exit_code**, **expected_seconds** (the model's own estimate), **timeout_seconds** (the hard limit), **notify_on_finish** (the task wakes the agent when it ends), plus the server-computed **elapsed_seconds**, **overdue**, and **running**. The task pool lives in the running **foxxycode** process; tasks recorded by an earlier process are merged in from the session bundle with status **orphaned**. Poll this endpoint for the status ticker: background tasks outlive the SSE stream of the turn that started them.",
					"parameters": []interface{}{
						map[string]interface{}{
							"name": "id", "in": "path", "required": true,
							"schema":      map[string]string{"type": "string"},
							"description": "Session id.",
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Task list with a **running** count"},
						"404": errorResponseRef(),
					},
				},
				"delete": map[string]interface{}{
					"summary":     "Clear the finished background tasks of a session",
					"description": "Drops every terminal task of the session, in memory and from the session bundle, and answers with **cleared**. Running tasks are left alone. History accumulates on its own and is deleted with the session, so this is the operator's explicit way to throw it away early.",
					"parameters": []interface{}{
						map[string]interface{}{
							"name": "id", "in": "path", "required": true,
							"schema":      map[string]string{"type": "string"},
							"description": "Session id.",
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Number of cleared tasks"},
						"404": errorResponseRef(),
					},
				},
			},
			"/foxxycode/sessions/{id}/background-tasks/{task_id}": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "One background task with its captured output",
					"description": "Returns the task row plus **output**, the combined stdout and stderr captured so far. Works while the task is still running. A task the pool no longer holds is answered from the session bundle log.",
					"parameters": []interface{}{
						map[string]interface{}{
							"name": "id", "in": "path", "required": true,
							"schema":      map[string]string{"type": "string"},
							"description": "Session id.",
						},
						map[string]interface{}{
							"name": "task_id", "in": "path", "required": true,
							"schema":      map[string]string{"type": "string"},
							"description": "Background task id (for example **bg_1**).",
						},
						map[string]interface{}{
							"name": "tail", "in": "query", "required": false,
							"schema":      map[string]string{"type": "integer"},
							"description": "Return only the last N lines of output. Omit for everything retained. A non-integer or negative value is a **400**. A log read back from the session bundle is capped at its last 256 KiB and flags the truncation.",
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Task with output"},
						"400": errorResponseRef(),
						"404": errorResponseRef(),
					},
				},
			},
			"/foxxycode/sessions/{id}/background-tasks/{task_id}/stop": map[string]interface{}{
				"post": map[string]interface{}{
					"summary":     "Stop a running background task",
					"description": "Terminates the task and the whole process group it started, then returns the final row and its output. Stopping a task that already finished changes nothing and still returns **200**. An unknown id is a **404**; a task that exists but could not be terminated is a **500**, never a 404.",
					"parameters": []interface{}{
						map[string]interface{}{
							"name": "id", "in": "path", "required": true,
							"schema":      map[string]string{"type": "string"},
							"description": "Session id.",
						},
						map[string]interface{}{
							"name": "task_id", "in": "path", "required": true,
							"schema":      map[string]string{"type": "string"},
							"description": "Background task id (for example **bg_1**).",
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Stopped task with output"},
						"404": errorResponseRef(),
						"500": errorResponseRef(),
					},
				},
			},
			"/foxxycode/sessions/{id}": map[string]interface{}{
				"patch": map[string]interface{}{
					"summary":     "Patch session composer metadata",
					"description": "Set **title** (pinned title), **selectedModelId** (YAML **`models[].model`** selector for this session), **selectedReasoning** (reasoning level; must be one of the effective model's **`reasoning_levels`**, empty to clear), and/or **markActivityRead** (boolean) to advance the read cursor for **activitySeq**. **markActivityRead** updates only activity counters in **session.json** and does not change **updatedAt** (history order stays stable until new chat content is saved).",
					"parameters": []interface{}{
						map[string]interface{}{
							"name": "id", "in": "path", "required": true,
							"schema":      map[string]string{"type": "string"},
							"description": "Session id.",
						},
					},
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type": "object",
									"properties": map[string]interface{}{
										"title":             map[string]string{"type": "string"},
										"selectedModelId":   map[string]string{"type": "string"},
										"selectedReasoning": map[string]string{"type": "string"},
										"markActivityRead":  map[string]string{"type": "boolean"},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Patched session"},
						"400": errorResponseRef(),
						"404": errorResponseRef(),
					},
				},
				"delete": map[string]interface{}{
					"summary": "Delete a persisted session",
					"description": "Removes the whole session directory (messages, **`tool_calls/`**, **`stats.json`**, assets, background task logs) and the in-memory MCP clients, after stopping anything the session left running. " +
						"A session that forked from another is also retracted from the **branches.json** of its source, and a branch point left with a single thread is dropped, so the branch navigator never points at a bundle that is gone. " +
						"Deleting an id with no bundle on disk still answers **200**.",
					"operationId": "foxxycodeSessionDelete",
					"parameters": []interface{}{
						map[string]interface{}{
							"name": "id", "in": "path", "required": true,
							"schema":      map[string]string{"type": "string"},
							"description": "Session id.",
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Session deleted",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"object": map[string]string{"type": "string"},
											"id":     map[string]string{"type": "string"},
										},
									},
								},
							},
						},
						"400": errorResponseRef(),
						"500": errorResponseRef(),
						"503": errorResponseRef(),
					},
				},
			},
			"/foxxycode/sessions/{id}/branches": map[string]interface{}{
				"post": map[string]interface{}{
					"summary": "Fork the session at one user message",
					"description": "Creates a sibling conversation that receives every message **before** the user message at **userMessageIndex** (0-based over **`user`** rows), so an edited version of that message can be resent without overwriting the original branch. " +
						"Workspace turn diffs recorded **after** the branch point are reversed in the session cwd first, so the files match the state the branch starts from; **fileRollbackNote** reports which turns were reversed or why none were. " +
						"Both branches are recorded in **branches.json** inside the source session bundle and are listed by **GET** on this path.",
					"operationId": "foxxycodeBranchCreate",
					"parameters": []interface{}{
						map[string]interface{}{
							"name": "id", "in": "path", "required": true,
							"schema":      map[string]string{"type": "string"},
							"description": "Source session id.",
						},
					},
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type":     "object",
									"required": []interface{}{"userMessageIndex"},
									"properties": map[string]interface{}{
										"userMessageIndex": map[string]interface{}{
											"type": "integer", "minimum": 0,
											"description": "0-based index of the user message to branch at.",
										},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Branch created",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"object":           map[string]string{"type": "string"},
											"newSessionId":     map[string]string{"type": "string"},
											"branchIndex":      map[string]string{"type": "integer"},
											"totalBranches":    map[string]string{"type": "integer"},
											"fileRollbackNote": map[string]string{"type": "string"},
										},
									},
								},
							},
						},
						"400": errorResponseRef(),
						"404": errorResponseRef(),
						"500": errorResponseRef(),
					},
				},
				"get": map[string]interface{}{
					"summary": "List branch points visible from a session",
					"description": "Reads **branches.json** from the session bundle. Each entry carries **userMessageIndex**, **currentIndex**, **total**, the sibling **sessions** (**sessionId**, **branchIndex**, **preview**, **lastUpdatedAt**), and **own** - **`true`** for a branch point this session introduced, **`false`** for the sibling view inherited from its parent. " +
						"Sessions whose bundle no longer exists are skipped and **currentIndex** is derived from the surviving list, so a stale branch file heals itself on read; a branch point with fewer than two surviving threads is not reported. " +
						"The bundled UI renders each entry as a **`‹ n/m ›`** navigator under that user message.",
					"operationId": "foxxycodeBranchList",
					"parameters": []interface{}{
						map[string]interface{}{
							"name": "id", "in": "path", "required": true,
							"schema":      map[string]string{"type": "string"},
							"description": "Session id.",
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Branch points",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"object":    map[string]string{"type": "string"},
											"sessionId": map[string]string{"type": "string"},
											"branchPoints": map[string]interface{}{
												"type": "array",
												"items": map[string]interface{}{
													"type": "object",
													"properties": map[string]interface{}{
														"userMessageIndex": map[string]string{"type": "integer"},
														"currentIndex":     map[string]string{"type": "integer"},
														"total":            map[string]string{"type": "integer"},
														"own":              map[string]string{"type": "boolean"},
														"sessions": map[string]interface{}{
															"type": "array",
															"items": map[string]interface{}{
																"type": "object",
																"properties": map[string]interface{}{
																	"sessionId":     map[string]string{"type": "string"},
																	"branchIndex":   map[string]string{"type": "integer"},
																	"preview":       map[string]string{"type": "string"},
																	"lastUpdatedAt": map[string]string{"type": "integer"},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
						"400": errorResponseRef(),
						"404": errorResponseRef(),
						"500": errorResponseRef(),
					},
				},
			},
			"/foxxycode/sessions/{id}/workspace": map[string]interface{}{
				"post": map[string]interface{}{
					"summary": "Switch the session workspace folder, git branch, worktree, or svn branch",
					"description": "Body **`{\"path\": dir}`** switches the session cwd to an existing folder (skills, project rules, and slash commands are re-derived; the new cwd persists in **session.json**). " +
						"Body **`{\"branch\": b}`** checks the branch out in place; when the branch is already checked out in another worktree (including the main one) the session cwd jumps there instead. " +
						"Body **`{\"branch\": b, \"worktree\": true}`** ensures a dedicated worktree for the branch (created under **`<home>/worktrees/<repo>/`** on demand) and moves the session cwd into it. " +
						"**`vcs`** selects the version control system: **`git`** (default) or **`svn`**. With **`{\"vcs\": \"svn\", \"branch\": b}`** the working copy is switched in place (`svn switch`); " +
						"Subversion has no worktrees, so **`{\"vcs\": \"svn\", \"branch\": b, \"worktree\": true}`** checks the branch out into its own folder under **`<home>/worktrees/<wc>/`** and moves the session cwd there. An existing checkout of that branch is reused. " +
						"The workspace is chosen **once per session**: as soon as the conversation has messages, switching yields **409** (`workspace is locked once the conversation starts`). " +
						"A missing folder or a branch switch outside the corresponding repository yields **400**; git checkout/worktree and svn switch/checkout failures yield **409**, as does an svn switch with **`vcs.svn.enabled: false`** or no svn client installed. " +
						"The session is created on demand (draft flow). Responds with the fresh workspace context.",
					"operationId": "foxxycodeSessionWorkspacePost",
					"parameters": []interface{}{
						map[string]interface{}{
							"name": "id", "in": "path", "required": true,
							"schema":      map[string]string{"type": "string"},
							"description": "Session id.",
						},
					},
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type": "object",
									"properties": map[string]interface{}{
										"path":     map[string]string{"type": "string"},
										"branch":   map[string]string{"type": "string"},
										"worktree": map[string]string{"type": "boolean"},
										"vcs": map[string]interface{}{
											"type":        "string",
											"enum":        []interface{}{"git", "svn"},
											"description": "Version control system for a branch switch. Default git.",
										},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Workspace context after the switch",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/FoxxyCodeWorkspaceContext",
									},
								},
							},
						},
						"400": errorResponseRef(),
						"404": errorResponseRef(),
						"409": errorResponseRef(),
						"500": errorResponseRef(),
					},
				},
			},
			"/foxxycode/sessions/{id}/messages": map[string]interface{}{
				"get": map[string]interface{}{
					"summary": "Read conversation transcript",
					"description": "Top-level **model** is the effective YAML backend for this session (**`selectedModelId`** when set, else configured **`agent.model`**). **selectedModelId** echoes the stored session override (may be empty). Assistant rows in **messages** may include **`model`** (YAML selector used for that reply). " +
						"**user** and **assistant** rows may include **created_at** (RFC3339 UTC) when the server appended that message to history. " +
						"When long-term memory copilot has run for this session bundle, responses may include **memoryTurns** (persisted observability parallel to Chat Completions transcript; not forwarded to main LLM). " +
						"**uiLog** (optional) lists UI-only rows such as persisted LLM/request errors keyed by **userTurnIndex**; these are not part of **messages** and are not sent to the model. " +
						"Immediately after **POST /foxxycode/sessions/{id}/cancel**, the returned **messages** list can briefly omit or shorten the in-progress **assistant** row compared to what was already streamed; UIs that keep a local shadow should merge when the server snapshot is a strict prefix of on-screen rows.",
					"parameters": []interface{}{
						map[string]interface{}{"name": "id", "in": "path", "required": true, "schema": map[string]string{"type": "string"}},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "OpenAI-shaped messages payload"},
						"404": errorResponseRef(),
						"503": errorResponseRef(),
					},
				},
			},
			"/foxxycode/sessions/{id}/export": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Export the session transcript as a downloadable document",
					"description": "Renders the dialogue surface — **user** and **assistant** turns plus any assistant **reasoning** blocks; tool/system rows are omitted — into the requested **format** and returns it as a `Content-Disposition: attachment` download. GitHub-flavoured markdown in message content is preserved across formats: headings, emphasis, strikethrough, links (with their targets), nested and task lists, blockquotes, thematic breaks, syntax-highlighted code blocks, and **tables**, which render as real tables rather than rows of pipe characters. Images that resolve to a file in the session's own assets directory are embedded in the document (as a `data:` URI in **html**); remote URLs are never fetched at export time and render as a caption plus a link. The **html**, **pdf** and **docx** documents drop the ambient editor state the agent appends to each user turn (the `<foxxycode_ide_context>` active-file / open-tabs block and the `<foxxycode_terminal_context>` summary), and render the `<foxxycode_session_assets>` uploads wrapper as an **Attachments** section rather than as raw XML; **json** keeps everything verbatim for re-import. The disposition carries an ASCII `filename` plus an RFC 8187 `filename*=UTF-8''…` so non-Latin titles survive. The bundled UI only exposes this action once at least one assistant answer exists; the server applies the same guard and returns **404** for a session that has none.",
					"operationId": "exportSession",
					"parameters": []interface{}{
						map[string]interface{}{"name": "id", "in": "path", "required": true, "schema": map[string]string{"type": "string"}},
						map[string]interface{}{
							"name":        "format",
							"in":          "query",
							"required":    true,
							"description": "Output format.",
							"schema":      map[string]interface{}{"type": "string", "enum": []string{"json", "html", "pdf", "docx"}},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "The exported document, sent as an attachment.",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"session_id":  map[string]string{"type": "string"},
											"title":       map[string]string{"type": "string"},
											"exported_at": map[string]string{"type": "string"},
											"messages":    map[string]interface{}{"type": "array", "items": map[string]string{"type": "object"}},
										},
									},
								},
								"text/html": map[string]interface{}{
									"schema": map[string]string{"type": "string"},
								},
								"application/pdf": map[string]interface{}{
									"schema": map[string]string{"type": "string", "format": "binary"},
								},
								"application/vnd.openxmlformats-officedocument.wordprocessingml.document": map[string]interface{}{
									"schema": map[string]string{"type": "string", "format": "binary"},
								},
							},
						},
						"400": errorResponseRef(),
						"404": errorResponseRef(),
						"500": errorResponseRef(),
					},
				},
			},
			"/foxxycode/sessions/{id}/export/file": map[string]interface{}{
				"post": map[string]interface{}{
					"summary":     "Render the session transcript to a file on disk",
					"description": "Same document as `GET .../export`, written under the OS temp directory instead of returned as an attachment, and announced to connected editor plugins as a `reveal_file` event on `/foxxycode/ide/events`. This exists for editor panels, which cannot save a download: IntelliJ's JCEF drops downloads no handler claims, and the VS Code panel hosts the SPA in a cross-origin iframe with no download permission. The path is derived from the session and its title — it is never taken from the caller — and re-exporting the same session and format overwrites the same file, falling back to `<title>_1`, `<title>_2`, … when the previous export is still held open (Windows locks a `.docx` open in Word).",
					"operationId": "exportSessionToFile",
					"parameters": []interface{}{
						map[string]interface{}{"name": "id", "in": "path", "required": true, "schema": map[string]string{"type": "string"}},
						map[string]interface{}{
							"name":        "format",
							"in":          "query",
							"required":    true,
							"description": "Output format.",
							"schema":      map[string]interface{}{"type": "string", "enum": []string{"json", "html", "pdf", "docx"}},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "The document was written; the response carries its absolute path.",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"path": map[string]interface{}{
												"type":        "string",
												"description": "Absolute path of the rendered document.",
											},
											"delivered": map[string]interface{}{
												"type":        "boolean",
												"description": "Whether an editor plugin was connected to receive the reveal request.",
											},
										},
									},
								},
							},
						},
						"400": errorResponseRef(),
						"404": errorResponseRef(),
						"409": errorResponseRef(),
						"500": errorResponseRef(),
					},
				},
			},
			"/foxxycode/sessions/{id}/assets/{name}/thumbnail": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Read a persisted session image thumbnail",
					"description": "Returns the bounded PNG preview created for an uploaded image. The asset name comes from a user message **`files[].preview_url`**; arbitrary original asset bytes are not exposed by this route.",
					"parameters": []interface{}{
						map[string]interface{}{"name": "id", "in": "path", "required": true, "schema": map[string]string{"type": "string"}},
						map[string]interface{}{"name": "name", "in": "path", "required": true, "schema": map[string]string{"type": "string"}},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "PNG thumbnail",
							"content": map[string]interface{}{
								"image/png": map[string]interface{}{"schema": map[string]string{"type": "string", "format": "binary"}},
							},
						},
						"400": errorResponseRef(),
						"404": errorResponseRef(),
						"503": errorResponseRef(),
					},
				},
			},
			"/foxxycode/events": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Subscribe to server-wide session events",
					"description": "Server-Sent Events for activity that is not tied to one session, so a client can be told a turn started in a session it is not driving instead of polling **GET /foxxycode/sessions**. Emits **event: turn_started** and **event: turn_ended** (**`{object, sessionId, phase, at}`**) for every turn in this server process, whichever surface started it. On connect it replays one **turn_started** per turn already running, then **event: ready** to mark the snapshot complete; an idle stream sends **SSE comments** as keepalives. Like the composer stream, this route also accepts the bearer token as **`?access_token=`**.",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "text/event-stream of session turn events"},
						"500": errorResponseRef(),
					},
				},
			},
			"/foxxycode/sessions/{id}/composer-stream": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Subscribe to live composer SSE for an in-flight turn",
					"description": "Server-Sent Events with the same **data:** and **event:** frames as **POST /v1/responses** (**stream: true**) for the active **agent**/**plan**/**docs**/**ask**/**debug** turn. Replays bytes generated so far, then forwards live chunks until the turn ends (relay closes). This also covers the **autonomous turn** a finished **notify_on_finish** background task wakes: that turn registers a relay of its own, so a client watching the session sees it live rather than only after a reload. While a turn is running but no relay exists yet, emits **SSE comments** (`: composer stream pending`) until a composer POST attaches a relay or the wait window expires (**event: error**). When no turn is in flight for the session, answers immediately with **event: error** carrying **error.code** **no_active_stream** instead of waiting, so a client can fall back to the persisted transcript. Optional header **X-FoxxyCode-Session-ID** must match **{id}** when set. Frames replayed to a subscriber carry an **`id:`** sequence; send it back as **Last-Event-ID** (or **`?last_event_id=`**) to resume after it instead of replaying the whole turn. When the frames a client asks to resume from have already been trimmed, the stream leads with **event: desync** so it can reload the transcript instead of rendering a gap. The primary **POST** stream is unchanged and carries no ids.",
					"parameters": []interface{}{
						map[string]interface{}{"name": "id", "in": "path", "required": true, "schema": map[string]string{"type": "string"}},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "text/event-stream composer relay"},
						"400": errorResponseRef(),
						"404": errorResponseRef(),
						"500": errorResponseRef(),
					},
				},
			},
			"/foxxycode/ide/events": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Stream structured file-edit events for native editor clients",
					"description": "Server-Sent Events stream for native editors (e.g. the IntelliJ plugin) to render inline diffs. Emits **`event: edit_proposed`** when a **`write`**/**`edit`**/**`apply_patch`** tool is awaiting permission (gated mode) and **`event: edit_applied`** after a successful write. Each **`data`** payload is a JSON object **`{type, toolCallId, sessionId, path, before, after}`** where **`path`** is absolute and **`before`**/**`after`** hold full file content. Resolve a gated edit via **`POST /foxxycode/sessions/{id}/permission`**. Also emits **`event: open_file`** (only **`path`** and **`sessionId`** set) when the user picks **Show in IDE** on a plan card via **`POST /foxxycode/sessions/{id}/plans/{slug}/open-in-ide`**; that one is user-initiated and points outside the project, so clients must open it without their in-project / native-diff filters.",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "SSE stream (text/event-stream) of edit events",
							"content": map[string]interface{}{
								"text/event-stream": map[string]interface{}{
									"schema": map[string]string{"type": "string"},
								},
							},
						},
						"500": errorResponseRef(),
					},
				},
			},
			"/foxxycode/ide/editor-state": map[string]interface{}{
				"post": map[string]interface{}{
					"summary":     "Report the IDE's open tabs and active file",
					"description": "Native editor clients (VSCode extension, IntelliJ plugin) push the currently open editor tabs and the focused file here whenever the editor selection changes. The latest snapshot is injected into subsequent agent turns as a **`<foxxycode_ide_context>`** block so the model knows which files the user is actively viewing. Paths are absolute; **`openFiles`** may be empty and **`activeFile`** may be omitted.",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type": "object",
									"properties": map[string]interface{}{
										"openFiles": map[string]interface{}{
											"type":        "array",
											"items":       map[string]string{"type": "string"},
											"description": "Absolute paths of the open editor tabs.",
										},
										"activeFile": map[string]interface{}{
											"type":        "string",
											"description": "Absolute path of the focused editor, if any.",
										},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"204": map[string]interface{}{"description": "Snapshot stored"},
						"400": errorResponseRef(),
					},
				},
			},
			"/foxxycode/ide/terminal-state": map[string]interface{}{
				"post": map[string]interface{}{
					"summary":     "Report the IDE's open terminals and their recent output",
					"description": "Native editor clients (VSCode extension, IntelliJ plugin) push every open terminal here — with an id, name, optional shell/cwd/last command, a bounded tail of recent output, and an **`active`** flag — whenever the terminal state changes. The latest snapshot is injected into subsequent agent turns as a compact **`<foxxycode_terminal_context>`** block, and an **`@terminal`** / **`@terminal:<name>`** mention in the user's message expands to a fuller **`<foxxycode_terminal_output>`** block. Gated per IDE by the **`trackTerminals`** setting.",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type": "object",
									"properties": map[string]interface{}{
										"terminals": map[string]interface{}{
											"type":        "array",
											"description": "Every open IDE terminal.",
											"items": map[string]interface{}{
												"type": "object",
												"properties": map[string]interface{}{
													"id":          map[string]interface{}{"type": "string", "description": "Client-stable terminal id."},
													"name":        map[string]interface{}{"type": "string", "description": "Terminal title (required)."},
													"shell":       map[string]interface{}{"type": "string", "description": "Shell path or name."},
													"cwd":         map[string]interface{}{"type": "string", "description": "Terminal working directory."},
													"lastCommand": map[string]interface{}{"type": "string", "description": "Most recently run command."},
													"output":      map[string]interface{}{"type": "string", "description": "Bounded tail of recent output."},
													"active":      map[string]interface{}{"type": "boolean", "description": "Whether this is the focused terminal."},
												},
											},
										},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"204": map[string]interface{}{"description": "Snapshot stored"},
						"400": errorResponseRef(),
					},
				},
				"get": map[string]interface{}{
					"summary":     "List the currently tracked IDE terminals",
					"description": "Returns the tracked terminals (id, name and **`active`** flag only — no output) so the SPA can populate the **`@terminal`** mention menu.",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Tracked terminals",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"terminals": map[string]interface{}{
												"type": "array",
												"items": map[string]interface{}{
													"type": "object",
													"properties": map[string]interface{}{
														"id":     map[string]string{"type": "string"},
														"name":   map[string]string{"type": "string"},
														"active": map[string]string{"type": "boolean"},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			"/foxxycode/sessions/{id}/permission": map[string]interface{}{
				"post": map[string]interface{}{
					"summary":     "Resolve a pending tool permission prompt from a streaming ReAct turn",
					"description": "Completes **`event: permission`** on **`POST /v1/responses`** (**stream: true**). Body **`toolCallId`** must match **`toolCall.toolCallId`** from the SSE payload; **`optionId`** is **`allow`**, **`allow_always`** (remembers this exact command), **`allow_always_program`** (offered for **run_command** only, and only when the command is a single plain invocation; remembers the program, or the program plus its subcommand for multiplexers like **git**), or **`reject`** (or send **`outcome`** **`allow`** / **`cancelled`**). Optional header **X-FoxxyCode-Session-ID** must match **{id}** when set.",
					"parameters": []interface{}{
						map[string]interface{}{
							"name":        "id",
							"in":          "path",
							"required":    true,
							"schema":      map[string]string{"type": "string"},
							"description": "Session id.",
						},
					},
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type": "object",
									"required": []interface{}{
										"toolCallId",
									},
									"properties": map[string]interface{}{
										"toolCallId": map[string]string{"type": "string"},
										"optionId":   map[string]string{"type": "string"},
										"outcome":    map[string]string{"type": "string"},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"204": map[string]interface{}{"description": "Permission choice accepted"},
						"400": errorResponseRef(),
						"404": errorResponseRef(),
					},
				},
			},
			"/foxxycode/sessions/{id}/question": map[string]interface{}{
				"post": map[string]interface{}{
					"summary":     "Answer a pending interactive question from a streaming ReAct turn",
					"description": "Completes **`event: question`** on **`POST /v1/responses`** (**stream: true**). Body **`requestId`** must match the payload from SSE, and **`answers`** is an array of string arrays (one row per question, entries are selected labels or custom text). Optional header **X-FoxxyCode-Session-ID** must match **{id}** when set.",
					"parameters": []interface{}{
						map[string]interface{}{
							"name":        "id",
							"in":          "path",
							"required":    true,
							"schema":      map[string]string{"type": "string"},
							"description": "Session id.",
						},
					},
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type": "object",
									"required": []interface{}{
										"requestId", "answers",
									},
									"properties": map[string]interface{}{
										"requestId": map[string]string{"type": "string"},
										"answers": map[string]interface{}{
											"type": "array",
											"items": map[string]interface{}{
												"type":  "array",
												"items": map[string]string{"type": "string"},
											},
										},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"204": map[string]interface{}{"description": "Answer accepted"},
						"400": errorResponseRef(),
						"404": errorResponseRef(),
					},
				},
			},
			"/foxxycode/skills": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "List skills",
					"description": "Returns all skills discovered from **`skills.dirs`** with their enabled/disabled status. The disabled state is read from the managed skills directory (`~/.foxxycode/skills/.disabled`).",
					"operationId": "listSkills",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Skill list",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/SkillList",
									},
								},
							},
						},
						"500": errorResponseRef(),
					},
				},
			},
			"/foxxycode/skills/{name}/enable": map[string]interface{}{
				"post": map[string]interface{}{
					"summary":     "Enable a skill",
					"description": "Removes **{name}** from the disabled list so the skill is loaded on the next session turn.",
					"operationId": "enableSkill",
					"parameters": []interface{}{
						map[string]interface{}{
							"name": "name", "in": "path", "required": true,
							"schema":      map[string]string{"type": "string"},
							"description": "Canonical skill name (single segment, no slashes).",
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Skill enabled."},
						"400": errorResponseRef(),
					},
				},
			},
			"/foxxycode/providers/{name}/codex-auth": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Get Codex OAuth status",
					"description": "Reports whether the named Codex provider has a server-side ChatGPT OAuth credential. It never returns token values. A valid unsaved provider name is accepted so Settings can show status before config is saved.",
					"operationId": "getProviderCodexAuth",
					"parameters":  []interface{}{codexProviderNameParameter()},
					"responses": map[string]interface{}{
						"200": jsonSchemaResponse("Non-secret Codex OAuth connection status.", "#/components/schemas/CodexAuthStatus"),
						"400": errorResponseRef(),
						"409": errorResponseRef(),
						"500": errorResponseRef(),
					},
				},
				"delete": map[string]interface{}{
					"summary":     "Remove FoxxyCode-managed Codex OAuth credentials",
					"description": "Deletes only the credential stored under `FOXXYCODE_HOME/providers/{name}/codex-auth.json`. A separate Codex CLI login may remain available as a compatibility fallback.",
					"operationId": "deleteProviderCodexAuth",
					"parameters":  []interface{}{codexProviderNameParameter()},
					"responses": map[string]interface{}{
						"200": jsonSchemaResponse("Connection status after removal.", "#/components/schemas/CodexAuthStatus"),
						"400": errorResponseRef(),
						"409": errorResponseRef(),
						"500": errorResponseRef(),
					},
				},
			},
			"/foxxycode/providers/{name}/codex-auth/device": map[string]interface{}{
				"post": map[string]interface{}{
					"summary":     "Start Codex ChatGPT device authorization",
					"description": "Starts the official ChatGPT device flow. Open `verification_url`, enter `user_code`, then poll the returned `login_id`. The server performs the token exchange and stores credentials with restrictive file permissions.",
					"operationId": "startProviderCodexDeviceAuth",
					"parameters":  []interface{}{codexProviderNameParameter()},
					"responses": map[string]interface{}{
						"200": jsonSchemaResponse("Device authorization instructions.", "#/components/schemas/CodexAuthDeviceStart"),
						"400": errorResponseRef(),
						"409": errorResponseRef(),
						"502": errorResponseRef(),
					},
				},
			},
			"/foxxycode/providers/{name}/neuraldeep-auth": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Get NeuralDeep sign-in status",
					"description": "Reports whether the named neuraldeep provider has a server-side hub login, masked, plus the credential source requests actually use (`oauth`, `api_key`, `api_key_command`, `env`, or `none`). Key values are never returned. A valid unsaved provider name is accepted so Settings can show status before config is saved.",
					"operationId": "getProviderNeuralDeepAuth",
					"parameters":  []interface{}{codexProviderNameParameter()},
					"responses": map[string]interface{}{
						"200": jsonSchemaResponse("Non-secret NeuralDeep sign-in status.", "#/components/schemas/NeuralDeepAuthStatus"),
						"400": errorResponseRef(),
						"409": errorResponseRef(),
						"500": errorResponseRef(),
					},
				},
				"delete": map[string]interface{}{
					"summary":     "Sign out of NeuralDeep",
					"description": "Best-effort revokes the key on the hub, then deletes the credential stored under `FOXXYCODE_HOME/providers/{name}/neuraldeep-auth.json`.",
					"operationId": "deleteProviderNeuralDeepAuth",
					"parameters":  []interface{}{codexProviderNameParameter()},
					"responses": map[string]interface{}{
						"200": jsonSchemaResponse("Connection status after sign-out.", "#/components/schemas/NeuralDeepAuthStatus"),
						"400": errorResponseRef(),
						"409": errorResponseRef(),
						"500": errorResponseRef(),
					},
				},
			},
			"/foxxycode/providers/{name}/neuraldeep-auth/device": map[string]interface{}{
				"post": map[string]interface{}{
					"summary":     "Start NeuralDeep device authorization",
					"description": "Starts the hub's RFC 8628 device flow for client `foxxycode`. Open `verification_url` (it carries the pre-filled code), confirm on the hub portal, then poll the returned `login_id`. The server polls the hub and stores the key with restrictive file permissions.",
					"operationId": "startProviderNeuralDeepDeviceAuth",
					"parameters":  []interface{}{codexProviderNameParameter()},
					"responses": map[string]interface{}{
						"200": jsonSchemaResponse("Device authorization instructions.", "#/components/schemas/CodexAuthDeviceStart"),
						"400": errorResponseRef(),
						"409": errorResponseRef(),
						"502": errorResponseRef(),
					},
				},
			},
			"/foxxycode/providers/{name}/neuraldeep-auth/device/{loginID}": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Poll NeuralDeep device authorization",
					"description": "Returns `pending`, `completed`, or `failed`. Key values are never returned.",
					"operationId": "getProviderNeuralDeepDeviceAuth",
					"parameters": []interface{}{
						codexProviderNameParameter(),
						map[string]interface{}{
							"name": "loginID", "in": "path", "required": true,
							"schema": map[string]string{"type": "string"},
						},
					},
					"responses": map[string]interface{}{
						"200": jsonSchemaResponse("Current device authorization state.", "#/components/schemas/CodexAuthDeviceStatus"),
						"400": errorResponseRef(),
						"404": errorResponseRef(),
						"409": errorResponseRef(),
					},
				},
			},
			"/foxxycode/providers/{name}/codex-auth/device/{loginID}": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Poll Codex device authorization",
					"description": "Returns `pending`, `completed`, or `failed`. Token values are never returned.",
					"operationId": "getProviderCodexDeviceAuth",
					"parameters": []interface{}{
						codexProviderNameParameter(),
						map[string]interface{}{
							"name": "loginID", "in": "path", "required": true,
							"schema": map[string]string{"type": "string"},
						},
					},
					"responses": map[string]interface{}{
						"200": jsonSchemaResponse("Current device authorization state.", "#/components/schemas/CodexAuthDeviceStatus"),
						"400": errorResponseRef(),
						"404": errorResponseRef(),
						"409": errorResponseRef(),
					},
				},
			},
			"/foxxycode/skills/{name}/disable": map[string]interface{}{
				"post": map[string]interface{}{
					"summary":     "Disable a skill",
					"description": "Adds **{name}** to the disabled list so the skill is skipped during loading. The skill files are not removed.",
					"operationId": "disableSkill",
					"parameters": []interface{}{
						map[string]interface{}{
							"name": "name", "in": "path", "required": true,
							"schema":      map[string]string{"type": "string"},
							"description": "Canonical skill name (single segment, no slashes).",
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Skill disabled."},
						"400": errorResponseRef(),
					},
				},
			},
			"/foxxycode/mcp": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "List MCP servers",
					"description": "Returns the merged MCP server list from three levels: **`mcp_servers`** in config.yaml and the global **`<home>/mcp.json`** (scope `global`), plus the project-local **`.foxxycode/mcp.json`** (scope `local`); all mcp.json files are Cursor-compatible and later levels override earlier ones by name. Enabled servers are probed for their tool inventory over their transport (stdio spawn, streamable HTTP with legacy-SSE fallback, or SSE; connect, `tools/list`, close); results are cached until the server definition changes. **`?refresh=1`** forces a re-probe.\n\nA project-local entry arrives with the checkout, so it is **not** probed until it is approved for this workspace (see **POST** `/foxxycode/mcp/{name}/trust`): such a row comes back with `status: \"needs_approval\"`, `trusted: false`, no tools, and the `command`/`args`/`env`/`url`/`fingerprint` an approval would cover. Under `mcp.project_trust: deny` the status is `denied`.",
					"operationId": "listMCPServers",
					"parameters": []interface{}{
						map[string]interface{}{
							"name": "refresh", "in": "query", "required": false,
							"schema":      map[string]string{"type": "string"},
							"description": "Set to `1` to bypass the probe cache.",
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "MCP server list",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/MCPServerList",
									},
								},
							},
						},
						"500": errorResponseRef(),
					},
				},
			},
			"/foxxycode/mcp/{name}/enable": map[string]interface{}{
				"post": map[string]interface{}{
					"summary":     "Enable an MCP server",
					"description": "Clears the disabled flag, persisting into the file that defines the server (config.yaml or `.foxxycode/mcp.json`). New sessions connect it; live sessions see its tools on their next turn.",
					"operationId": "enableMCPServer",
					"parameters":  []interface{}{mcpServerNameParam()},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Server enabled."},
						"400": errorResponseRef(),
					},
				},
			},
			"/foxxycode/mcp/{name}/disable": map[string]interface{}{
				"post": map[string]interface{}{
					"summary":     "Disable an MCP server",
					"description": "Sets the disabled flag in the owning file. The server's tools disappear from live sessions on their next turn; new sessions skip connecting it.",
					"operationId": "disableMCPServer",
					"parameters":  []interface{}{mcpServerNameParam()},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Server disabled."},
						"400": errorResponseRef(),
					},
				},
			},
			"/foxxycode/mcp/{name}/trust": map[string]interface{}{
				"post": map[string]interface{}{
					"summary":     "Approve a project MCP server for this workspace",
					"description": "Records the operator's approval of the **current** declaration of a project-local (`.foxxycode/mcp.json`) server for the server's workspace, so sessions may start it. The approval is bound to the workspace and to a digest of the command-bearing declaration (transport, command, args, env, url, headers), and is stored in `<home>/mcp-trust.json` with a receipt naming what was approved (env and header **names** only). Rewriting the entry withdraws it. Refused with 400 for servers defined in config.yaml or `<home>/mcp.json` (they need no approval) and under `mcp.project_trust: deny`.",
					"operationId": "trustMCPServer",
					"parameters":  []interface{}{mcpServerNameParam()},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Server approved; the response carries the approved `fingerprint`.",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"ok":          map[string]interface{}{"type": "boolean"},
											"fingerprint": map[string]interface{}{"type": "string", "description": "Digest the approval is bound to."},
										},
									},
								},
							},
						},
						"400": errorResponseRef(),
						"500": errorResponseRef(),
					},
				},
			},
			"/foxxycode/mcp/{name}/untrust": map[string]interface{}{
				"post": map[string]interface{}{
					"summary":     "Withdraw a project MCP server approval",
					"description": "Removes the workspace approval of a project-local server. Sessions already holding a connected client keep it; new sessions no longer start the server. `removed` reports whether an approval was actually on file.",
					"operationId": "untrustMCPServer",
					"parameters":  []interface{}{mcpServerNameParam()},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Approval withdrawn (or none was on file).",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"ok":      map[string]interface{}{"type": "boolean"},
											"removed": map[string]interface{}{"type": "boolean"},
										},
									},
								},
							},
						},
						"500": errorResponseRef(),
					},
				},
			},
			"/foxxycode/mcp/project-trust": map[string]interface{}{
				"post": map[string]interface{}{
					"summary":     "Set the project MCP trust policy",
					"description": "Persists **`mcp.project_trust`** into config.yaml and reloads it. Body: **`{\"policy\":\"ask\"|\"allow\"|\"deny\"}`**. `ask` (default) keeps project-local `.foxxycode/mcp.json` servers cold until each declaration is approved for its workspace; `allow` starts them automatically; `deny` never loads them. The MCP tab of the bundled UI edits this next to the servers it governs, so it never joins the settings-document save flow.",
					"operationId": "setMCPProjectTrust",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type":     "object",
									"required": []string{"policy"},
									"properties": map[string]interface{}{
										"policy": map[string]interface{}{"type": "string", "enum": []string{"ask", "allow", "deny"}},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Policy stored; the response echoes the effective `project_trust`.",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"ok":            map[string]interface{}{"type": "boolean"},
											"project_trust": map[string]interface{}{"type": "string", "enum": []string{"ask", "allow", "deny"}},
										},
									},
								},
							},
						},
						"400": errorResponseRef(),
					},
				},
			},
			"/foxxycode/mcp/{name}/tools/{tool}/enable": map[string]interface{}{
				"post": map[string]interface{}{
					"summary":     "Enable a single MCP tool",
					"description": "Removes **{tool}** from the server's disabled-tools list in the owning file.",
					"operationId": "enableMCPTool",
					"parameters":  []interface{}{mcpServerNameParam(), mcpToolNameParam()},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Tool enabled."},
						"400": errorResponseRef(),
					},
				},
			},
			"/foxxycode/mcp/{name}/tools/{tool}/disable": map[string]interface{}{
				"post": map[string]interface{}{
					"summary":     "Disable a single MCP tool",
					"description": "Adds **{tool}** to the server's disabled-tools list (`disabled_tools` in config.yaml, `disabledTools` in `.foxxycode/mcp.json`). The tool is hidden from the agent and rejected at dispatch.",
					"operationId": "disableMCPTool",
					"parameters":  []interface{}{mcpServerNameParam(), mcpToolNameParam()},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Tool disabled."},
						"400": errorResponseRef(),
					},
				},
			},
			"/foxxycode/mcp/{name}": map[string]interface{}{
				"put": map[string]interface{}{
					"summary":     "Create or update an mcp.json MCP server",
					"description": "Upserts one named entry in an mcp.json file (Cursor format: `env` and `headers` are objects, per-tool switches use `disabledTools`). **`?scope=local`** (default) writes the project **`.foxxycode/mcp.json`**; **`?scope=global`** writes the user-global **`<home>/mcp.json`**. Either `command` (stdio) or `url` is required; names must not contain `__`. Config.yaml-defined servers are edited via **PUT** `/foxxycode/config` instead.",
					"operationId": "putMCPServer",
					"parameters": []interface{}{
						mcpServerNameParam(),
						map[string]interface{}{
							"name": "scope", "in": "query", "required": false,
							"schema":      map[string]interface{}{"type": "string", "enum": []string{"global", "local"}},
							"description": "Target file: local (default) = ./.foxxycode/mcp.json, global = <home>/mcp.json.",
						},
					},
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{"$ref": "#/components/schemas/MCPJSONServer"},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Server saved."},
						"400": errorResponseRef(),
						"500": errorResponseRef(),
					},
				},
				"delete": map[string]interface{}{
					"summary":     "Delete an mcp.json MCP server",
					"description": "Removes the named entry from the mcp.json file that defines it (project **`.foxxycode/mcp.json`** or global **`<home>/mcp.json`**). Servers defined in config.yaml are refused with 400.",
					"operationId": "deleteMCPServer",
					"parameters":  []interface{}{mcpServerNameParam()},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Server deleted."},
						"400": errorResponseRef(),
					},
				},
			},
			"/foxxycode/skills/sync": map[string]interface{}{
				"post": map[string]interface{}{
					"summary":     "Sync remote skill sources",
					"description": "Fetches every source in **`skills.sources`** (GitHub repos, git URLs, or an http(s) URL to an agents-standard **`marketplace.json`**) and materializes their skills into the managed skills directory. Manual only — never runs automatically. Returns lists of added/updated skill names and per-source failures.",
					"operationId": "syncSkills",
					"parameters": []interface{}{
						map[string]interface{}{
							"name": "source", "in": "query", "required": false,
							"schema":      map[string]string{"type": "string"},
							"description": "Sync only this marketplace source; omit to sync all configured sources.",
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Sync result.",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{"$ref": "#/components/schemas/SkillSyncResult"},
								},
							},
						},
						"500": errorResponseRef(),
					},
				},
			},
			"/foxxycode/skills/sources": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "List remote skill sources",
					"description": "Returns the configured **`skills.sources`** entries (GitHub repos, git URLs, or marketplace.json URLs).",
					"operationId": "listSkillSources",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Configured sources.",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"object": map[string]string{"type": "string", "example": "foxxycode.skills_sources"},
											"items":  map[string]interface{}{"type": "array", "items": map[string]string{"type": "string"}},
										},
									},
								},
							},
						},
					},
				},
				"post": map[string]interface{}{
					"summary":     "Add a remote skill source",
					"description": "Appends a source to **`skills.sources`** in **config.yaml** and reloads config. Set **`sync:true`** to also fetch it immediately. The source is a GitHub repo (`owner/repo[@ref]`), a git URL, or an http(s) URL to an agents-standard **`marketplace.json`**.",
					"operationId": "addSkillSource",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type": "object",
									"properties": map[string]interface{}{
										"source": map[string]string{"type": "string", "description": "owner/repo[@ref], a git URL, or a marketplace.json URL."},
										"sync":   map[string]interface{}{"type": "boolean", "description": "Fetch the source immediately after adding."},
									},
									"required": []interface{}{"source"},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Source added (with optional sync result)."},
						"400": errorResponseRef(),
						"500": errorResponseRef(),
					},
				},
				"delete": map[string]interface{}{
					"summary":     "Remove a remote skill source",
					"description": "Removes a source from **`skills.sources`** in **config.yaml** (matched case-insensitively) and reloads config. Already-installed skills remain until removed. The source is passed as the **`source`** query parameter. Missing **`source`** returns 400.",
					"operationId": "removeSkillSource",
					"parameters": []interface{}{
						map[string]interface{}{
							"name": "source", "in": "query", "required": true,
							"schema":      map[string]string{"type": "string"},
							"description": "The exact configured source string to remove.",
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Source removed (or absent, with removed:false)."},
						"400": errorResponseRef(),
					},
				},
			},
			"/foxxycode/skills/available": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "List installable marketplace plugins",
					"description": "Fetches every configured marketplace manifest (network / git) and returns the plugins they advertise, each flagged with `installed`. Backs the browse/filter install control.",
					"operationId": "listAvailablePlugins",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Available plugins (name, description, version, source, installed)."},
						"500": errorResponseRef(),
					},
				},
			},
			"/foxxycode/skills/install": map[string]interface{}{
				"post": map[string]interface{}{
					"summary":     "Install one plugin from a marketplace",
					"description": "Installs a single named plugin from a marketplace source (rather than syncing every plugin the source advertises).",
					"operationId": "installPlugin",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type": "object",
									"properties": map[string]interface{}{
										"source": map[string]string{"type": "string", "description": "Configured marketplace source the plugin comes from."},
										"plugin": map[string]string{"type": "string", "description": "Plugin name to install."},
									},
									"required": []interface{}{"source", "plugin"},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Install result (added/updated/failed)."},
						"400": errorResponseRef(),
					},
				},
			},
			"/foxxycode/skills/updates": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Check installed remote skills for updates",
					"description": "For every installed remote skill, fetches its marketplace source and compares the installed version against the latest declared upstream. Performs network / git access. Returns one entry per remote skill with **`update_available`** set when a newer version exists.",
					"operationId": "checkSkillUpdates",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Per-skill update status.",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{"$ref": "#/components/schemas/SkillUpdateList"},
								},
							},
						},
						"500": errorResponseRef(),
					},
				},
			},
			"/foxxycode/skills/{name}/update": map[string]interface{}{
				"post": map[string]interface{}{
					"summary":     "Update a skill to its latest version",
					"description": "Re-syncs the marketplace source that provides **{name}**, installing whatever version that source currently declares. Fails with 400 when the skill was not installed from a remote source.",
					"operationId": "updateSkill",
					"parameters": []interface{}{
						map[string]interface{}{
							"name": "name", "in": "path", "required": true,
							"schema":      map[string]string{"type": "string"},
							"description": "Canonical skill name (single segment, no slashes).",
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Update result.",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{"$ref": "#/components/schemas/SkillSyncResult"},
								},
							},
						},
						"400": errorResponseRef(),
					},
				},
			},
			"/foxxycode/skills/{name}": map[string]interface{}{
				"delete": map[string]interface{}{
					"summary":     "Remove a remote skill",
					"description": "Deletes any on-disk skill by name (its directory, and its remote provenance entry when synced). Bundled (read-only) skills cannot be deleted and return 400; so do skills outside the configured skill directories.",
					"operationId": "removeRemoteSkill",
					"parameters": []interface{}{
						map[string]interface{}{
							"name": "name", "in": "path", "required": true,
							"schema":      map[string]string{"type": "string"},
							"description": "Canonical skill name (single segment, no slashes).",
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Remote skill removed."},
						"400": errorResponseRef(),
					},
				},
			},
			"/foxxycode/providers/{name}/models": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "List a provider's available models",
					"description": "Fetches the model list advertised by the named provider's server (openai: **`GET {api_base}/models`**; anthropic: **`GET {api_base}/v1/models`**; neuraldeep: **`GET https://api.neuraldeep.ru/v1/models`**; codex: the fixed official Codex backend with the saved ChatGPT OAuth token). The provider is resolved from the saved config, so its credentials and `proxy` apply server-side without exposing secrets. Returns **`{ok:true, models:[{id,name,vision}]}`** on success, or **`{ok:false, error, models:[]}`** with HTTP 200 when the upstream call fails so the UI can fall back to manual model entry. **`vision`** is omitted unless the catalog advertises image input (**`capabilities.vision`** or **`modalities.input`** containing **`image`**); it is advisory - the UI seeds the `models[].multimodal` default from it and the user can override. Unknown provider name returns 404.",
					"operationId": "listProviderModels",
					"parameters": []interface{}{
						map[string]interface{}{
							"name": "name", "in": "path", "required": true,
							"schema":      map[string]string{"type": "string"},
							"description": "Provider name from `providers[].name`.",
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Model list result (ok:true with models, or ok:false with error)."},
						"404": errorResponseRef(),
						"500": errorResponseRef(),
					},
				},
			},
			"/foxxycode/providers/models-probe": map[string]interface{}{
				"post": map[string]interface{}{
					"summary":     "List models for an unsaved provider (onboarding probe)",
					"description": "Fetches the model list for a provider that is not saved in the config yet: API credentials arrive in the request body instead of being resolved by provider name (openai: **`GET {api_base}/models`**; anthropic: **`GET {api_base}/v1/models`**; empty `api_base` uses the provider type's default). For `type: codex`, `provider_name` is required and the server reads that name's managed OAuth credential; no token enters the request body. Returns **`{ok:true, models:[{id,name,vision}]}`** on success, or **`{ok:false, error, models:[]}`** with HTTP 200 when the upstream call fails so the UI can fall back to manual model entry. **`vision`** carries the same advisory image-input flag as **`GET /foxxycode/providers/{name}/models`**. Malformed body, invalid Codex provider name, or unsupported `type` returns 400.",
					"operationId": "probeProviderModels",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type":     "object",
									"required": []string{"type"},
									"properties": map[string]interface{}{
										"type":          map[string]interface{}{"type": "string", "enum": []string{"openai", "anthropic", "neuraldeep", "codex"}},
										"provider_name": map[string]interface{}{"type": "string", "description": "Provider name whose server-side OAuth credential is used. Required for type codex; ignored otherwise."},
										"api_base":      map[string]interface{}{"type": "string", "description": "Provider base URL (e.g. http://localhost:11434/v1). Empty uses the type default. Ignored for type neuraldeep and codex."},
										"api_key":       map[string]interface{}{"type": "string"},
										"proxy":         map[string]interface{}{"type": "string", "description": "Optional proxy URL."},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Model list result (ok:true with models, or ok:false with error)."},
						"400": errorResponseRef(),
						"500": errorResponseRef(),
					},
				},
			},
			"/foxxycode/sessions/{id}/cancel": map[string]interface{}{
				"post": map[string]interface{}{
					"summary":     "Cancel active generation for a session",
					"description": "Best-effort cancellation of the current ReAct or direct completion turn. Writes a cross-process cancel signal for persisted bundles so another **foxxycode** process holding the turn can observe cooperative cancel. When assistant tokens were already streamed, the server persists that partial **assistant** message for the interrupted turn before the turn ends. Optional header **X-FoxxyCode-Session-ID** must match **{id}** when set.",
					"parameters": []interface{}{
						map[string]interface{}{
							"name":        "id",
							"in":          "path",
							"required":    true,
							"schema":      map[string]string{"type": "string"},
							"description": "Session id.",
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Cancellation applied (idempotent when nothing is running)."},
						"400": errorResponseRef(),
						"404": errorResponseRef(),
					},
				},
			},
			"/foxxycode/sessions/{id}/compact": map[string]interface{}{
				"post": map[string]interface{}{
					"summary":     "Compact (summarize) older session history",
					"description": "Summarizes conversation history into a single summary row inserted into the transcript (coddy compaction engine). As a manual trigger it forces compaction, folding whatever exists even below the keep-recent boundary (**compaction.keep_recent_turns**, default 2 user turns) by reducing the kept tail as needed; nothing_to_compact is returned only when there is no prior conversation. Later LLM prompts replay only the summary plus the kept tail; the persisted transcript keeps every original message. Equivalent to the built-in **/compact** prompt command. Requires the composer turn lock (409 when another agent turn is running).",
					"parameters": []interface{}{
						map[string]interface{}{
							"name":        "id",
							"in":          "path",
							"required":    true,
							"schema":      map[string]string{"type": "string"},
							"description": "Session id.",
						},
					},
					"requestBody": map[string]interface{}{
						"required": false,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type": "object",
									"properties": map[string]interface{}{
										"instructions": map[string]string{
											"type":        "string",
											"description": "Optional extra guidance for the summarizer (what to emphasize).",
										},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Compaction outcome.",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{"$ref": "#/components/schemas/CompactResult"},
								},
							},
						},
						"400": errorResponseRef(),
						"404": errorResponseRef(),
						"409": errorResponseRef(),
					},
				},
			},
			"/foxxycode/sessions/{id}/plans": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "List design plans stored in the session bundle",
					"description": "Design plans live as **plans/<slug>.plan.md** inside the session bundle, written by the **plan_write** tool in plan mode and rendered by the bundled UI as the plan card.",
					"parameters":  []interface{}{designPlanIDParam()},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Plan documents for the session.",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"object": map[string]string{"type": "string"},
											"plans": map[string]interface{}{
												"type":  "array",
												"items": map[string]interface{}{"$ref": "#/components/schemas/DesignPlan"},
											},
										},
									},
								},
							},
						},
						"400": errorResponseRef(),
						"404": errorResponseRef(),
					},
				},
				"post": map[string]interface{}{
					"summary":     "Create a design plan",
					"description": "Creates **plans/<slug>.plan.md** and appends a **plan_document** row to the transcript. **409** when the slug already exists.",
					"parameters":  []interface{}{designPlanIDParam()},
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type": "object",
									"properties": map[string]interface{}{
										"slug":    map[string]string{"type": "string", "description": "Lowercase alphanumeric and hyphens, up to 64 chars."},
										"content": map[string]string{"type": "string", "description": "Full file content including the YAML frontmatter fence."},
									},
									"required": []string{"slug"},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": designPlanResponseRef(),
						"400": errorResponseRef(),
						"404": errorResponseRef(),
						"409": errorResponseRef(),
					},
				},
			},
			"/foxxycode/sessions/{id}/plans/{slug}": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":    "Read one design plan",
					"parameters": []interface{}{designPlanIDParam(), designPlanSlugParam()},
					"responses": map[string]interface{}{
						"200": designPlanResponseRef(),
						"400": errorResponseRef(),
						"404": errorResponseRef(),
					},
				},
				"put": map[string]interface{}{
					"summary":     "Replace a design plan body or content",
					"description": "Send **body** to rewrite only the markdown below the frontmatter (the bundled UI autosaves this while editing the card), or **content** to replace the whole file. Sending neither is **400**.",
					"parameters":  []interface{}{designPlanIDParam(), designPlanSlugParam()},
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type": "object",
									"properties": map[string]interface{}{
										"body":    map[string]string{"type": "string", "description": "Markdown below the frontmatter; frontmatter is preserved."},
										"content": map[string]string{"type": "string", "description": "Full file content. With body, it seeds frontmatter for a plan missing on disk."},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": designPlanResponseRef(),
						"400": errorResponseRef(),
						"404": errorResponseRef(),
					},
				},
				"patch": map[string]interface{}{
					"summary":     "Update design plan frontmatter fields",
					"description": "Partial update of **name**, **overview**, and **todos** without touching the markdown body.",
					"parameters":  []interface{}{designPlanIDParam(), designPlanSlugParam()},
					"responses": map[string]interface{}{
						"200": designPlanResponseRef(),
						"400": errorResponseRef(),
						"404": errorResponseRef(),
					},
				},
				"delete": map[string]interface{}{
					"summary":     "Discard a design plan",
					"description": "Removes the plan file and marks the **plan_document** transcript row **discarded** so the card renders as dismissed.",
					"parameters":  []interface{}{designPlanIDParam(), designPlanSlugParam()},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Plan discarded."},
						"400": errorResponseRef(),
						"404": errorResponseRef(),
					},
				},
			},
			"/foxxycode/sessions/{id}/plans/{slug}/open-in-ide": map[string]interface{}{
				"post": map[string]interface{}{
					"summary": "Open the plan file in the connected editor (Show in IDE)",
					"description": "Broadcasts **`event: open_file`** on **GET /foxxycode/ide/events** so the IntelliJ / VS Code plugin opens the plan file in its own editor. " +
						"The absolute path is resolved server-side from the session bundle — the caller cannot name a file — and a plan missing on disk is **404** with nothing broadcast. " +
						"**delivered** reports whether an editor client was subscribed at that moment; the SPA renders the button only inside an editor embed.",
					"parameters": []interface{}{designPlanIDParam(), designPlanSlugParam()},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Open request broadcast to IDE clients.",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"object":    map[string]string{"type": "string"},
											"path":      map[string]string{"type": "string", "description": "Absolute path of the plan file."},
											"delivered": map[string]string{"type": "boolean", "description": "True when at least one IDE client was listening."},
										},
									},
								},
							},
						},
						"400": errorResponseRef(),
						"404": errorResponseRef(),
					},
				},
			},
		},
		"components": map[string]interface{}{
			"securitySchemes": map[string]interface{}{
				"bearerAuth": map[string]interface{}{
					"type":        "http",
					"scheme":      "bearer",
					"description": "Optional. When httpserver.auth_token (or --auth-token / FOXXYCODE_HTTP_TOKEN) is set, every /v1/* and /foxxycode/* route requires `Authorization: Bearer <token>` and returns 401 otherwise. Disabled by default. /docs and /openapi.* are also protected unless httpserver.public_docs is true. The local /foxxycode/ide/* routes stay public.",
				},
			},
			"schemas": map[string]interface{}{
				"DesignPlan": map[string]interface{}{
					"type":        "object",
					"description": "A design plan file (plans/<slug>.plan.md) inside the session bundle.",
					"properties": map[string]interface{}{
						"slug":     map[string]string{"type": "string"},
						"name":     map[string]string{"type": "string", "description": "Frontmatter name, falling back to the slug."},
						"overview": map[string]string{"type": "string", "description": "Frontmatter overview; omitted when empty."},
						"content":  map[string]string{"type": "string", "description": "Full file content including the frontmatter fence."},
						"body":     map[string]string{"type": "string", "description": "Markdown below the frontmatter."},
						"todos": map[string]interface{}{
							"type":        "array",
							"description": "Frontmatter todo steps; omitted when empty.",
							"items": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"content":  map[string]string{"type": "string"},
									"status":   map[string]string{"type": "string"},
									"priority": map[string]string{"type": "string"},
								},
							},
						},
						"updatedAt": map[string]string{"type": "string", "format": "date-time"},
					},
				},
				"CompactResult": map[string]interface{}{
					"type":        "object",
					"description": "Result of POST /foxxycode/sessions/{id}/compact.",
					"properties": map[string]interface{}{
						"compacted":          map[string]string{"type": "boolean", "description": "Whether history was compacted."},
						"reason":             map[string]string{"type": "string", "description": "Present when compacted is false (e.g. nothing_to_compact)."},
						"summary":            map[string]string{"type": "string", "description": "Generated summary text (without the transcript preamble)."},
						"compacted_messages": map[string]string{"type": "integer", "description": "How many history messages were folded into the summary."},
						"kept_messages":      map[string]string{"type": "integer", "description": "How many messages after the summary stayed verbatim."},
						"model":              map[string]string{"type": "string", "description": "models[].model that produced the summary."},
					},
				},
				"ErrorEnvelope": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"error": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"message":    map[string]string{"type": "string"},
								"code":       map[string]string{"type": "string", "description": "Machine-readable error kind when the server has one; `session_busy` for a 409 raised by a live agent turn."},
								"sessionId":  map[string]string{"type": "string", "description": "Session the error refers to (sent with `session_busy`)."},
								"turnActive": map[string]string{"type": "boolean", "description": "True when the named session has an agent turn in flight (sent with `session_busy`)."},
							},
						},
					},
				},
				"SkillRow": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"name":        map[string]string{"type": "string", "description": "Canonical skill name."},
						"description": map[string]string{"type": "string"},
						"file_path":   map[string]string{"type": "string"},
						"enabled":     map[string]interface{}{"type": "boolean", "description": "False when the skill is in the disabled list."},
						"version":     map[string]string{"type": "string", "description": "Installed version: the marketplace-declared version for synced skills, else the SKILL.md frontmatter version. Absent when unknown."},
						"source":      map[string]string{"type": "string", "description": "Configured source string when the skill was installed via `skills.sources`; absent for local/bundled skills."},
						"readonly":    map[string]interface{}{"type": "boolean", "description": "True for bundled skills, which cannot be deleted."},
					},
				},
				"SkillSyncResult": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"ok":      map[string]interface{}{"type": "boolean"},
						"added":   map[string]interface{}{"type": "array", "items": map[string]string{"type": "string"}},
						"updated": map[string]interface{}{"type": "array", "items": map[string]string{"type": "string"}},
						"failed": map[string]interface{}{
							"type": "array",
							"items": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"source": map[string]string{"type": "string"},
									"error":  map[string]string{"type": "string"},
								},
							},
						},
					},
				},
				"SkillUpdateList": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"object": map[string]string{"type": "string", "example": "foxxycode.skills_updates"},
						"items": map[string]interface{}{
							"type": "array",
							"items": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"name":             map[string]string{"type": "string", "description": "Installed remote skill name."},
									"source":           map[string]string{"type": "string", "description": "Configured source it was installed from."},
									"version":          map[string]string{"type": "string", "description": "Installed version."},
									"latest":           map[string]string{"type": "string", "description": "Latest version declared by the source."},
									"update_available": map[string]interface{}{"type": "boolean", "description": "True when latest is newer than the installed version."},
								},
							},
						},
					},
				},
				"SkillList": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"object": map[string]string{"type": "string", "example": "foxxycode.skills_list"},
						"items": map[string]interface{}{
							"type":  "array",
							"items": map[string]interface{}{"$ref": "#/components/schemas/SkillRow"},
						},
					},
				},
				"FoxxyCodeOnboardingStatus": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"first_run":          map[string]string{"type": "boolean"},
						"has_config":         map[string]string{"type": "boolean"},
						"has_providers":      map[string]string{"type": "boolean"},
						"has_models":         map[string]string{"type": "boolean"},
						"has_agent_model":    map[string]string{"type": "boolean"},
						"missing_api_keys":   map[string]interface{}{"type": "array", "items": map[string]string{"type": "string"}},
						"suggested_defaults": map[string]interface{}{"type": "object"},
					},
				},
				"FoxxyCodeProject": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"object": map[string]string{"type": "string", "example": "foxxycode.project"},
						"path":   map[string]string{"type": "string", "description": "Current project directory (working directory for new sessions)"},
						"source": map[string]interface{}{
							"type": "string",
							"enum": []interface{}{"project", "default"},
						},
						"native_picker": map[string]string{"type": "boolean", "description": "Whether the native OS folder dialog is available (desktop app)"},
					},
				},
				"FoxxyCodeRecentProjects": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"object": map[string]string{"type": "string", "example": "list"},
						"data": map[string]interface{}{
							"type": "array",
							"items": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"path":           map[string]string{"type": "string"},
									"name":           map[string]string{"type": "string", "description": "Folder basename for compact display"},
									"last_opened_at": map[string]string{"type": "string", "format": "date-time"},
									"exists":         map[string]string{"type": "boolean"},
								},
							},
						},
					},
				},
				"FoxxyCodeProjectLastSession": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"object":     map[string]string{"type": "string", "example": "foxxycode.project_last_session"},
						"path":       map[string]string{"type": "string", "description": "Project directory the record belongs to (same value as **GET** `/foxxycode/project`)"},
						"session_id": map[string]string{"type": "string", "description": "Session to reopen; empty when there is none or the recorded one is no longer usable"},
					},
				},
				"FoxxyCodeProjectPick": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"object":    map[string]string{"type": "string", "example": "foxxycode.project_pick"},
						"cancelled": map[string]string{"type": "boolean"},
						"path":      map[string]string{"type": "string", "description": "Chosen directory; empty when cancelled"},
					},
				},
				"MCPToolRow": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"name":        map[string]string{"type": "string", "description": "Tool name as advertised by the server."},
						"description": map[string]string{"type": "string"},
						"enabled":     map[string]interface{}{"type": "boolean", "description": "False when the tool is in the server's disabled-tools list."},
					},
				},
				"MCPServerRow": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"name":        map[string]string{"type": "string", "description": "Server name (unique across the merged list)."},
						"source":      map[string]interface{}{"type": "string", "enum": []string{"global", "local"}, "description": "Scope: global (config.yaml or <home>/mcp.json) or local (./.foxxycode/mcp.json)."},
						"origin":      map[string]interface{}{"type": "string", "enum": []string{"config", "home", "project"}, "description": "File that owns the definition: config.yaml, <home>/mcp.json, or ./.foxxycode/mcp.json."},
						"readonly":    map[string]interface{}{"type": "boolean", "description": "True for config.yaml-defined servers: not editable or deletable via this API."},
						"transport":   map[string]string{"type": "string", "description": "Effective transport: stdio, http (streamable, with legacy-SSE fallback), or sse."},
						"command":     map[string]string{"type": "string"},
						"args":        map[string]interface{}{"type": "array", "items": map[string]string{"type": "string"}},
						"url":         map[string]string{"type": "string"},
						"env":         map[string]interface{}{"type": "object", "additionalProperties": map[string]string{"type": "string"}},
						"headers":     map[string]interface{}{"type": "object", "additionalProperties": map[string]string{"type": "string"}, "description": "HTTP headers sent to http/sse servers."},
						"enabled":     map[string]interface{}{"type": "boolean", "description": "False when the server-level disabled switch is set."},
						"status":      map[string]interface{}{"type": "string", "enum": []string{"connected", "error", "disabled", "unsupported", "needs_approval", "denied"}, "description": "Probe result: connected (tools listed), error (probe failed), disabled (switched off), unsupported (unknown transport type), needs_approval (project entry awaiting workspace approval; not probed), denied (project entries switched off by mcp.project_trust)."},
						"error":       map[string]string{"type": "string", "description": "Probe error message when status is error or unsupported, or why the trust gate refused the entry."},
						"source_path": map[string]string{"type": "string", "description": "File the declaration was read from."},
						"trusted":     map[string]interface{}{"type": "boolean", "description": "False only for a project entry the workspace trust gate holds back."},
						"gated":       map[string]interface{}{"type": "boolean", "description": "True for project-local entries, the ones the trust gate applies to."},
						"fingerprint": map[string]string{"type": "string", "description": "Digest of the command-bearing declaration; an approval binds to this value."},
						"tools": map[string]interface{}{
							"type":  "array",
							"items": map[string]interface{}{"$ref": "#/components/schemas/MCPToolRow"},
						},
						"disabled_tools": map[string]interface{}{"type": "array", "items": map[string]string{"type": "string"}},
					},
				},
				"MCPServerList": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"object":        map[string]string{"type": "string", "example": "foxxycode.mcp_list"},
						"workspace":     map[string]string{"type": "string", "description": "Workspace the rows were merged for; approvals are recorded against it."},
						"project_trust": map[string]interface{}{"type": "string", "enum": []string{"ask", "allow", "deny"}, "description": "Effective mcp.project_trust policy."},
						"items": map[string]interface{}{
							"type":  "array",
							"items": map[string]interface{}{"$ref": "#/components/schemas/MCPServerRow"},
						},
					},
				},
				"MCPJSONServer": map[string]interface{}{
					"type":        "object",
					"description": "One mcp.json entry (global <home>/mcp.json or project .foxxycode/mcp.json; Cursor-compatible).",
					"properties": map[string]interface{}{
						"type":          map[string]interface{}{"type": "string", "enum": []string{"stdio", "http", "sse"}, "description": "Transport; empty means stdio. Inferred as http for url-only entries."},
						"command":       map[string]string{"type": "string", "description": "Executable for stdio transport."},
						"args":          map[string]interface{}{"type": "array", "items": map[string]string{"type": "string"}},
						"env":           map[string]interface{}{"type": "object", "additionalProperties": map[string]string{"type": "string"}},
						"url":           map[string]string{"type": "string", "description": "Remote endpoint for http/sse transports."},
						"headers":       map[string]interface{}{"type": "object", "additionalProperties": map[string]string{"type": "string"}},
						"disabled":      map[string]interface{}{"type": "boolean"},
						"disabledTools": map[string]interface{}{"type": "array", "items": map[string]string{"type": "string"}},
					},
				},
				"FoxxyCodeConfigJSON": map[string]interface{}{
					"type":        "object",
					"description": "FoxxyCode configuration as JSON (same logical fields as **config.yaml**). See **GET** `/foxxycode/config/schema` for the machine-readable JSON Schema.",
				},
				"FoxxyCodeConfigValidateResponse": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"ok":    map[string]string{"type": "boolean"},
						"error": map[string]string{"type": "string"},
					},
				},
				"NeuralDeepAuthStatus": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"connected": map[string]string{"type": "boolean"},
						"masked": map[string]interface{}{
							"type":        "string",
							"description": "Display mask of the stored key (`sk-ab…1234`); never the key itself.",
						},
						"key_name": map[string]string{"type": "string"},
						"source": map[string]interface{}{
							"type":        "string",
							"enum":        []string{"oauth", "api_key", "api_key_command", "env", "none"},
							"description": "Credential requests actually use. An explicit api_key / command / env var wins over a stored hub login.",
						},
					},
					"required": []string{"connected", "source"},
				},
				"CodexAuthStatus": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"connected": map[string]string{"type": "boolean"},
						"source": map[string]interface{}{
							"type": "string", "enum": []string{"foxxycode", "codex_cli"},
						},
						"account_id": map[string]string{"type": "string"},
					},
					"required": []string{"connected"},
				},
				"CodexAuthDeviceStart": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"login_id":         map[string]string{"type": "string"},
						"verification_url": map[string]interface{}{"type": "string", "format": "uri"},
						"user_code":        map[string]string{"type": "string"},
						"status":           map[string]string{"type": "string", "example": "pending"},
						"connected":        map[string]string{"type": "boolean"},
					},
					"required": []string{"login_id", "verification_url", "user_code", "status", "connected"},
				},
				"CodexAuthDeviceStatus": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"status": map[string]interface{}{
							"type": "string", "enum": []string{"pending", "completed", "failed"},
						},
						"connected": map[string]string{"type": "boolean"},
						"error":     map[string]string{"type": "string"},
					},
					"required": []string{"status", "connected"},
				},
				"ModelList": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"object": map[string]string{"type": "string", "example": "list"},
						"default_agent_model": map[string]interface{}{
							"type":        "string",
							"description": "Configured **`agent.model`** (**`models[].model`** selector). Omitted when empty. The embedded UI uses it as the default LLM choice for ReAct turns.",
						},
						"data": map[string]interface{}{
							"type": "array",
							"items": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"id":                 map[string]string{"type": "string"},
									"object":             map[string]string{"type": "string", "example": "model"},
									"created":            map[string]string{"type": "integer", "format": "int64"},
									"owned_by":           map[string]string{"type": "string", "example": "foxxycode"},
									"max_context_tokens": map[string]string{"type": "integer"},
									"multimodal":         map[string]string{"type": "boolean"},
									"reasoning_levels": map[string]interface{}{
										"type":        "array",
										"items":       map[string]string{"type": "string"},
										"description": "Reasoning levels offered for this model (e.g. minimal, low, medium, high). Models served by a `type: codex` provider report `none` instead of `minimal`, which the Codex backend rejects. Omitted for non-reasoning models.",
									},
									"reasoning_default": map[string]string{
										"type":        "string",
										"description": "Reasoning level pre-selected for new chats with this model. Omitted when none is configured.",
									},
								},
							},
						},
					},
				},
				"OpenAIMessage": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"role": map[string]interface{}{
							"type": "string",
							"enum": []interface{}{"system", "user", "assistant", "tool"},
						},
						"content": map[string]interface{}{
							"description": "JSON string or raw text/object per OpenAI client conventions.",
							"oneOf": []interface{}{
								map[string]string{"type": "string"},
								map[string]interface{}{"type": "array"},
								map[string]interface{}{"type": "object"},
							},
						},
						"reasoning": map[string]interface{}{
							"type":        "string",
							"description": "FoxxyCode transcript extension persisted model reasoning alongside assistant replies.",
						},
						"reasoning_duration_ms": map[string]interface{}{
							"type":        "integer",
							"format":      "int64",
							"description": "Wall-clock thinking span (ms). FoxxyCode persists this for UI restores.",
						},
						"model": map[string]interface{}{
							"type":        "string",
							"description": "YAML `models[].model` selector persisted on assistant replies (FoxxyCode extension).",
						},
						"files": map[string]interface{}{
							"type":        "array",
							"readOnly":    true,
							"description": "FoxxyCode transcript extension for uploaded files on a user row.",
							"items":       map[string]interface{}{"$ref": "#/components/schemas/FoxxyCodeMessageFile"},
						},
						"tool_call_id": map[string]string{"type": "string"},
						"name":         map[string]string{"type": "string"},
					},
					"required": []string{"role"},
				},
				"FoxxyCodeMessageFile": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"name":      map[string]string{"type": "string"},
						"mime_type": map[string]string{"type": "string"},
						"preview_url": map[string]interface{}{
							"type":        "string",
							"description": "Session-scoped URL for a bounded PNG preview; present only when the backend persisted a decodable image thumbnail.",
						},
					},
					"required": []string{"name", "mime_type"},
				},
				"ChatCompletionRequest": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"model": map[string]interface{}{
							"type":        "string",
							"description": "Any `id` from `GET /v1/models` (agent, plan, docs, ask, debug, or `models[].model`).",
						},
						"messages": map[string]interface{}{
							"type":  "array",
							"items": map[string]interface{}{"$ref": "#/components/schemas/OpenAIMessage"},
						},
						"stream":      map[string]string{"type": "boolean"},
						"max_tokens":  map[string]string{"type": "integer"},
						"temperature": map[string]interface{}{"type": "number", "format": "float"},
						"metadata": map[string]interface{}{
							"type":                 "object",
							"description":          "Optional. For agent/plan/docs/ask/debug only, `model` key selects `models[].model`. Not allowed for direct completion `model` values.",
							"additionalProperties": true,
						},
					},
					"required": []string{"model", "messages"},
				},
				"ChatCompletionResponse": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id":      map[string]string{"type": "string"},
						"object":  map[string]string{"type": "string", "example": "chat.completion"},
						"created": map[string]string{"type": "integer", "format": "int64"},
						"model":   map[string]string{"type": "string"},
						"metadata": map[string]interface{}{
							"type":                 "object",
							"description":          "Effective YAML model selector under `model`, optional `api_model`.",
							"additionalProperties": map[string]string{"type": "string"},
						},
						"choices": map[string]interface{}{
							"type": "array",
							"items": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"index": map[string]string{"type": "integer"},
									"message": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"role":    map[string]string{"type": "string"},
											"content": map[string]string{"type": "string"},
										},
									},
									"finish_reason": map[string]string{"type": "string"},
								},
							},
						},
					},
				},
				"ResponsesCreateRequest": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"model": map[string]interface{}{
							"type":        "string",
							"description": "Any `id` from `GET /v1/models`.",
						},
						"input": map[string]string{"type": "string"},
						"stream": map[string]interface{}{
							"type":        "boolean",
							"description": "Emit **text/event-stream** when true.",
						},
						"metadata": map[string]interface{}{
							"type":                 "object",
							"description":          "Optional. For agent/plan/docs/ask/debug only, `model` key selects `models[].model`.",
							"additionalProperties": true,
						},
						"attachments": map[string]interface{}{
							"type":        "array",
							"description": "Allowed only when **model** is **`agent`**, **`plan`**, **`docs`**, **`ask`**, or **`debug`**. Hydrated text file bodies from session **cwd** **path** fields, converted to UTF-8 when the file uses another detected encoding.",
							"items":       map[string]interface{}{"$ref": "#/components/schemas/ResponsesPromptAttachment"},
						},
						"inline_files": map[string]interface{}{
							"type":        "array",
							"description": "Supported for all modes when the effective YAML model has **`multimodal: true`**. Entries sent for a non-multimodal model are ignored and never forwarded to its provider. Each accepted file is saved to `~/.foxxycode/sessions/<id>/assets/` with read-only permissions (0o444); decodable images also get a bounded PNG thumbnail for transcript history. For **`agent`** / **`plan`** / **`docs`** / **`ask`** / **`debug`**, the model receives a `<foxxycode_session_assets>` annotation with the on-disk paths. For direct YAML model, each entry also becomes an image content part sent inline to the provider.",
							"items":       map[string]interface{}{"$ref": "#/components/schemas/ResponsesInlineFile"},
						},
					},
					"required": []string{"model", "input"},
				},
				"ResponsesPromptAttachment": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]string{
							"type":        "string",
							"description": "Relative path within session **cwd** (no traversal). Folder paths (**trailing slash**) are rejected.",
						},
						"source": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"literal": map[string]string{"type": "string"},
								"start":   map[string]string{"type": "integer"},
								"end":     map[string]string{"type": "integer"},
							},
						},
					},
					"required": []string{"path"},
				},
				"ResponsesInlineFile": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"name": map[string]string{
							"type":        "string",
							"description": "Original file name (e.g. `photo.png`). Informational only.",
						},
						"data_url": map[string]string{
							"type":        "string",
							"description": "Data URI: `data:<mime>;base64,<bytes>` or an HTTPS image URL.",
						},
					},
					"required": []string{"data_url"},
				},
				"ResponsesCreateResponse": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id":     map[string]string{"type": "string"},
						"object": map[string]string{"type": "string", "example": "response"},
						"status": map[string]string{"type": "string", "example": "completed"},
						"model":  map[string]string{"type": "string"},
						"metadata": map[string]interface{}{
							"type":                 "object",
							"additionalProperties": map[string]string{"type": "string"},
						},
						"output": map[string]interface{}{
							"type": "array",
							"items": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"type": map[string]string{"type": "string", "example": "text"},
									"text": map[string]string{"type": "string"},
								},
							},
						},
					},
				},
				"ResponsesGetResponse": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id":     map[string]string{"type": "string"},
						"object": map[string]string{"type": "string", "example": "response"},
						"status": map[string]string{"type": "string", "example": "completed"},
					},
				},
				"FoxxyCodeSlashCommandRow": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"name":        map[string]string{"type": "string", "description": "Slash command id (text after `/`)."},
						"description": map[string]string{"type": "string", "description": "Short summary for pickers."},
					},
					"required": []string{"name", "description"},
				},
				"FoxxyCodeSlashCommandsPage": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"object": map[string]string{"type": "string", "example": "foxxycode.slash_commands_page"},
						"items": map[string]interface{}{
							"type":  "array",
							"items": map[string]interface{}{"$ref": "#/components/schemas/FoxxyCodeSlashCommandRow"},
						},
						"total":     map[string]string{"type": "integer", "description": "Row count after prefix filter."},
						"has_more":  map[string]string{"type": "boolean"},
						"page":      map[string]string{"type": "integer"},
						"page_size": map[string]string{"type": "integer"},
					},
					"required": []string{"object", "items", "total", "has_more", "page", "page_size"},
				},
				"FoxxyCodeWorkspaceFileRow": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"name": map[string]string{"type": "string"},
						"path_rel": map[string]string{
							"type":        "string",
							"description": "POSIX-style relative segment from cwd. Directory rows end with **/** when **dirs=true**.",
						},
						"kind": map[string]interface{}{
							"type": "string",
							"enum": []interface{}{"file", "dir"},
						},
					},
					"required": []string{"name", "path_rel", "kind"},
				},
				"FoxxyCodeWorkspaceFilesPage": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"object": map[string]string{"type": "string", "example": "foxxycode.workspace_files_page"},
						"items": map[string]interface{}{
							"type":  "array",
							"items": map[string]interface{}{"$ref": "#/components/schemas/FoxxyCodeWorkspaceFileRow"},
						},
						"total":     map[string]string{"type": "integer"},
						"has_more":  map[string]string{"type": "boolean"},
						"page":      map[string]string{"type": "integer"},
						"page_size": map[string]string{"type": "integer"},
					},
					"required": []string{"object", "items", "total", "has_more", "page", "page_size"},
				},
				"FoxxyCodeWorkspaceContext": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"object":      map[string]string{"type": "string", "example": "foxxycode.workspace_context"},
						"path":        map[string]string{"type": "string"},
						"name":        map[string]string{"type": "string"},
						"is_git_repo": map[string]string{"type": "boolean"},
						"is_worktree": map[string]string{"type": "boolean"},
						"repo_root":   map[string]string{"type": "string"},
						"branch":      map[string]string{"type": "string"},
						"branches": map[string]interface{}{
							"type":  "array",
							"items": map[string]string{"type": "string"},
						},
						"worktrees": map[string]interface{}{
							"type": "array",
							"items": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"path":   map[string]string{"type": "string"},
									"branch": map[string]string{"type": "string"},
									"main":   map[string]string{"type": "boolean"},
								},
							},
						},
						"is_svn_repo": map[string]interface{}{
							"type":        "boolean",
							"description": "The folder is an svn working copy. Detected independently of git: a branch folder holding a git repository reports both.",
						},
						"svn": map[string]interface{}{
							"type":        "object",
							"description": "Subversion state; present whenever vcs.svn.enabled is on, with available:false when no client is installed. Absent when Subversion support is disabled.",
							"properties": map[string]interface{}{
								"available":       map[string]string{"type": "boolean"},
								"wc_root":         map[string]string{"type": "string"},
								"url":             map[string]string{"type": "string"},
								"relative_url":    map[string]string{"type": "string"},
								"repository_root": map[string]string{"type": "string"},
								"revision":        map[string]string{"type": "integer"},
								"branch": map[string]interface{}{
									"type":        "string",
									"description": "trunk, branches/<name>, or tags/<name>; empty for an unrecognised layout.",
								},
								"branches": map[string]interface{}{
									"type":  "array",
									"items": map[string]string{"type": "string"},
								},
								"nested": map[string]interface{}{
									"type":        "boolean",
									"description": "The folder itself is unversioned; the working copy root sits above it.",
								},
							},
						},
						"id": map[string]interface{}{
							"type":        "string",
							"description": "Session id (present on POST /foxxycode/sessions/{id}/workspace responses).",
						},
					},
					"required": []string{"object", "path", "name", "is_git_repo", "is_worktree", "is_svn_repo"},
				},
			},
		},
	}
	mergeOpenAPISchedulerDoc(&doc)
	mergeOpenAPIMemoryDoc(&doc)
	mergeOpenAPIMiniAppsDoc(&doc)
	return doc
}

// sessionBusyResponseRef documents the 409 raised while another agent turn holds the
// session: the body carries `code: session_busy` plus `sessionId`/`turnActive` so a client
// can re-attach to the running turn via GET /foxxycode/sessions/{id}/composer-stream.
func sessionBusyResponseRef() map[string]interface{} {
	out := errorResponseRef()
	out["description"] = "Session busy - another agent turn is in progress. `error.code` is `session_busy`, `error.sessionId` names the busy session, `error.turnActive` is true. Streaming requests wait up to 3s for the lock first, so a send issued right after `POST /foxxycode/sessions/{id}/cancel` succeeds while the cancelled turn unwinds."
	return out
}

func errorResponseRef() map[string]interface{} {
	return map[string]interface{}{
		"description": "Error",
		"content": map[string]interface{}{
			"application/json": map[string]interface{}{
				"schema": map[string]interface{}{
					"$ref": "#/components/schemas/ErrorEnvelope",
				},
			},
		},
	}
}

func codexProviderNameParameter() map[string]interface{} {
	return map[string]interface{}{
		"name":        "name",
		"in":          "path",
		"required":    true,
		"schema":      map[string]string{"type": "string"},
		"description": "Codex provider name. Valid unsaved provider names are accepted by the OAuth routes.",
	}
}

func jsonSchemaResponse(description, ref string) map[string]interface{} {
	return map[string]interface{}{
		"description": description,
		"content": map[string]interface{}{
			"application/json": map[string]interface{}{
				"schema": map[string]interface{}{"$ref": ref},
			},
		},
	}
}

func mcpServerNameParam() map[string]interface{} {
	return map[string]interface{}{
		"name": "name", "in": "path", "required": true,
		"schema":      map[string]string{"type": "string"},
		"description": "MCP server name (no `__`, spaces, or path separators).",
	}
}

func mcpToolNameParam() map[string]interface{} {
	return map[string]interface{}{
		"name": "tool", "in": "path", "required": true,
		"schema":      map[string]string{"type": "string"},
		"description": "Tool name as advertised by the server.",
	}
}

func foxxycodePagingParams() []interface{} {
	return []interface{}{
		map[string]interface{}{
			"name": "limit", "in": "query", "schema": map[string]string{"type": "string"},
			"description": "Maximum rows (default 50, capped at 100).",
		},
		map[string]interface{}{
			"name": "cursor", "in": "query", "schema": map[string]string{"type": "string"},
			"description": "Numeric offset for the next results page.",
		},
		map[string]interface{}{
			"name":        "q",
			"in":          "query",
			"schema":      map[string]string{"type": "string"},
			"description": `Optional substring filter over session title OR the first persisted user message content only (case-insensitive). Other messages are not searched.`,
		},
	}
}

func designPlanIDParam() map[string]interface{} {
	return map[string]interface{}{
		"name": "id", "in": "path", "required": true,
		"schema":      map[string]string{"type": "string"},
		"description": "Session id.",
	}
}

func designPlanSlugParam() map[string]interface{} {
	return map[string]interface{}{
		"name": "slug", "in": "path", "required": true,
		"schema":      map[string]string{"type": "string"},
		"description": "Plan slug (lowercase alphanumeric and hyphens).",
	}
}

func designPlanResponseRef() map[string]interface{} {
	return map[string]interface{}{
		"description": "Design plan document.",
		"content": map[string]interface{}{
			"application/json": map[string]interface{}{
				"schema": map[string]interface{}{"$ref": "#/components/schemas/DesignPlan"},
			},
		},
	}
}

func encodeOpenAPIYAML() ([]byte, error) {
	doc := openAPISpec()
	data, err := yaml.Marshal(doc)
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func (s *Server) handleOpenAPIYAML(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	data, err := encodeOpenAPIYAML()
	if err != nil {
		s.log.Error("openapi yaml", "error", err)
		http.Error(w, "failed to build OpenAPI document", http.StatusInternalServerError)
		return
	}
	// Inline + text-ish type so browsers show the document instead of forcing download (application/yaml often saves a file).
	w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
	w.Header().Set("Content-Disposition", `inline; filename="openapi.yaml"`)
	_, _ = w.Write(data)
}

func (s *Server) handleOpenAPIJSON(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(openAPISpec()); err != nil {
		s.log.Error("openapi json", "error", err)
		http.Error(w, "failed to build OpenAPI document", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `inline; filename="openapi.json"`)
	_, _ = w.Write(buf.Bytes())
}
