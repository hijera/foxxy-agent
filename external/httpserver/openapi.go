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
			"description": "OpenAI-compatible endpoints backed by FoxxyCode sessions and agents. **`GET /v1/models`** returns one list: **agent**, **plan**, and **docs** first (**`owned_by`**: **`foxxycode`**), then every configured **`models[].model`** row (**`id`** is the YAML selector, **`owned_by`** is the provider prefix). " +
				"Classify POST **model** values: **agent** / **plan** / **docs** run the ReAct agent; a selector with **provider/rest** form (see config) that appears in **`models`** triggers a single direct LLM completion (no tools). " +
				"**`metadata.model`** may appear only on agent/plan/docs requests to set the session **`SelectedModelID`**; it is **not** allowed on direct completion. " +
				"**`metadata.reasoning`** (optional, agent/plan/docs only) sets the reasoning level; it must be one of the effective model's **`reasoning_levels`** (or null/empty to clear). " +
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
					"description": "Returns **agent**, then **plan**, then **docs** (**`owned_by`**: **`foxxycode`**), then each **`models[].model`** from configuration (**`owned_by`**: provider segment of **`id`**). " +
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
					"description": "Chat completion in OpenAI-compatible shape. **`model`** must match an **`id`** from **`GET /v1/models`**: **`agent`** / **`plan`** / **`docs`** (ReAct) or a configured **`models[].model`** YAML selector (single direct completion). " +
						"Optional **`metadata`** on agent/plan/docs only: **`metadata.model`** sets the backed LLM (**`models[].model`**); omit or omit the key to use session defaults. " +
						"**`metadata`** must not carry **`model`** for direct-completion **`model`** values. " +
						"When **stream** is true the response is **text/event-stream** (OpenAI-shaped chunks plus optional **`event: foxxycode_meta`** before **`[DONE]`**). Otherwise JSON. " +
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
						"409": errorResponseRef(),
						"500": errorResponseRef(),
					},
				},
			},
			"/v1/responses": map[string]interface{}{
				"post": map[string]interface{}{
					"summary": "Create response",
					"description": "Responses-style call with **`model`**, **`input`** text, optional **`stream`** (SSE). **`model`** is any **`id`** from **`GET /v1/models`**. " +
						"**`metadata.model`** applies only when **`model`** is **`agent`**, **`plan`**, or **`docs`**. **`attachments`** (workspace-relative **`path`** rows) hydrate UTF-8 file bodies from session **cwd** on **`agent`** / **`plan`** / **`docs`** only.",
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
							"description": "Completed JSON or streamed SSE (when **stream** is true). SSE default lines are OpenAI-style `data: { ... chat.completion.chunk ... }`. Named events: **tool_call**, **tool_call_update**, **plan**, **token_usage**, **`foxxycode_meta`** (effective **`metadata`** map last), then **`[DONE]`**.",
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
						"409": errorResponseRef(),
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
					"summary": "Workspace context for the composer chips (folder, git branch, worktree)",
					"description": "Describes the workspace of the session in **`X-FoxxyCode-Session-ID`** (or the server default cwd without the header). " +
						"With **`path`** the given folder is described instead (pre-session preview); a missing folder yields **400**. " +
						"Inside a git repository the payload adds **`repo_root`**, **`branch`**, **`branches`**, and **`worktrees`** (from `git worktree list`); **`is_worktree`** is true when the workspace is a linked (non-main) worktree.",
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
					"description": "Validates the body, writes **config.yaml** atomically, reloads in-process config. On reload failure after write, restores **config.yaml.bak** to the primary path.",
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
			},
			"/foxxycode/sessions/{id}/workspace": map[string]interface{}{
				"post": map[string]interface{}{
					"summary": "Switch the session workspace folder, git branch, or worktree",
					"description": "Body **`{\"path\": dir}`** switches the session cwd to an existing folder (skills, project rules, and slash commands are re-derived; the new cwd persists in **session.json**). " +
						"Body **`{\"branch\": b}`** checks the branch out in place; when the branch is already checked out in another worktree (including the main one) the session cwd jumps there instead. " +
						"Body **`{\"branch\": b, \"worktree\": true}`** ensures a dedicated worktree for the branch (created under **`<home>/worktrees/<repo>/`** on demand) and moves the session cwd into it. " +
						"The workspace is chosen **once per session**: as soon as the conversation has messages, switching yields **409** (`workspace is locked once the conversation starts`). " +
						"A missing folder or a branch switch outside a git repository yields **400**; git checkout/worktree failures yield **409**. The session is created on demand (draft flow). Responds with the fresh workspace context.",
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
			"/foxxycode/sessions/{id}/composer-stream": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Subscribe to live composer SSE for an in-flight turn",
					"description": "Server-Sent Events with the same **data:** and **event:** frames as **POST /v1/responses** (**stream: true**) for the active **agent**/**plan**/**docs** turn. Replays bytes generated so far, then forwards live chunks until the turn ends (relay closes). While no relay exists yet, emits **SSE comments** (`: composer stream pending`) until a composer POST attaches a relay or the wait window expires (**event: error**). Optional header **X-FoxxyCode-Session-ID** must match **{id}** when set.",
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
					"description": "Completes **`event: permission`** on **`POST /v1/responses`** (**stream: true**). Body **`toolCallId`** must match **`toolCall.toolCallId`** from the SSE payload; **`optionId`** is **`allow`**, **`allow_always`**, or **`reject`** (or send **`outcome`** **`allow`** / **`cancelled`**). Optional header **X-FoxxyCode-Session-ID** must match **{id}** when set.",
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
					"description": "Fetches the model list advertised by the named provider's server (openai: **`GET {api_base}/models`**; anthropic: **`GET {api_base}/v1/models`**). The provider is resolved from the saved config, so its credentials (`api_key` / `api_key_command` / `NAME_API_KEY`) and `proxy` apply server-side without exposing secrets. Returns **`{ok:true, models:[{id,name}]}`** on success, or **`{ok:false, error, models:[]}`** with HTTP 200 when the upstream call fails so the UI can fall back to manual model entry. Unknown provider name returns 404.",
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
					"description": "Fetches the model list for a provider that is not saved in the config yet: credentials arrive in the request body instead of being resolved by provider name (openai: **`GET {api_base}/models`**; anthropic: **`GET {api_base}/v1/models`**; empty `api_base` uses the provider type's default). Returns **`{ok:true, models:[{id,name}]}`** on success, or **`{ok:false, error, models:[]}`** with HTTP 200 when the upstream call fails so the UI can fall back to manual model entry. Malformed body or unsupported `type` returns 400.",
					"operationId": "probeProviderModels",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type":     "object",
									"required": []string{"type"},
									"properties": map[string]interface{}{
										"type":     map[string]interface{}{"type": "string", "enum": []string{"openai", "anthropic", "neuraldeep"}},
										"api_base": map[string]interface{}{"type": "string", "description": "Provider base URL (e.g. http://localhost:11434/v1). Empty uses the type default. Ignored for type neuraldeep, whose endpoint is fixed at https://api.neuraldeep.ru/v1."},
										"api_key":  map[string]interface{}{"type": "string"},
										"proxy":    map[string]interface{}{"type": "string", "description": "Optional proxy URL."},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Model list result (ok:true with models, or ok:false with error)."},
						"400": errorResponseRef(),
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
								"message": map[string]string{"type": "string"},
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
				"FoxxyCodeProjectPick": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"object":    map[string]string{"type": "string", "example": "foxxycode.project_pick"},
						"cancelled": map[string]string{"type": "boolean"},
						"path":      map[string]string{"type": "string", "description": "Chosen directory; empty when cancelled"},
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
										"description": "Reasoning levels offered for this model (e.g. minimal, low, medium, high). Omitted for non-reasoning models.",
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
						"tool_call_id": map[string]string{"type": "string"},
						"name":         map[string]string{"type": "string"},
					},
					"required": []string{"role"},
				},
				"ChatCompletionRequest": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"model": map[string]interface{}{
							"type":        "string",
							"description": "Any `id` from `GET /v1/models` (agent, plan, docs, or `models[].model`).",
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
							"description":          "Optional. For agent/plan/docs only, `model` key selects `models[].model`. Not allowed for direct completion `model` values.",
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
							"description":          "Optional. For agent/plan/docs only, `model` key selects `models[].model`.",
							"additionalProperties": true,
						},
						"attachments": map[string]interface{}{
							"type":        "array",
							"description": "Allowed only when **model** is **`agent`**, **`plan`**, or **`docs`**. Hydrated UTF-8 file bodies from session **cwd** **path** fields.",
							"items":       map[string]interface{}{"$ref": "#/components/schemas/ResponsesPromptAttachment"},
						},
						"inline_files": map[string]interface{}{
							"type":        "array",
							"description": "Supported for all modes. For **`agent`** / **`plan`** / **`docs`**: each file is saved to `~/.foxxycode/sessions/<id>/assets/` with read-only permissions (0o444) and the model receives a `<foxxycode_session_assets>` annotation with the on-disk paths. For direct YAML model: each entry becomes an image content part sent inline to the provider.",
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
						"id": map[string]interface{}{
							"type":        "string",
							"description": "Session id (present on POST /foxxycode/sessions/{id}/workspace responses).",
						},
					},
					"required": []string{"object", "path", "name", "is_git_repo", "is_worktree"},
				},
			},
		},
	}
	mergeOpenAPISchedulerDoc(&doc)
	mergeOpenAPIMemoryDoc(&doc)
	return doc
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
