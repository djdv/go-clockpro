package clockpro

import (
	"iter"

	"github.com/djdv/go-clockpro/internal/ring"
)

type (
	page[Key comparable, Value any] = ring.Ring[Key, Value]
	metadata[Key comparable]        = ring.Metadata[Key]
	// Cache utilizes the Cache-Pro+ replacement algorithm.
	// Concurrent access must be guarded by the caller.
	// Constructed by [New].
	Cache[Key comparable, Value any] struct {
		index map[Key]*page[Key, Value]
		hot, cold,
		test, lru *page[Key, Value]
		capacity, coldTarget, hotTarget,
		coldCount, hotCount, testCount,
		demotions int
	}
)

// MinimumCapacity defines the lowest value supported by [New].
const MinimumCapacity = 2

// New creates a [Cache] with the given capacity.
// Capacity must be at least [MinimumCapacity] to allow both hot and cold cache pages.
func New[Key comparable, Value any](capacity int) (*Cache[Key, Value], error) {
	const minimumColdRatio = 0.01
	if capacity < MinimumCapacity {
		return nil, minCapacityError(capacity)
	}
	var ( // Range: [1,half-capacity]
		coldInitial = max(float64(capacity)*minimumColdRatio, 1)
		coldTarget  = min(int(coldInitial), capacity/2)
		hotTarget   = capacity - coldTarget
	)
	return &Cache[Key, Value]{
		capacity:   capacity,
		index:      make(map[Key]*page[Key, Value], hotTarget),
		coldTarget: coldTarget,
		hotTarget:  hotTarget,
	}, nil
}

// Load returns the cached value for key (if resident). Otherwise, it calls fetch,
// inserts and returns the value on success.
// If fetch returns an error, the value is not cached.
func (c *Cache[Key, Value]) Load(key Key, fetch func() (Value, error)) (Value, error) {
	if value, hadPage := c.Get(key); hadPage {
		return value, nil
	}
	value, err := fetch()
	if err != nil {
		return value, err
	}
	const hadMetadata = false
	c.handleMiss(key, value, hadMetadata)
	return value, nil
}

// Get returns the Value for key if it is resident
// in the cache, and marks it as referenced;
// otherwise it returns the zero value and false.
func (c *Cache[Key, Value]) Get(key Key) (Value, bool) {
	if page, ok := c.index[key]; ok &&
		page.Resident {
		page.Referenced = true
		return page.Value, true
	}
	var zero Value
	return zero, false
}

// Set inserts or updates key with value
// and marks it as referenced.
func (c *Cache[Key, Value]) Set(key Key, value Value) {
	page, found := c.index[key]
	if found && page.Resident {
		page.Referenced = true
		page.Value = value
		return
	}
	c.handleMiss(key, value, found)
}

// handleMiss should be called after a page access misses.
// Caller must provide if the page's metadata was present
// (even if the page's value was not resident).
func (c *Cache[Key, Value]) handleMiss(key Key, value Value, hadMetadata bool) {
	c.sweepHot()
	c.sweepCold()
	if hadMetadata {
		// If a page for the key was found and not evicted
		// by the hand sweeps above, it is resurrected as resident.
		if test, hit := c.index[key]; hit {
			c.promoteTest(test, value)
			return
		}
	}
	if c.atCapacity() {
		c.evictCold()
	}
	c.addNew(key, value)
}

func (c *Cache[_, _]) atCapacity() bool {
	return c.coldCount+c.hotCount == c.capacity
}

// addNew creates and adds a new page to the clock,
// and performs hand sweeps/actions as necessary.
func (c *Cache[Key, Value]) addNew(key Key, value Value) {
	var (
		lowIRR = c.coldCount == 0 &&
			c.hotCount < c.hotTarget
		page = &page[Key, Value]{
			Metadata: metadata[Key]{
				Name:     key,
				Resident: true,
				LIR:      lowIRR,
				Stacked:  true,
			},
			Value: value,
		}
	)
	c.addToClock(page)
	if lowIRR {
		c.hotCount++
	} else {
		if c.cold == nil {
			c.cold = page
		}
		c.coldCount++
	}
	c.sweepCold()
	c.pruneTest()
}

// promoteTest resurrects a nonresident page as resident,
// promoting it to hot. The cache targets are also adjusted.
func (c *Cache[Key, Value]) promoteTest(testToHot *page[Key, Value], value Value) {
	if debugging {
		assert(testToHot.Stacked,
			"hit a non-resident cold page out of the stack")
		assert(!testToHot.Referenced,
			"hit a referenced non-resident cold page")
		assert(c.hotCount+c.coldCount == c.capacity,
			"cache not full")
	}
	c.increaseColdTarget()
	c.evictCold()
	testToHot.Value = value
	testToHot.Resident = true
	c.testCount--
	c.coldCount++
	if testToHot == c.test {
		c.sweepTest()
	}
	c.promoteCold(testToHot)
	c.sweepCold()
}

func (c *Cache[_, _]) sweepHot() {
	if c.hotCount == 0 {
		return
	}
	page := c.hot
	for !page.LIR || page.Referenced {
		next := page.Next()
		if page.LIR {
			c.handleHotLIR(page)
		} else {
			c.handleHotHIR(page, next)
		}
		page = next
	}
	c.hot = page
}

func (c *Cache[Key, Value]) handleHotLIR(page *page[Key, Value]) {
	page.Referenced = false
	c.lru = page
}

