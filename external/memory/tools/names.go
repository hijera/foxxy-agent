//go:build memory

package memtools

// Tool names for the memory copilot (never exposed to the main agent).
const (
	NameSearch = "foxxycode_memory_search"
	NameList   = "foxxycode_memory_list"
	NameRead   = "foxxycode_memory_read"
	NameMkdir  = "foxxycode_memory_mkdir"
	NameSave   = "foxxycode_memory_save"
	NameDelete = "foxxycode_memory_delete"
)
