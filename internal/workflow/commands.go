package workflow

import (
	"fmt"
	"strings"
)

type CommandContext struct {
	Actor           string
	Role            string
	ExpectedVersion int
	IdempotencyKey  string
}

func (c CommandContext) Validate(action string) error {
	if strings.TrimSpace(c.Actor) == "" {
		return fmt.Errorf("操作者不能为空")
	}
	if c.ExpectedVersion < 0 {
		return fmt.Errorf("expectedVersion 无效")
	}
	if len(c.IdempotencyKey) > 128 {
		return fmt.Errorf("幂等键过长")
	}
	if c.Role != "" && !domainRoleAllows(c.Role, action) {
		return fmt.Errorf("角色%s无权执行%s", c.Role, action)
	}
	return nil
}
func domainRoleAllows(role, action string) bool {
	switch strings.ToLower(role) {
	case "admin":
		return true
	case "editor":
		return action == "create" || action == "prepare" || action == "revise"
	case "reviewer":
		return action == "check" || action == "finding" || action == "review"
	case "delivery":
		return action == "freeze" || action == "deliver"
	}
	return false
}
func normalizeActor(v string) string  { return strings.TrimSpace(v) }
func normalizeReason(v string) string { return strings.TrimSpace(v) }
func ensureExpected(current, expected int) error {
	if current != expected {
		return fmt.Errorf("版本冲突: 期望%d，当前%d", expected, current)
	}
	return nil
}
