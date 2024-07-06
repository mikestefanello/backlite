package task

import (
	"context"
	"database/sql"
	"github.com/mikestefanello/backlite/internal/query"
	"time"
)

type Completed struct {
	ID             string
	Queue          string
	Task           []byte
	Attempts       int
	Succeeded      bool
	LastDuration   time.Duration
	ExpiresAt      *time.Time
	CreatedAt      time.Time
	LastExecutedAt time.Time
	Error          *string
}

func (c *Completed) InsertTx(ctx context.Context, tx *sql.Tx) error {
	var expiresAt *int64
	if c.ExpiresAt != nil {
		v := c.ExpiresAt.UnixMilli()
		expiresAt = &v
	}

	_, err := tx.ExecContext(
		ctx,
		query.InsertCompletedTask,
		c.ID,
		c.CreatedAt.UnixMilli(),
		c.Queue,
		c.LastExecutedAt.UnixMilli(),
		c.Attempts,
		c.LastDuration.Microseconds(),
		c.Succeeded,
		c.Task,
		expiresAt,
		c.Error,
	)
	return err
}
