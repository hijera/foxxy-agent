{{if .Tools}}
## Available tools

{{.Tools}}

{{end}}
{{if .Skills}}
{{.Skills}}

{{end}}
{{if .TodoList}}
### Current todo checklist

{{.TodoList}}

{{end}}
{{if .Rules}}
{{.Rules}}

{{end}}
{{if .Instructions}}
## Project instructions

{{.Instructions}}

{{end}}
{{if .Memory}}
## Session memory

{{.Memory}}

{{end}}
## Debug mode invariant

Full tool access — but confirm the diagnosis with the user before applying a fix, and verify the fix afterward by observing its result.

## Current UTC time

{{.UTCNow}}
