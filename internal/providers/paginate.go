package providers

import (
	"context"
	"log/slog"
	"time"

	"github.com/rafi/gits/internal/logging"
)

// paginate drives a provider fetch loop: it traces each page, invokes
// fetchPage until it reports there are no further pages, and stops early on
// context cancellation.
//
// pause is asked, once per gap between pages, how long to wait before the
// next one. It is asked after the page it prices, so a provider whose
// response carries its own rate-limit budget can answer from what it just
// read. A positive answer sleeps without ignoring cancellation.
//
// Page fetches are debug tracing, not Diagnostic Output: a default run says
// nothing about them, and `-v` shows the walk.
func paginate(
	ctx context.Context,
	logger *slog.Logger,
	what string,
	pause func() time.Duration,
	fetchPage func(page int) (more bool, err error),
) error {
	logger = logging.Or(logger)
	for page := 1; ; page++ {
		logger.DebugContext(ctx, "fetching page", "what", what, "page", page)
		more, err := fetchPage(page)
		if err != nil {
			return err
		}
		if !more {
			return nil
		}
		if wait := pause(); wait > 0 {
			logger.DebugContext(ctx, "waiting before the next page",
				"what", what, "wait", wait.Round(time.Millisecond))
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
			}
		} else if err := ctx.Err(); err != nil {
			return err
		}
	}
}

// constantPause is the pause callback for providers that learn nothing from a
// page about when to ask for the next one.
func constantPause(d time.Duration) func() time.Duration {
	return func() time.Duration { return d }
}
