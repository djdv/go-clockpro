package clockpro_test

import (
	"fmt"
	"iter"
	"slices"
	"testing"

	"github.com/djdv/go-clockpro"
)

type testCache[Key comparable, Value any] interface {
	benchCache[Key, Value]
	Len() int
	Keys() iter.Seq[Key]
}

func TestClockProPlus(t *testing.T) {
	t.Run("invalid capacity", invalidCapacity)
	t.Run("empty miss", emptyMiss)
	t.Run("basic", basic)
	t.Run("update", update)
	t.Run("minimum capacity", testMinimumCapacity)
	t.Run("capacity bounds", capacityBounds)
	t.Run("eviction order", evictionOrder)
	t.Run("readmit page", ghostHit)
	t.Run("only resident keys", keysStopsAfterResidents)
}

func invalidCapacity(t *testing.T) {
	invalidSizes := []int{-1, 0, 1}
	for _, capacity := range invalidSizes {
		t.Run(fmt.Sprintf("%d", capacity), func(t *testing.T) {
			t.Parallel()
			cache, err := clockpro.New[int, int](capacity)
			if cache != nil || err == nil {
				t.Errorf(
					"New did not return an error when passed an invalid capacity: %d",
					capacity,
				)
			}
		})
	}
}

func emptyMiss(t *testing.T) {
	t.Parallel()
	const (
		capacity = clockpro.MinimumCapacity
		key      = "whatever"
		whyMiss  = "empty cache"
	)
	cache := newCache[string, int](t, capacity)
	mustMiss(t, cache, key, whyMiss)
}

func basic(t *testing.T) {
	const (
		key      = 1
		value    = 1
		capacity = clockpro.MinimumCapacity
		errCtx   = "after add"
	)
	cache := newCache[int, int](t, capacity)
	t.Run("add", func(t *testing.T) {
		cache.Set(key, value)
	})
	t.Run("get", func(t *testing.T) {
		checkGet(t, cache, key, value, errCtx)
	})
	const wantLength = 1
	wantKeys := []int{key}
	checkSize(t, cache, wantLength, errCtx)
	keysMatch(t, cache, wantKeys, errCtx)
}

func update(t *testing.T) {
	t.Parallel()
	const (
		capacity = clockpro.MinimumCapacity
		key      = "shared"
		value    = 1
	)
	cache := newCache[string, int](t, capacity)
	t.Run("add", func(t *testing.T) {
		cache.Set(key, value)
		checkGet(t, cache, key, value, "just added")
	})
	t.Run("update", func(t *testing.T) {
		size := cache.Len()
		cache.Set(key, value)
		checkGet(t, cache, key, value, "just updated")
		checkSize(t, cache, size, "after updating page")
	})
}

func testMinimumCapacity(t *testing.T) {
	t.Parallel()
	const capacity = clockpro.MinimumCapacity
	cache, err := clockpro.New[int, int](capacity)
	if err != nil {
		t.Error(err)
	}
	addIncrementingInts(cache, capacity)
	checkSize(t, cache, capacity, "added full set")
	checkKeyLength(t, cache, capacity, "added full set")
	mustGet(t, cache, 1)
}

func capacityBounds(t *testing.T) {
	const (
		capacity          = clockpro.MinimumCapacity * 2
		msg               = "added more than capacity"
		metadataLimit     = capacity * 2
		evictionThreshold = metadataLimit + 1
	)
	for _, test := range []struct {
		name  string
		limit int
	}{
		{"at capacity", capacity},
		{"metadata limit", metadataLimit},
		{"must evict", evictionThreshold},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			cache := newCache[int, int](t, capacity)
			addIncrementingInts(cache, test.limit)
			checkSize(t, cache, capacity, msg)
			checkKeyLength(t, cache, capacity, msg)
		})
	}
}

func evictionOrder(t *testing.T) {
	const capacity = 3
	cache := newCache[int, int](t, capacity)
	t.Run("fill cache", func(t *testing.T) {
		addIncrementingInts(cache, capacity)
	})
	t.Run("access pages", func(t *testing.T) {
		// Access elements (1&2) to set their reference bit
		// (promote them on replace).
		mustGet(t, cache, 1)
		mustGet(t, cache, 2)
	})
	t.Run("evict+add page", func(t *testing.T) {
		// Inserting 4 should evict 3 (unreferenced cold).
		cache.Set(4, 4)
	})
	want := []int{1, 2, 4}
	keysMatch(
		t, cache, want,
		"unexpected keys after eviction",
	)
}

