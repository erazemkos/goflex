package query

import (
	"context"
	"errors"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

func TestUseQueryFetchesOnceAndCaches(t *testing.T) {
	resetQueryTest(t)
	var calls atomic.Int32
	q := UseQuery(Key{"x"}, func(context.Context) (int, error) {
		calls.Add(1)
		return 7, nil
	})
	waitFor(t, func() bool { return !q.Loading() && q.Data() == 7 })
	q2 := UseQuery(Key{"x"}, func(context.Context) (int, error) {
		calls.Add(1)
		return 8, nil
	})
	if got := q2.Data(); got != 7 {
		t.Fatalf("cached data=%d", got)
	}
	if calls.Load() != 1 {
		t.Fatalf("fetch calls=%d", calls.Load())
	}
}

func TestLoadingDataAndErrorTransitions(t *testing.T) {
	resetQueryTest(t)
	releaseFetch := make(chan struct{})
	q := UseQuery(Key{"slow"}, func(context.Context) (string, error) {
		<-releaseFetch
		return "done", nil
	})
	if !q.Loading() || !q.Fetching() || q.Data() != "" {
		t.Fatalf("initial state loading=%v fetching=%v data=%q", q.Loading(), q.Fetching(), q.Data())
	}
	close(releaseFetch)
	waitFor(t, func() bool { return !q.Loading() && !q.Fetching() && q.Data() == "done" && q.Err() == nil })

	resetQueryTest(t)
	var calls atomic.Int32
	errBoom := errors.New("boom")
	errQuery := UseQuery(Key{"err"}, func(context.Context) (int, error) {
		if calls.Add(1) == 1 {
			return 0, errBoom
		}
		return 42, nil
	})
	waitFor(t, func() bool { return !errQuery.Loading() && errQuery.Err() != nil })
	if !errors.Is(errQuery.Err(), errBoom) {
		t.Fatalf("err=%v", errQuery.Err())
	}
	errQuery.Refetch()
	waitFor(t, func() bool { return errQuery.Err() == nil && errQuery.Data() == 42 })
}

func TestStaleWhileRevalidate(t *testing.T) {
	resetQueryTest(t)
	current := time.Unix(0, 0)
	now = func() time.Time { return current }
	var calls atomic.Int32
	q := UseQuery(Key{"todos"}, func(context.Context) (string, error) {
		call := calls.Add(1)
		if call == 1 {
			return "v1", nil
		}
		return "v2", nil
	}, StaleTime(100*time.Millisecond))
	waitFor(t, func() bool { return q.Data() == "v1" })
	current = current.Add(150 * time.Millisecond)
	q2 := UseQuery(Key{"todos"}, func(context.Context) (string, error) {
		calls.Add(1)
		return "v2", nil
	}, StaleTime(100*time.Millisecond))
	if q2.Loading() || q2.Data() != "v1" {
		t.Fatalf("stale render loading=%v data=%q", q2.Loading(), q2.Data())
	}
	waitFor(t, func() bool { return q2.Data() == "v2" && !q2.Fetching() })
}

func TestCacheEvictionAfterRelease(t *testing.T) {
	resetQueryTest(t)
	var scheduled func()
	afterFunc = func(_ time.Duration, fn func()) *time.Timer {
		scheduled = fn
		return nil
	}
	q := UseQuery(Key{"evict"}, func(context.Context) (int, error) { return 1, nil }, CacheTime(200*time.Millisecond))
	waitFor(t, func() bool { return q.Data() == 1 })
	q.Release()
	if scheduled == nil {
		t.Fatal("expected eviction to be scheduled")
	}
	scheduled()
	if _, ok := Snapshot()[normalize(Key{"evict"})]; ok {
		t.Fatal("entry was not evicted")
	}
}

func TestInvalidateByPrefixRefetchesMountedQueries(t *testing.T) {
	resetQueryTest(t)
	var calls atomic.Int32
	q := UseQuery(Key{"todos", 1}, func(context.Context) (int, error) {
		return int(calls.Add(1)), nil
	})
	waitFor(t, func() bool { return q.Data() == 1 })
	Invalidate(Key{"todos"})
	waitFor(t, func() bool { return q.Data() == 2 })
}

func TestMutationInvalidationAndOptimisticRollback(t *testing.T) {
	resetQueryTest(t)
	todos := []string{"a"}
	q := UseQuery(Key{"todos"}, func(context.Context) ([]string, error) {
		return append([]string(nil), todos...), nil
	})
	waitFor(t, func() bool { return reflect.DeepEqual(q.Data(), []string{"a"}) })

	create := UseMutation(func(_ context.Context, title string) (string, error) {
		todos = append(todos, title)
		return title, nil
	}, OnSuccess[string, string](func(string) { Invalidate(Key{"todos"}) }))
	_, err := create.MutateAsync("b")
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return reflect.DeepEqual(q.Data(), []string{"a", "b"}) })

	serverMayReturn := make(chan struct{})
	failed := UseMutation(func(_ context.Context, title string) (string, error) {
		<-serverMayReturn
		return "", errors.New("server failed")
	}, Optimistic[string, string](func(title string) {
		SetData(Key{"todos"}, func(old []string) []string { return append(old, title) })
	}))
	failed.Mutate("optimistic")
	waitFor(t, func() bool { return reflect.DeepEqual(q.Data(), []string{"a", "b", "optimistic"}) })
	close(serverMayReturn)
	waitFor(t, func() bool { return reflect.DeepEqual(q.Data(), []string{"a", "b"}) && failed.Err() != nil })
}

