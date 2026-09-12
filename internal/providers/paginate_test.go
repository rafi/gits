package providers

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPaginate(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("walks pages until fetchPage reports no more", func(t *testing.T) {
		t.Parallel()

		pages := []int{}
		err := paginate(ctx, nil, "things", constantPause(0), func(page int) (bool, error) {
			pages = append(pages, page)
			return page < 3, nil
		})
		if err != nil {
			t.Fatalf("paginate: %v", err)
		}
		if len(pages) != 3 || pages[0] != 1 || pages[2] != 3 {
			t.Errorf("pages fetched = %v, want [1 2 3]", pages)
		}
	})

	t.Run("stops on fetch error", func(t *testing.T) {
		t.Parallel()

		boom := errors.New("boom")
		calls := 0
		err := paginate(ctx, nil, "things", constantPause(0), func(int) (bool, error) {
			calls++
			return true, boom
		})
		if !errors.Is(err, boom) {
			t.Errorf("paginate error = %v, want boom", err)
		}
		if calls != 1 {
			t.Errorf("calls = %d, want 1", calls)
		}
	})

	// A zero pause skips the select entirely, so cancellation has to be
	// checked on its own — the providers that pace nothing are the ones with
	// the most pages to walk.
	t.Run("cancellation stops between pages with no pause to interrupt", func(t *testing.T) {
		t.Parallel()

		cancelCtx, cancel := context.WithCancel(ctx)
		calls := 0
		// Bounded, so a loop that ignores cancellation ends the test with a
		// failure rather than spinning until the suite times out.
		err := paginate(cancelCtx, nil, "things", constantPause(0), func(page int) (bool, error) {
			calls++
			cancel()
			return page < 10, nil
		})
		if !errors.Is(err, context.Canceled) {
			t.Errorf("paginate error = %v, want context.Canceled", err)
		}
		if calls != 1 {
			t.Errorf("calls = %d, want 1 (no page after cancellation)", calls)
		}
	})

	t.Run("cancellation stops between pages, even mid-pause", func(t *testing.T) {
		t.Parallel()

		cancelCtx, cancel := context.WithCancel(ctx)
		calls := 0
		err := paginate(cancelCtx, nil, "things", constantPause(time.Hour), func(int) (bool, error) {
			calls++
			cancel() // canceled while "fetching"; the pause must not block
			return true, nil
		})
		if !errors.Is(err, context.Canceled) {
			t.Errorf("paginate error = %v, want context.Canceled", err)
		}
		if calls != 1 {
			t.Errorf("calls = %d, want 1 (no page after cancellation)", calls)
		}
	})

	// The pause is asked once per gap between pages, and never after the last
	// one — a provider that reads its rate-limit budget must not be charged a
	// wait it will never spend.
	t.Run("pause is consulted once between pages and not after the last", func(t *testing.T) {
		t.Parallel()

		asked := 0
		pause := func() time.Duration {
			asked++
			return 0
		}
		err := paginate(ctx, nil, "things", pause, func(page int) (bool, error) {
			return page < 3, nil
		})
		if err != nil {
			t.Fatalf("paginate: %v", err)
		}
		if asked != 2 {
			t.Errorf("pause consulted %d times over 3 pages, want 2", asked)
		}
	})

	// The pause is recomputed per page rather than sampled once, so a budget
	// that degrades mid-walk is reflected on the very next gap.
	t.Run("pause is recomputed for every gap", func(t *testing.T) {
		t.Parallel()

		waits := []time.Duration{0, time.Millisecond, 0}
		asked := 0
		pause := func() time.Duration {
			d := waits[asked]
			asked++
			return d
		}
		err := paginate(ctx, nil, "things", pause, func(page int) (bool, error) {
			return page < 4, nil
		})
		if err != nil {
			t.Fatalf("paginate: %v", err)
		}
		if asked != 3 {
			t.Errorf("pause consulted %d times over 4 pages, want 3", asked)
		}
	})
}
