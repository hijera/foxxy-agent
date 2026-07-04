package todo

// LLM-visible tool identifiers (ACP / prompt surface).
const (
	ToolNamePlanRead    = "foxxycode_todo_plan_read"
	ToolNamePlanReplace = "foxxycode_todo_plan_replace"
	ToolNamePlanArchive = "foxxycode_todo_plan_archive"
	ToolNameItemAdd     = "foxxycode_todo_item_add"
	ToolNameItemRemove  = "foxxycode_todo_item_remove"
	ToolNameItemUpdate  = "foxxycode_todo_item_update"
	ToolNameItemMove    = "foxxycode_todo_item_move"
)

// AllFoxxyCodeTodoToolNames lists every built-in foxxycode todo/plan tool name.
var AllFoxxyCodeTodoToolNames = []string{
	ToolNamePlanRead,
	ToolNamePlanReplace,
	ToolNamePlanArchive,
	ToolNameItemAdd,
	ToolNameItemRemove,
	ToolNameItemUpdate,
	ToolNameItemMove,
}
