package todo

// ValidPlanStepStatuses lists allowed values for PlanEntry.Status when updating items.
var ValidPlanStepStatuses = map[string]bool{
	"pending":     true,
	"in_progress": true,
	"completed":   true,
	"failed":      true,
	"cancelled":   true,
}
