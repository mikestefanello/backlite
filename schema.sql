CREATE TABLE IF NOT EXISTS backlite_tasks (
    id text primary key DEFAULT (lower(hex(randomblob(16)))),
    created_at integer NOT NULL,
    queue text NOT NULL,
    task blob NOT NULL,
    wait_until integer,
    claimed_at integer,
    last_executed_at integer,
    attempts integer NOT NULL DEFAULT 0
) STRICT;

CREATE TABLE IF NOT EXISTS backlite_tasks_completed (
    id text primary key NOT NULL,
    created_at integer NOT NULL,
    queue text NOT NULL,
    last_executed_at integer,
    attempts integer not null,
    last_duration_micro integer,
    succeeded integer,
    task blob,
    expires_at integer,
    error text
) STRICT;

-- Do we need indexes?
-- CREATE INDEX IF NOT EXISTS backlite_tasks_created_at on backlite_tasks (created_at);