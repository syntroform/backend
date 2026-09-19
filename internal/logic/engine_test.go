package logic

import "testing"

func TestEvaluateSkipsHiddenBranch(t *testing.T) {
	result := Evaluate([]string{"q1", "q2", "q3"}, map[string]string{"q1": "No"}, []Rule{{Conditions: []Condition{{QuestionID: "q1", Operator: "equals", Value: "No"}}, Actions: []Action{{Type: "skip_question", TargetID: "q2"}}}}, "q1")
	if result.NextID != "q3" { t.Fatalf("expected q3, got %q", result.NextID) }
	if result.Visible["q2"] { t.Fatal("expected q2 to be hidden") }
}

func TestEvaluateEndsWhenNoVisibleItemsRemain(t *testing.T) {
	result := Evaluate([]string{"q1"}, map[string]string{}, nil, "q1")
	if !result.End { t.Fatal("expected terminal result") }
}
