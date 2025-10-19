// Package ring is a specialized adaption of `container/ring` for use in LIRS.
package ring

import "iter"

type (
	// A Ring is an element of a circular list, or ring.
	// Rings do not have a beginning or end; a pointer to any ring element
	// serves as reference to the entire ring. Empty rings are represented
	// as nil Ring pointers. The zero value for a Ring is a one-element
	// ring with a nil Value.
	Ring[Key comparable, Value any] struct {
		next, prev *Ring[Key, Value]
		Value      Value
		Metadata[Key]
	}
	// Metadata stores LIRS (Low Inter‑Reference Recency Set) state of a cache page.
	// It is used by CLOCK‑Pro and related eviction algorithms.
	Metadata[Key comparable] struct {
		// Name is the identifier of the data this metadata is bound to.
		Name Key
		// LIR (Low Inter-Reference Recency) is true
		// if the page is frequently accessed relative to other pages,
		// to be spared from eviction because of its short "reuse distance".
		// See LIRS algorithm for more detail.
		LIR bool
		// Resident is true if the data this metadata
		// is associated with, is to be considered valid.
		// I.e. true if the data is still stored in memory
		// and has not been nullified via eviction.
		Resident bool
		// Demoted is true if the page has been moved
		// to the test/ghost list (HIR non-resident).
		Demoted bool
		// Referenced is true if the page was
		// accessed since the last sweep.
		Referenced bool
		// Stacked is true if the page is currently in the LRU/LIRS stack.
		Stacked bool
	}
)

func (r *Ring[Key, Value]) init() *Ring[Key, Value] {
	r.next = r
	r.prev = r
	return r
}

// Next returns the next ring element. r must not be empty.
func (r *Ring[Key, Value]) Next() *Ring[Key, Value] {
	if r.next == nil {
		return r.init()
	}
	return r.next
}

// Prev returns the previous ring element. r must not be empty.
func (r *Ring[Key, Value]) Prev() *Ring[Key, Value] {
	if r.next == nil {
		return r.init()
	}
	return r.prev
}

// Move moves n % r.Len() elements backward (n < 0) or forward (n >= 0)
// in the ring and returns that ring element. r must not be empty.
func (r *Ring[Key, Value]) Move(n int) *Ring[Key, Value] {
	if r.next == nil {
		return r.init()
	}
	switch {
	case n < 0:
		for ; n < 0; n++ {
			r = r.prev
		}
	case n > 0:
		for ; n > 0; n-- {
			r = r.next
		}
	}
	return r
}

// New creates a ring of n elements.
func New[Key comparable, Value any](n int) *Ring[Key, Value] {
	if n <= 0 {
		return nil
	}
	var (
		r = new(Ring[Key, Value])
		p = r
	)
	for i := 1; i < n; i++ {
		p.next = &Ring[Key, Value]{prev: p}
		p = p.next
	}
	p.next = r
	r.prev = p
	return r
}

// Link connects ring r with ring s such that r.Next()
// becomes s and returns the original value for r.Next().
// r must not be empty.
//
// If r and s point to the same ring, linking
// them removes the elements between r and s from the ring.
// The removed elements form a subring and the result is a
// reference to that subring (if no elements were removed,
// the result is still the original value for r.Next(),
// and not nil).
//
// If r and s point to different rings, linking
// them creates a single ring with the elements of s inserted
// after r. The result points to the element following the
// last element of s after insertion.
func (r *Ring[Key, Value]) Link(s *Ring[Key, Value]) *Ring[Key, Value] {
	n := r.Next()
	if s != nil {
		p := s.Prev()
		// Note: Cannot use multiple assignment because
		// evaluation order of LHS is not specified.
		r.next = s
		s.prev = r
		n.prev = p
		p.next = n
	}
	return n
}

// Unlink removes n % r.Len() elements from the ring r, starting
// at r.Next(). If n % r.Len() == 0, r remains unchanged.
// The result is the removed subring. r must not be empty.
func (r *Ring[Key, Value]) Unlink(n int) *Ring[Key, Value] {
	if n <= 0 {
		return nil
	}
	return r.Link(r.Move(n + 1))
}

// Len computes the number of elements in ring r.
// It executes in time proportional to the number of elements.
func (r *Ring[Key, Value]) Len() int {
	n := 0
	if r != nil {
		n = 1
		for p := r.Next(); p != r; p = p.next {
			n++
		}
	}
	return n
}

// Do calls function f on each element of the ring, in forward order,
// stopping early if yield returns false.
// The behavior of Do is undefined if f changes *r.
func (r *Ring[Key, Value]) Do(yield func(Value) bool) {
	r.do(func(r *Ring[Key, Value]) bool {
		return yield(r.Value)
	})
}

func (r *Ring[Key, Value]) do(yield func(*Ring[Key, Value]) bool) {
	if r == nil ||
		!yield(r) {
		return
	}
	for p := r.Next(); p != r; p = p.next {
		if !yield(p) {
			return
		}
	}
}

func (r *Ring[Key, Value]) Iter() iter.Seq[*Ring[Key, Value]] {
	return func(yield func(*Ring[Key, Value]) bool) {
		r.do(yield)
	}
}
