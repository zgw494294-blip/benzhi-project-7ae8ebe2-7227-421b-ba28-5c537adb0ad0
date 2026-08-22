package workflow

import (
	"fmt"
	"strings"
)

var roleActions = map[string]map[string]bool{"editor": {"create": true, "prepare": true, "revise": true}, "reviewer": {"check": true, "finding": true, "review": true}, "delivery": {"freeze": true, "deliver": true}, "admin": {"create": true, "prepare": true, "revise": true, "check": true, "finding": true, "review": true, "freeze": true, "deliver": true}}

func Authorize(actor, action string) error {
	if strings.TrimSpace(actor) == "" {
		return fmt.Errorf("操作者不能为空")
	}
	if strings.Contains(actor, "/") {
		return fmt.Errorf("操作者标识非法")
	}
	return nil
}
func ActionAllowed(role, action string) bool {
	if m, ok := roleActions[strings.ToLower(role)]; ok {
		return m[action]
	}
	return false
}
