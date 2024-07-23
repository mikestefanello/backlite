package query

import "testing"

func TestClaimTasks(t *testing.T) {
	got := ClaimTasks(3)
	expected := `
		UPDATE backlite_tasks
		SET
			claimed_at = ?,
			attempts = attempts + 1
		WHERE id IN (?,?,?)
	`

	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}
