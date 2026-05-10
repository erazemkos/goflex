package reactive

import "testing"

func TestEffectRunsImmediatelyAndOnlyForDependencies(t *testing.T) {
	count := NewSignal(1)
	name := NewSignal("Gopher")
	runs := 0
	seen := 0

	Effect(func() {
		runs++
		seen = count.Get() * 2
	})

	if runs != 1 || seen != 2 {
		t.Fatalf("initial runs=%d seen=%d", runs, seen)
	}
	name.Set("Pi")
	if runs != 1 {
		t.Fatalf("unrelated signal re-ran effect: runs=%d", runs)
	}
	count.Set(2)
	if runs != 2 || seen != 4 {
		t.Fatalf("count update runs=%d seen=%d", runs, seen)
	}
}

func TestSetEqualValueDoesNotRunEffects(t *testing.T) {
	count := NewSignal(1)
	runs := 0
	Effect(func() {
		runs++
		_ = count.Get()
	})
	count.Set(1)
	if runs != 1 {
		t.Fatalf("equal Set should be a no-op, runs=%d", runs)
	}
}

func TestEffectDependenciesAreDynamic(t *testing.T) {
	useA := NewSignal(true)
	a := NewSignal("a1")
	b := NewSignal("b1")
	var seen string
	runs := 0

	Effect(func() {
		runs++
		if useA.Get() {
			seen = a.Get()
			return
		}
		seen = b.Get()
	})

	if seen != "a1" || runs != 1 {
		t.Fatalf("initial seen=%q runs=%d", seen, runs)
	}
	b.Set("b2")
	if seen != "a1" || runs != 1 {
		t.Fatalf("inactive branch update seen=%q runs=%d", seen, runs)
	}
	useA.Set(false)
	if seen != "b2" || runs != 2 {
		t.Fatalf("branch switch seen=%q runs=%d", seen, runs)
	}
	a.Set("a2")
	if seen != "b2" || runs != 2 {
		t.Fatalf("old dependency update seen=%q runs=%d", seen, runs)
	}
	b.Set("b3")
	if seen != "b3" || runs != 3 {
		t.Fatalf("new dependency update seen=%q runs=%d", seen, runs)
	}
}

func TestDisposeStopsEffect(t *testing.T) {
	count := NewSignal(0)
	runs := 0
	dispose := Effect(func() {
		runs++
		_ = count.Get()
	})
	dispose.Dispose()
	count.Set(1)
	if runs != 1 {
		t.Fatalf("disposed effect re-ran: runs=%d", runs)
	}
}

func TestPeekDoesNotSubscribe(t *testing.T) {
	count := NewSignal(0)
	runs := 0
	Effect(func() {
		runs++
		_ = count.Peek()
	})
	count.Set(1)
	if runs != 1 {
		t.Fatalf("Peek subscribed effect: runs=%d", runs)
	}
}
