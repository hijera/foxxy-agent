package todo

// LLM-visible tool identifiers (ACP / prompt surface).
const (
	ToolNamePlanRead    = "coddy_todo_plan_read"
	ToolNamePlanReplace = "coddy_todo_plan_replace"
	ToolNamePlanArchive = "coddy_todo_plan_archive"
	ToolNameItemAdd     = "coddy_todo_item_add"
	ToolNameItemRemove  = "coddy_todo_item_remove"
	ToolNameItemUpdate  = "coddy_todo_item_update"
	ToolNameItemMove    = "coddy_todo_item_move"
)

// AllCoddyTodoToolNames lists every built-in coddy todo/plan tool name.
var AllCoddyTodoToolNames = []string{
	ToolNamePlanRead,
	ToolNamePlanReplace,
	ToolNamePlanArchive,
	ToolNameItemAdd,
	ToolNameItemRemove,
	ToolNameItemUpdate,
	ToolNameItemMove,
}
