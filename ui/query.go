package ui

const itemLimit = 25

const selectRunningTasks = `
	SELECT 
	    id, queue, task, attempts, wait_until, created_at
	FROM 
	    backlite_tasks
	WHERE
	    claimed_at IS NOT NULL
	LIMIT ?
`
const selectCompletedTasks = `
	SELECT *
	FROM
	    backlite_tasks_completed 
	WHERE
	    succeeded = ?
	ORDER BY
	    last_executed_at DESC
	LIMIT ?
`