func TestTwoComponentsShareOneQuery(t *testing.T) {
	resetQueryTest(t)
	release := make(chan struct{})
	var calls atomic.Int32
	fetch := func(context.Context) (int, error) {
		calls.Add(1)
		<-release
		return 9, nil
	}
	q1 := UseQuery(Key{"shared"}, fetch)
	q2 := UseQuery(Key{"shared"}, fetch)
	waitFor(t, func() bool { return calls.Load() == 1 })
	if calls.Load() != 1 {
		t.Fatalf("fetch calls=%d", calls.Load())
	}
	if !q1.Loading() || !q2.Loading() {
		t.Fatalf("both should be loading")
	}
	close(release)
	waitFor(t, func() bool { return q1.Data() == 9 && q2.Data() == 9 })
}

func TestConcurrentMutations(t *testing.T) {
	resetQueryTest(t)
	var calls atomic.Int32
	var invalidations atomic.Int32
	m := UseMutation(func(_ context.Context, req int) (int, error) {
		calls.Add(1)
		return req, nil
	}, OnSuccess[int, int](func(int) { invalidations.Add(1) }))
	done := make(chan struct{}, 2)
	go func() { _, _ = m.MutateAsync(1); done <- struct{}{} }()
	go func() { _, _ = m.MutateAsync(2); done <- struct{}{} }()
	<-done
	<-done
	if calls.Load() != 2 || invalidations.Load() != 2 {
		t.Fatalf("calls=%d invalidations=%d", calls.Load(), invalidations.Load())
	}
	if m.Pending() {
		t.Fatal("mutation should not be pending")
	}
}

func TestFocusRefetch(t *testing.T) {
	resetQueryTest(t)
	current := time.Unix(0, 0)
	now = func() time.Time { return current }
	var calls atomic.Int32
	q := UseQuery(Key{"focus"}, func(context.Context) (int, error) {
		return int(calls.Add(1)), nil
	}, StaleTime(100*time.Millisecond), RefetchOnFocus(true))
	waitFor(t, func() bool { return q.Data() == 1 })
	Focus()
	time.Sleep(20 * time.Millisecond)
	if calls.Load() != 1 {
		t.Fatalf("fresh focus refetched: %d", calls.Load())
	}
	current = current.Add(150 * time.Millisecond)
	Focus()
	waitFor(t, func() bool { return q.Data() == 2 })
}

func TestProviderAndInspector(t *testing.T) {
	resetQueryTest(t)
	SetData(Key{"inspect"}, 1)
	if len(Inspector()) != 1 {
		t.Fatalf("inspector=%+v", Inspector())
	}
	_ = Provider()
}

func resetQueryTest(t *testing.T) {
	t.Helper()
	Clear()
	focus.Lock()
	focus.keys = map[string]struct{}{}
	focus.Unlock()
	oldNow := now
	oldAfter := afterFunc
	oldBackground := background
	now = time.Now
	afterFunc = time.AfterFunc
	background = func(fn func()) { go fn() }
	t.Cleanup(func() {
		Clear()
		focus.Lock()
		focus.keys = map[string]struct{}{}
		focus.Unlock()
		now = oldNow
		afterFunc = oldAfter
		background = oldBackground
	})
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition not met")
}
