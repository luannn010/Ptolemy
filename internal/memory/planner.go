package memory

import (
	"fmt"
	"strings"
)

// validateAction enforces per-type semantics the grammar can't: known type,
// non-empty retrieve query, non-empty give_up reason. The grammar already
// guarantees the JSON shape and a known type literal; this is defense in depth
// plus the empty-field rules.
func validateAction(a AgentAction) error {
	switch a.Type {
	case ActionRetrieve:
		if strings.TrimSpace(a.Query) == "" {
			return fmt.Errorf("retrieve action requires a non-empty query")
		}
	case ActionAnswer:
		// no required fields
	case ActionGiveUp:
		if strings.TrimSpace(a.Reason) == "" {
			return fmt.Errorf("give_up action requires a non-empty reason")
		}
	default:
		return fmt.Errorf("unknown action type %q", a.Type)
	}
	return nil
}
