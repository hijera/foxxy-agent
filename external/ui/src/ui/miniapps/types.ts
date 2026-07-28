export type MiniAppState = "draft" | "released";

export type MiniAppCatalogEntry = {
  id: string;
  name: string;
  description: string;
  state: MiniAppState;
  version?: string;
  revision?: string;
  tags?: string[];
  archived?: boolean;
  updated_at: string;
};

export type MiniAppInput = {
  id: string;
  type: string;
  title: string;
  description?: string;
  required?: boolean;
  default?: unknown;
  validation?: { enum?: unknown[]; pattern?: string };
  ui: { control: string; order?: number; placeholder?: string };
  visible_when?: MiniAppCondition;
  enabled_when?: MiniAppCondition;
  required_when?: MiniAppCondition;
};

export type MiniAppCondition = {
  op: string;
  args?: MiniAppCondition[];
  left?: unknown;
  right?: unknown;
  value?: unknown;
};

export type MiniAppStep = {
  id: string;
  kind: string;
  title: string;
  [key: string]: unknown;
};

export type MiniAppSuccess = {
  mode: "all" | "any";
  expectations?: string;
  expected_result?: string;
  acceptance_criterion?: string;
  checks: Array<{
    kind: string;
    prompt?: string;
    model_binding?: string;
    [key: string]: unknown;
  }>;
};

export type MiniAppDocument = {
  schema_version: "1.0.0";
  kind: "foxxycode.miniapp";
  id: string;
  version?: string;
  state: MiniAppState;
  revision?: string;
  metadata: {
    name: string;
    description: string;
    goal: string;
    author?: string;
    tags?: string[];
    archived?: boolean;
  };
  requirements?: Record<string, unknown>;
  permissions?: Record<string, unknown>;
  inputs: MiniAppInput[];
  workflow: MiniAppStep[];
  success: MiniAppSuccess;
  outputs: Array<{
    id: string;
    type: string;
    value: unknown;
    renderer?: string;
    title?: string;
  }>;
  display?: Record<string, unknown>;
  runtime: Record<string, unknown>;
};

export type MiniAppRun = {
  id: string;
  app_id: string;
  status: "pending" | "running" | "succeeded" | "failed" | "cancelled";
  outputs?: Record<string, unknown>;
  error?: string;
};

export type DistillationJob = {
  id: string;
  session_id: string;
  status: "queued" | "analyzing" | "completed" | "failed" | "cancelled";
  phase: string;
  progress: number;
  app_id?: string;
  summary?: string;
  error?: string;
};

export type SourceEvidence = {
  session_id?: string;
  accepted_result?: string;
  sanitized_user?: string;
  created_at: string;
};

export type ExpectedResultGeneration = {
  app: MiniAppDocument;
  suggestion: {
    expectations: string;
    expected_result: string;
    acceptance_criterion: string;
    model_binding: string;
  };
};