func ghostHit(t *testing.T) {
	const capacity = 2
	cache := newCache[int, int](t, capacity)
	t.Run("fill cache", func(t *testing.T) {
		addIncrementingInts(cache, capacity)
	})
	t.Run("evict and add page", func(t *testing.T) {
		// Evict 1 (cold, unreferenced).
		// Becomes HIR, moves to test/ghost list.
		cache.Set(3, 3)
	})
	t.Run("access evicted page in its test period", func(t *testing.T) {
		// Setting 1 again should be a test/ghost hit.
		// I.e. residents should be 1 and 3.
		// Adaption should increase the cold target.
		cache.Set(1, -1)
	})
	want := []int{1, 3}
	keysMatch(
		t, cache, want,
		"unexpected residents after ghost re-admit",
	)
}

func keysStopsAfterResidents(t *testing.T) {
	const capacity = 4
	cache := newCache[int, int](t, capacity)
	// Fill cache
	addIncrementingInts(cache, capacity)
	// Evict to grow test set
	for i := capacity + 1; i <= capacity*3; i++ {
		cache.Set(i, i)
	}
	var (
		got  int
		want = cache.Len() // Len should be the resident count.
	)
	for range cache.Keys() { // Count how many keys were actually emitted.
		got++
	}
	if got != want { // Mismatch implies Len or Keys is not respecting the resident boundary.
		t.Fatalf(
			"expected key count to match length"+
				"\n\tgot: %v"+
				"\n\twant: %v",
			got, want)
	}
}

func newCache[
	Key comparable, Value any,
](tb testing.TB, capacity int) testCache[Key, Value] {
	tb.Helper()
	cache, err := clockpro.New[Key, Value](capacity)
	if err != nil {
		tb.Fatal(err)
	}
	return cache
}

func mustMiss[
	Key comparable,
	Value any,
](
	tb testing.TB,
	cache testCache[Key, Value],
	key Key, why string,
) {
	tb.Helper()
	value, ok := cache.Get(key)
	if !ok {
		return
	}
	tb.Fatalf(
		"expected miss due to %s but got: %v %t",
		why, value, ok)
}

func mustGet[
	Key comparable, Value any,
](
	tb testing.TB,
	cache testCache[Key, Value],
	key Key,
) Value {
	tb.Helper()
	if got, ok := cache.Get(key); ok {
		return got
	}
	tb.Fatalf("expected value from Get for key %v", key)
	var zero Value
	return zero
}

func mustGetMsg[
	Key comparable, Value any,
](
	tb testing.TB,
	cache testCache[Key, Value],
	key Key, msg string,
) Value {
	tb.Helper()
	if got, ok := cache.Get(key); ok {
		return got
	}
	tb.Fatalf(
		"expected value from Get for key `%v` - %s",
		key, msg)
	var zero Value
	return zero
}

func checkGet[
	Key comparable, Value comparable,
](
	tb testing.TB,
	cache testCache[Key, Value],
	key Key, want Value, msg string,
) {
	tb.Helper()
	got := mustGetMsg(tb, cache, key, msg)
	if got == want {
		return
	}
	tb.Fatalf(
		"expected value to match"+
			"\n\tgot: %v"+
			"\n\twant: %v",
		got, want)
}

func checkSize[
	Key comparable, Value any,
](
	tb testing.TB,
	cache testCache[Key, Value],
	size int, action string,
) {
	tb.Helper()
	got := cache.Len()
	if got == size {
		return
	}
	tb.Fatalf(
		"expected cache to be specific size %s"+
			"\n\tgot: %d"+
			"\n\twant: %d",
		action, got, size)
}

func checkKeyLength[
	Key comparable, Value any,
](
	tb testing.TB,
	cache testCache[Key, Value],
	length int, action string,
) {
	tb.Helper()
	var got int
	for range cache.Keys() {
		got++
	}
	if got == length {
		return
	}
	tb.Fatalf(
		"expected cache to be specific size %s"+
			"\n\tgot: %d"+
			"\n\twant: %d",
		action, got, length)
}

func addIncrementingInts(cache testCache[int, int], end int) {
	for i := range end {
		indexed := i + 1
		cache.Set(indexed, indexed)
	}
}

func keysMatch[
	Key comparable,
	Value any,
](
	tb testing.TB,
	cache testCache[Key, Value],
	want []Key, msg string,
) {
	tb.Helper()
	got := cache.Keys()
	if !keysEqualUnordered(want, got) {
		tb.Fatalf(
			"%s"+
				"want: %v"+
				"\ngot %v",
			msg, want, slices.Collect(got))
	}
}

func keysEqualUnordered[Key comparable](want []Key, seq iter.Seq[Key]) bool {
	counts := make(map[Key]int, len(want))
	for _, key := range want {
		counts[key]++
	}
	for key := range seq {
		if counts[key] == 0 {
			return false
		}
		counts[key]--
	}
	return true
}
