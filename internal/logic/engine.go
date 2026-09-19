package logic

// Package logic contains the single server-side path evaluator used by public
// form actions. Rules reference stable IDs; titles are only presentation data.
type Condition struct{ QuestionID, Operator, Value string }
type Action struct{ Type, TargetID string }
type Rule struct {
	Conditions []Condition
	Actions    []Action
}

type Result struct {
	Visible map[string]bool
	NextID  string
	End     bool
}

// Evaluate applies rules in order. The default path is the next item in the
// supplied ordered IDs; hide/skip/jump/end actions then alter that path.
func Evaluate(orderedIDs []string, answers map[string]string, rules []Rule, currentID string) Result {
	visible := make(map[string]bool, len(orderedIDs))
	for _, id := range orderedIDs {
		visible[id] = true
	}
	for _, rule := range rules {
		matched := true
		for _, condition := range rule.Conditions {
			actual := answers[condition.QuestionID]
			switch condition.Operator {
			case "equals":
				matched = matched && actual == condition.Value
			case "not_equals":
				matched = matched && actual != condition.Value
			case "contains":
				matched = matched && contains(actual, condition.Value)
			case "is_answered":
				matched = matched && actual != ""
			case "is_not_answered":
				matched = matched && actual == ""
			}
		}
		if !matched {
			continue
		}
		for _, action := range rule.Actions {
			switch action.Type {
			case "hide_question", "skip_question":
				visible[action.TargetID] = false
			case "show_question":
				visible[action.TargetID] = true
			case "end_form":
				return Result{Visible: visible, End: true}
			case "jump_question":
				if visible[action.TargetID] {
					return Result{Visible: visible, NextID: action.TargetID}
				}
			}
		}
	}
	start := -1
	for i, id := range orderedIDs {
		if id == currentID {
			start = i
			break
		}
	}
	for i := start + 1; i < len(orderedIDs); i++ {
		if visible[orderedIDs[i]] {
			return Result{Visible: visible, NextID: orderedIDs[i]}
		}
	}
	return Result{Visible: visible, End: true}
}

// Next returns the next visible item after currentID and is useful to both
// question-by-question and section-by-section public renderers.
func Next(orderedIDs []string, answers map[string]string, rules []Rule, currentID string) Result {
	return Evaluate(orderedIDs, answers, rules, currentID)
}

func contains(value, wanted string) bool {
	for i := 0; i+len(wanted) <= len(value); i++ {
		if value[i:i+len(wanted)] == wanted {
			return true
		}
	}
	return false
}
