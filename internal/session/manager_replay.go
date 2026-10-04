package session

import (
	"strings"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/mention"
)

func (m *Manager) replayConversation(sessionID string, msgs []llm.Message, sessionDir string) error {
	if m.server == nil {
		return nil
	}

	for i := 0; i < len(msgs); i++ {
		msg := msgs[i]

		// Messages superseded by auto-compaction are kept on disk but not re-rendered on reload.
		if msg.Compacted {
			continue
		}
		// The synthetic summary message stands in for the compacted turns: show it as a distinct
		// compaction note followed by its summary text.
		if msg.CompactionSummary {
			_ = m.server.SendSessionUpdate(sessionID, acp.CompactionUpdate{
				SessionUpdate: acp.UpdateTypeCompaction,
				Phase:         acp.CompactionPhaseDone,
			})
			if txt := strings.TrimSpace(msg.Content); txt != "" {
				_ = m.server.SendSessionUpdate(sessionID, acp.MessageChunkUpdate{
					SessionUpdate: "agent_message_chunk",
					Content:       acp.ContentBlock{Type: acp.ContentTypeText, Text: txt},
				})
			}
			continue
		}

		switch msg.Role {
		case llm.RoleUser:
			// A woken turn's first message was not typed by anybody: it is
			// replayed as the wake it stands for, so a client shows what it
			// showed live - nothing, or a one-line note - instead of a
			// message from the user.
			if msg.BackgroundWake != nil {
				_ = m.server.SendSessionUpdate(sessionID, BackgroundWakeUpdate(msg.BackgroundWake))
				continue
			}
			// The attachments a message was sent with ride in its content;
			// a client shows the mentions that brought them, not their bodies
			// (mention.ForDisplay, the web UI's stripFoxxyCodeAttachments twin).
			content := strings.TrimSpace(mention.ForDisplay(StripContextBlocks(msg.Content, TagSessionAssets)))
			if content != "" {
				_ = m.server.SendSessionUpdate(sessionID, acp.MessageChunkUpdate{
					SessionUpdate: "user_message_chunk",
					Content:       acp.ContentBlock{Type: acp.ContentTypeText, Text: content},
				})
			}

		case llm.RoleAssistant:
			if txt := strings.TrimSpace(msg.Content); txt != "" {
				_ = m.server.SendSessionUpdate(sessionID, acp.MessageChunkUpdate{
					SessionUpdate: "agent_message_chunk",
					Content:       acp.ContentBlock{Type: acp.ContentTypeText, Text: txt},
				})
			}
			for _, tc := range msg.ToolCalls {
				_ = m.server.SendSessionUpdate(sessionID, acp.ToolCallUpdate{
					SessionUpdate: acp.UpdateTypeToolCall,
					ToolCallID:    tc.ID,
					Title:         tc.Name,
					Kind:          replayToolKind(tc.Name),
					Status:        "pending",
				})
			}
			for k := range msg.ToolCalls {
				tc := msg.ToolCalls[k]
				if i+1 >= len(msgs) || msgs[i+1].Role != llm.RoleTool {
					break
				}
				tm := msgs[i+1]
				i++
				display, pmeta := PreviewToolResultForSessionUpdate(tc.Name, tm.Content)
				pmeta = attachReplayTodoPlan(sessionDir, tm.ToolCallID, pmeta)
				var content []acp.ToolCallResultItem
				if display != "" {
					content = []acp.ToolCallResultItem{
						{Type: "content", Content: acp.ContentBlock{Type: acp.ContentTypeText, Text: display}},
					}
				}
				_ = m.server.SendSessionUpdate(sessionID, acp.ToolCallStatusUpdate{
					SessionUpdate: acp.UpdateTypeToolCallUpdate,
					ToolCallID:    tm.ToolCallID,
					Status:        "completed",
					Content:       content,
					Meta:          pmeta,
				})
			}

		case llm.RoleTool:
			toolName := ""
			var planSnapshot []acp.PlanEntry
			if sd := strings.TrimSpace(sessionDir); sd != "" {
				if meta, err := ReadToolCallMeta(sd, msg.ToolCallID); err == nil && meta != nil {
					toolName = meta.Name
					planSnapshot = meta.PlanSnapshot
				}
			}
			display, pmeta := PreviewToolResultForSessionUpdate(toolName, msg.Content)
			pmeta = AttachTodoPlanMeta(pmeta, planSnapshot)
			var content []acp.ToolCallResultItem
			if display != "" {
				content = []acp.ToolCallResultItem{
					{Type: "content", Content: acp.ContentBlock{Type: acp.ContentTypeText, Text: display}},
				}
			}
			_ = m.server.SendSessionUpdate(sessionID, acp.ToolCallStatusUpdate{
				SessionUpdate: acp.UpdateTypeToolCallUpdate,
				ToolCallID:    msg.ToolCallID,
				Status:        "completed",
				Content:       content,
				Meta:          pmeta,
			})

		default:
			continue
		}
	}

	return nil
}

func replayToolKind(name string) string {
	switch name {
	case "read", "glob", "grep":
		return "read"
	case "write", "edit", "apply_patch", "mkdir", "rmdir", "touch", "rm", "mv":
		return "write"
	case "run_command":
		return "run_command"
	default:
		return "other"
	}
}

// attachReplayTodoPlan adds the persisted plan snapshot of a todo tool call to the
// replayed tool_call_update meta, so a reopened session renders the same card the
// live turn did.
func attachReplayTodoPlan(sessionDir, toolCallID string, meta map[string]interface{}) map[string]interface{} {
	sd := strings.TrimSpace(sessionDir)
	if sd == "" || strings.TrimSpace(toolCallID) == "" {
		return meta
	}
	tcMeta, err := ReadToolCallMeta(sd, toolCallID)
	if err != nil || tcMeta == nil {
		return meta
	}
	return AttachTodoPlanMeta(meta, tcMeta.PlanSnapshot)
}
