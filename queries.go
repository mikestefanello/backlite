package backlite

import (
	"fmt"
	"strings"
)

const queryInsertTask = `
	INSERT INTO backlite_tasks 
	    (id, created_at, queue, task, wait_until)
	VALUES (?, ?, ?, ?, ?)
`

const querySelectTasks = `
	SELECT 
	    id, queue, task, attempts, wait_until, created_at
	FROM 
	    backlite_tasks
	WHERE
	    claimed_at IS NULL
		OR claimed_at < ?
	ORDER BY
	    wait_until ASC,
		id ASC
	LIMIT ?
`

const queryDeleteTask = `
	DELETE FROM backlite_tasks
	WHERE id = ?
`

const queryInsertCompletedTask = `
	INSERT INTO backlite_tasks_completed
		(id, created_at, queue, last_executed_at, attempts, last_duration_micro, succeeded, task, expires_at, error)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`

const queryTaskFailed = `
	UPDATE backlite_tasks
	SET 
	    claimed_at = NULL, 
	    wait_until = ?,
	    last_executed_at = ?
	WHERE id = ?
`

const queryDeleteExpiredCompletedTasks = `
	DELETE FROM backlite_tasks_completed
	WHERE
	    expires_at IS NOT NULL
		AND expires_at <= ?
`

func queryClaimTasks(count int) string {
	const query = `
		UPDATE backlite_tasks
		SET
			claimed_at = ?,
			attempts = attempts + 1
		WHERE id IN (%s)
	`

	param := strings.Repeat("?,", count)

	return fmt.Sprintf(query, param[:len(param)-1])
}
