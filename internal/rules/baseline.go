package rules

// Action names for rule mutations, written as the Action of the audit
// event chained by api/http/rules.go for every create/update/delete.
const (
	ActionRuleCreated = "rule.created"
	ActionRuleUpdated = "rule.updated"
	ActionRuleDeleted = "rule.deleted"
)

// Baseline floor action names (ADR-0002 amendments): server-observed
// categories that are audited unconditionally, regardless of tenant rule
// configuration. Tenant-defined rules are purely additive on top of this
// floor — nothing in the tenant's rule set can disable or narrow it.
const (
	ActionAuthFailure = "auth.failure"
	ActionAdminAction = "admin.action"
)

// BaselineActions is the v1 server-enforced floor: auth failures, rule
// mutations, and admin/destructive API actions.
var BaselineActions = []string{
	ActionAuthFailure,
	ActionRuleCreated,
	ActionRuleUpdated,
	ActionRuleDeleted,
	ActionAdminAction,
}
