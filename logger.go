package backlite

type (
	// Logger is used to log operations.
	Logger interface {
		// Info logs info messages.
		Info(message string, params ...any)

		// Error logs error messages.
		Error(message string, params ...any)
	}

	// noLogger is the default logger and will log nothing.
	noLogger struct{}
)

func (n noLogger) Info(message string, params ...any)  {}
func (n noLogger) Error(message string, params ...any) {}