func (c *Cache[Key, Value]) handleHotHIR(page, next *page[Key, Value]) {
	if page.Resident {
		if page.Referenced {
			page.Referenced = false
			if page.Demoted {
				c.decreaseColdTarget()
				page.Demoted = false
				c.demotions--
			}
			c.lru = page
			if page == c.cold {
				c.cold = next
			}
		} else {
			page.Stacked = false
		}
	} else {
		c.removeTest(page)
	}
}

func (c *Cache[_, _]) increaseColdTarget() {
	delta := max(
		c.demotions/c.testCount,
		1,
	)
	c.adjustColdTarget(delta)
}

func (c *Cache[_, _]) decreaseColdTarget() {
	delta := -max(
		c.testCount/c.demotions,
		1,
	)
	c.adjustColdTarget(delta)
}

func (c *Cache[_, _]) adjustColdTarget(delta int) {
	var (
		size       = c.capacity // Range: [1,half-capacity].
		diff       = max(c.coldTarget+delta, 1)
		coldTarget = min(diff, size/2)
	)
	c.coldTarget = coldTarget
	c.hotTarget = size - coldTarget
}

func (c *Cache[Key, Value]) removeTest(test *page[Key, Value]) {
	if test == c.test {
		c.test = test.Next()
	}
	delete(c.index, test.Name)
	test.Prev().Unlink(1)
	c.testCount--
	c.sweepTest()
}

func (c *Cache[_, _]) sweepTest() {
	if c.testCount == 0 {
		c.test = nil
		return
	}
	hand := c.test
	for hand.LIR || hand.Resident {
		hand = hand.Next()
	}
	c.test = hand
}

func (c *Cache[_, _]) sweepCold() {
	if c.coldCount == 0 {
		return
	}
	hand := c.cold
	for hand.LIR ||
		!hand.Resident ||
		hand.Referenced {
		page := hand
		hand = hand.Next()
		if page.LIR || !page.Referenced {
			continue
		}
		c.handleReferencedCold(page)
	}
	c.cold = hand
}

func (c *Cache[Key, Value]) handleReferencedCold(page *page[Key, Value]) {
	page.Referenced = false
	if page.Demoted {
		c.decreaseColdTarget()
		page.Demoted = false
		c.demotions--
	}
	if page.Stacked {
		c.promoteCold(page)
	} else {
		page.Stacked = true
		c.moveToLRU(page)
	}
}

func (c *Cache[Key, Value]) promoteCold(coldToHot *page[Key, Value]) {
	coldToHot.LIR = true
	c.hotCount++
	c.coldCount--
	c.moveToLRU(coldToHot)
	for c.hotCount > c.hotTarget {
		c.demoteHot()
	}
}

func (c *Cache[Key, Value]) moveToLRU(page *page[Key, Value]) {
	if page == c.lru {
		return
	}
	leaf := page.Prev().Unlink(1)
	c.lru.Link(leaf)
	c.lru = leaf
}

func (c *Cache[_, _]) demoteHot() {
	if debugging {
		assert(!c.hot.Referenced,
			"hot hand stops on a referenced page")
	}
	page := c.hot
	c.hot = page.Next()
	page.LIR = false
	page.Stacked = false
	page.Demoted = true
	c.hotCount--
	c.coldCount++
	c.demotions++
	c.moveToLRU(page)
	c.sweepHot()
}

// evictCold evicts the current cold hand.
// Eviction zeros the page's Value but retains
// metadata as a nonresident "test page" to guide adaptation.
// If the page is not stacked, it is removed entirely.
func (c *Cache[_, Value]) evictCold() {
	if debugging {
		assert(
			!c.cold.LIR && c.cold.Resident && !c.cold.Referenced,
			"cold hand does not stop at a non-referenced resident cold page")
	}
	var (
		zero Value
		page = c.cold
	)
	c.cold = page.Next()
	page.Resident = false
	page.Value = zero
	c.coldCount--
	c.testCount++
	if page.Demoted {
		page.Demoted = false
		c.demotions--
	}
	if c.test == nil {
		c.test = page
	}
	if !page.Stacked {
		if page == c.lru {
			c.lru = page.Prev()
		}
		c.removeTest(page)
	}
}

// addToClock links the page to the clock
// as well as the page index.
func (c *Cache[Key, Value]) addToClock(page *page[Key, Value]) {
	if c.lru == nil {
		c.lru = page
		c.hot = page
	} else {
		c.lru.Link(page)
		c.lru = page // == c.lru.Next().
	}
	c.index[page.Name] = page
}

func (c *Cache[_, _]) pruneTest() {
	metadataLimit := c.capacity * 2
	for c.coldCount+c.hotCount+c.testCount > metadataLimit {
		if debugging {
			assert(
				c.test.Stacked && !c.test.LIR && !c.test.Resident,
				"test hand does not stop at a test page")
		}
		c.removeTest(c.test)
	}
}

// Len returns the number of resident pages.
func (c *Cache[_, _]) Len() int {
	return c.hotCount + c.coldCount
}

// Keys returns an iterator over the (unordered) keys of resident pages.
func (c *Cache[Key, _]) Keys() iter.Seq[Key] {
	return func(yield func(Key) bool) {
		residents := c.Len()
		for key, page := range c.index {
			if page.Resident {
				if !yield(key) {
					return
				}
				if residents--; residents == 0 {
					return
				}
			}
		}
	}
}
