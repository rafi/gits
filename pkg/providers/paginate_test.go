package providers

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPaginate(t *testing.T) {
	ctx := context.Background()

	t.Run("walks pages until fetchPage reports no more", func(t *testing.T) {
		pages := []int{}
		err := paginate(ctx, "things", 0, func(page int) (bool, error) {
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
		boom := errors.New("boom")
		calls := 0
		err := paginate(ctx, "things", 0, func(int) (bool, error) {
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

	t.Run("cancellation stops between pages, even mid-delay", func(t *testing.T) {
		cancelCtx, cancel := context.WithCancel(ctx)
		calls := 0
		err := paginate(cancelCtx, "things", time.Hour, func(int) (bool, error) {
			calls++
			cancel() // cancelled while "fetching"; the delay must not block
			return true, nil
		})
		if !errors.Is(err, context.Canceled) {
			t.Errorf("paginate error = %v, want context.Canceled", err)
		}
		if calls != 1 {
			t.Errorf("calls = %d, want 1 (no page after cancellation)", calls)
		}
	})
}
