export type MiniAppState = "draft" | "released" | string;

export type MiniAppInput = {
  id: string;
  type: string;
  title: string;
  description?: string;
  required?: boolean;
  default?: unknown;
  validation?: {
    enum?: unknown[];
    pattern?: string;
    minimum?: number;
    maximum?: number;
  };
  ui?: { control?: string; placeholder?: string };
};

export type MiniAppStep = {
  id: string;
  kind: string;
  title: string;
  tool?: string;
  arguments?: unknown;
  prompt?: string;
  model_binding?: string;
  tools?: string[];
  max_turns?: number;
  message?: string;
  details?: unknown;
  if?: unknown;
  then?: MiniAppStep[];
  else?: MiniAppStep[];
  app_id?: string;
  app_version?: string;
  inputs?: Record<string, unknown>;
};

export type MiniAppDocument = {
  schema_version?: string;
  kind?: string;
  id: string;
  state?: MiniAppState;
  version?: string;
  revision?: string;
  metadata: {
    name: string;
    description?: string;
    goal?: string;
    author?: string;
    tags?: string[];
  };
  permissions?: { tools?: string[]; models?: string[]; apps?: string[] };
  inputs?: MiniAppInput[];
  workflow?: MiniAppStep[];
  success?: {
    mode?: string;
    expectations?: string;
    expected_result?: string;
    acceptance_criterion?: string;
    checks?: unknown[];
  };
  outputs?: unknown[];
  display?: { title?: string; description?: string; layout?: string };
  runtime?: Record<string, unknown>;
  [key: string]: unknown;
};

export type MiniAppCatalogEntry = {
  id: string;
  name?: string;
  description?: string;
  state?: MiniAppState;
  version?: string;
  revision?: string;
  tags?: string[];
  archived?: boolean;
  updated_at?: string;
};

export type ScenarioCandidate = {
  id: string;
  task?: string;
  accepted_outcome?: string;
  confidence?: number;
  action_indexes?: number[];
};

export type MiniAppDistillation = {
  id?: string;
  job_id?: string;
  status?: string;
  phase?: string;
  progress?: number;
  message?: string;
  app_id?: string;
  candidates?: ScenarioCandidate[];
  scenario_candidates?: ScenarioCandidate[];
  error?: string;
  [key: string]: unknown;
};

export type MiniAppRun = {
  id?: string;
  run_id?: string;
  app_id?: string;
  version?: string;
  status?: string;
  phase?: string;
  progress?: number;
  message?: string;
  result?: unknown;
  outputs?: unknown;
  report?: unknown;
  error?: string;
  confirmation?: { id?: string; message?: string; details?: unknown } | null;
  proposals?: MiniAppPatch[];
  events?: MiniAppRunEvent[];
  [key: string]: unknown;
};

export type MiniAppCommandManager = {
  id: string;
  package: string;
  command: string;
};

export type MiniAppCommandStatus = {
  name: string;
  binary: string;
  description?: string;
  permission: string;
  hash: string;
  resolved_path?: string;
  installed: boolean;
  trusted: boolean;
  source: string;
  managers?: MiniAppCommandManager[];
};

export type MiniAppInstallJob = {
  id?: string;
  status?: string;
  error?: string;
  result?: unknown;
};

export type MiniAppRunEvent = {
  id?: string;
  seq?: number;
  type?: string;
  status?: string;
  message?: string;
  progress?: number;
  step_id?: string;
  created_at?: string;
  [key: string]: unknown;
};

export type MiniAppPatch = {
  id?: string;
  summary?: string;
  reason?: string;
  diff?: unknown;
  status?: string;
  [key: string]: unknown;
};

export type MiniAppAssistantMessage = {
  role: "user" | "assistant";
  content: string;
};

export type MiniAppAssistantResponse = {
  reply: string;
  changes?: string[];
  draft: MiniAppDocument;
};

export type MiniAppApiResult<T> =
  | { ok: true; data: T; status: number }
  | { ok: false; status: number; message: string };
