export type TokenUsage = {
  inputTokens: number;
  outputTokens: number;
  totalTokens: number;
};

export type TranscriptItem =
  | {
      id: string;
      type: 'user_message';
      content: string;
    }
  | {
      id: string;
      type: 'thinking';
      status: 'in_progress' | 'completed';
      content: string;
      durationMs?: number;
      startedAtMs?: number;
    }
  | {
      id: string;
      type: 'assistant_message';
      content: string;
      streaming?: boolean;
    }
  | {
      id: string;
      type: 'tool_call';
      toolCallId: string;
      title?: string;
      kind?: string;
      status: 'pending' | 'in_progress' | 'completed' | 'failed' | 'cancelled';
      argsText?: string;
      resultText?: string;
      detailsLoaded?: boolean;
      startedAtMs?: number;
      finishedAtMs?: number;
      durationMs?: number;
    }
  | {
      id: string;
      type: 'memory_copilot';
      memoryRowId: string;
      userTurnIndex: number;
      /** Single before-main-agent memory pass (preferred). Legacy rows may omit this. */
      memoryStatus?: 'idle' | 'in_progress' | 'completed';
      memoryText?: string;
      recallStatus: 'idle' | 'in_progress' | 'completed';
      persistStatus: 'idle' | 'in_progress' | 'completed';
      recallText: string;
      recallReasoning: string;
      persistText: string;
      persistReasoning: string;
      recallDurationMs?: number;
      persistDurationMs?: number;
      /** Wall clock from memory phase start until completed (live until then). */
      memoryWallStartedAtMs?: number;
      /** When main-model thinking rows appear while recall/persist SSE is still marked busy, cap the live wall-clock label at this elapsed ms so it does not keep climbing beside thinking (see freezeMemoryWallWhenThinkingAfterRecall). */
      memoryWallLiveCapMs?: number;
      memoryWallDurationMs?: number;
      persistSaved?: boolean;
      persistRelativePath?: string;
      persistTitle?: string;
      /** Markdown body written when PersistSaved (from server, may be truncated). */
      persistSavedBody?: string;
      /** scope:relative paths read via coddy_memory_read during recall. */
      recallReadPaths?: string[];
    };

