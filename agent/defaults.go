package agent

// Model shortcuts
const (
	// DefaultModel is the default model used when none is specified.
	DefaultModel = "claude-sonnet-4-20250514"

	// ModelOpus is the most capable model.
	ModelOpus = "claude-opus-4-20250514"

	// ModelSonnet is the balanced model (default).
	ModelSonnet = "claude-sonnet-4-20250514"

	// ModelHaiku is the fastest model.
	ModelHaiku = "claude-haiku-3-5-20241022"
)

// modelShortcuts maps short names to full model IDs.
var modelShortcuts = map[string]string{
	"opus":   ModelOpus,
	"sonnet": ModelSonnet,
	"haiku":  ModelHaiku,
	"o":      ModelOpus,
	"s":      ModelSonnet,
	"h":      ModelHaiku,
}

// ExpandModel converts a short model name to its full ID.
// Returns the input unchanged if not a known shortcut.
//
// Example:
//
//	ExpandModel("opus")  // "claude-opus-4-20250514"
//	ExpandModel("sonnet") // "claude-sonnet-4-20250514"
//	ExpandModel("claude-3-opus") // "claude-3-opus" (unchanged)
func ExpandModel(m string) string {
	if full, ok := modelShortcuts[m]; ok {
		return full
	}
	return m
}

// Permission modes
type PermissionMode string

const (
	// PermissionDefault asks for permission on each action.
	PermissionDefault PermissionMode = "default"

	// PermissionAcceptEdits auto-accepts file edits.
	PermissionAcceptEdits PermissionMode = "acceptEdits"

	// PermissionBypass skips all permission checks.
	PermissionBypass PermissionMode = "bypassPermissions"

	// PermissionPlan enables plan mode.
	PermissionPlan PermissionMode = "plan"
)

// Approval represents the result of a permission check.
type Approval struct {
	decision approvalDecision
	reason   string
}

type approvalDecision int

const (
	approvalAllow approvalDecision = iota
	approvalDeny
)

// Allow creates an approval that allows the action.
func Allow() Approval {
	return Approval{decision: approvalAllow}
}

// Deny creates an approval that blocks the action.
func Deny(reason string) Approval {
	return Approval{decision: approvalDeny, reason: reason}
}
