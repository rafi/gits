package providers

import (
	"context"
	"time"

	log "github.com/sirupsen/logrus"
)

// paginate drives a provider fetch loop: it logs each page, invokes
// fetchPage until it reports there are no further pages, and stops early on
// context cancellation. A positive delay sleeps between pages without
// ignoring cancellation.
func paginate(
	ctx context.Context,
	what string,
	delay time.Duration,
	fetchPage func(page int) (more bool, err error),
) error {
	for page := 1; ; page++ {
		log.Infof("Fetching %s (%d)…", what, page)
		more, err := fetchPage(page)
		if err != nil {
			return err
		}
		if !more {
			return nil
		}
		if delay > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		} else if err := ctx.Err(); err != nil {
			return err
		}
	}
}
