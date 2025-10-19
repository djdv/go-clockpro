package clockpro_test

import (
	"fmt"
	"math/bits"
	"math/rand"
	"testing"
	"unsafe"

	"github.com/hashicorp/golang-lru/arc/v2"
)

type (
	benchCache[Key comparable, Value any] interface {
		Set(Key, Value)
		Get(Key) (Value, bool)
	}
	cacheCtor        = func(capacity int, b *testing.B) benchCache[int, int]
	cacheConstructor struct {
		name string
		new  cacheCtor
	}
	patternGen    = func(capacity int) []int
	accessPattern struct {
		name string
		gen  patternGen
	}
	arcWrapper[Key comparable, Value any] struct {
		*arc.ARCCache[Key, Value]
	}
)

func (aw arcWrapper[Key, Value]) Set(key Key, value Value) { aw.Add(key, value) }

// Fixed RNG seed for reproducibility.
// Change to test variance between runs.
const rngSeed = 1

func BenchmarkCache(b *testing.B) {
	b.Run("API overhead", apiOverhead)
	var (
		constructors = cacheConstructors()
		capacities   = []int{128, 512, 2048}
		patterns     = accessPatterns()
	)
	runPatterns(b, constructors, capacities, patterns)
}

func cacheConstructors() []cacheConstructor {
	return []cacheConstructor{
		{
			"ClockProPlus",
			func(capacity int, b *testing.B) benchCache[int, int] {
				return newCache[int, int](b, capacity)
			},
		},
		{
			"ARC",
			func(capacity int, b *testing.B) benchCache[int, int] {
				cache, err := arc.NewARC[int, int](capacity)
				if err != nil {
					b.Fatal(err)
				}
				return arcWrapper[int, int]{ARCCache: cache}
			},
		},
	}
}

func accessPatterns() []accessPattern {
	return []accessPattern{
		{
			"Sequential scan",
			func(int) []int {
				const (
					universe = 1 << 16 // Key space large enough to force misses.
					seqLen   = 1 << 15 // Power of two for cheap masking.
				)
				return makeSequential(universe, seqLen)
			},
		},
		{
			"Loop working set",
			func(capacity int) []int {
				const (
					universe = 8192 // Moderately larger than capacity.
					seqLen   = 1 << 16
					hotRatio = 0.9 // 90% of accesses hit hot set.
				)
				return makeLooping(capacity, universe, seqLen, hotRatio)
			},
		},
		{
			"Zipf",
			func(int) []int {
				const (
					universe = 16384 // Large enough to show skew.
					seqLen   = 1 << 16
					skew     = 1.2
					bias     = 1.0
				)
				return makeZipf(universe, seqLen, skew, bias)
			},
		},
		{
			"Uniform random",
			func(capacity int) []int {
				const seqLen = 1 << 16
				var (
					rng        = newReproducibleRNG()
					keyCount   = nextPow2(seqLen)
					upperBound = capacity * 4 // Universe bigger than capacity.
					seq        = makeRandomSequence(rng, upperBound, keyCount)
				)
				return seq
			},
		},
	}
}

func runPatterns(b *testing.B, constructors []cacheConstructor, capacities []int, patterns []accessPattern) {
	type (
		Key   = int
		Value = int
	)
	const (
		keySize   = unsafe.Sizeof(Key(0))
		valueSize = unsafe.Sizeof(Value(0))
		dataSize  = int64(keySize + valueSize)
	)
	for _, pattern := range patterns {
		b.Run(pattern.name, newBenchPattern(
			pattern.gen, capacities,
			constructors, dataSize,
		))
	}
}

func newBenchPattern(
	genPattern patternGen, capacities []int,
	constructors []cacheConstructor, dataSize int64,
) func(b *testing.B) {
	return func(b *testing.B) {
		for _, capacity := range capacities {
			var (
				name     = fmt.Sprintf("Cap%d", capacity)
				sequence = genPattern(capacity)
			)
			b.Run(name, newBenchCapacity(
				constructors, capacity,
				dataSize, sequence,
			))
		}
	}
}

func newBenchCapacity(
	constructors []cacheConstructor, capacity int,
	dataSize int64, sequence []int,
) func(b *testing.B) {
	return func(b *testing.B) {
		for _, constructor := range constructors {
			b.Run(constructor.name,
				newBenchCache(
					constructor.new, capacity,
					dataSize, sequence,
				))
		}
	}
}

func newBenchCache(
	ctor cacheCtor, capacity int,
	dataSize int64, sequence []int,
) func(b *testing.B) {
	return func(b *testing.B) {
		cache := ctor(capacity, b)
		warmUp(cache, sequence)
		b.ReportAllocs()
		b.SetBytes(dataSize)
		b.ResetTimer()
		var (
			hits, misses int64
			seqMask      = len(sequence) - 1
		)
		for i := 0; b.Loop(); i++ {
			key := sequence[i&seqMask]
			if _, ok := cache.Get(key); ok {
				hits++
			} else {
				misses++
				cache.Set(key, key)
			}
		}
		b.StopTimer()
		var (
			total    = float64(hits + misses)
			hitRate  = float64(hits) / total * 100.0
			missRate = float64(misses) / total * 100.0
		)
		b.ReportMetric(hitRate, "hit_rate_pct")
		b.ReportMetric(missRate, "miss_rate_pct")
	}
}

func makeSequential(universe, seqLen int) []int {
	seq := make([]int, nextPow2(seqLen))
	for i := range seq {
		seq[i] = i % universe
	}
	return seq
}

func makeLooping(capacity, universe, seqLen int, hotRatio float64) []int {
	var (
		seq      = make([]int, nextPow2(seqLen))
		rng      = newReproducibleRNG()
		hotSize  = max(1, capacity)
		coldSize = max(1, universe-hotSize)
	)
	for i := range seq {
		if rng.Float64() < hotRatio {
			seq[i] = rng.Intn(hotSize)
		} else {
			seq[i] = hotSize + rng.Intn(coldSize)
		}
	}
	return seq
}

func makeZipf(universe, seqLen int, skew, bias float64) []int {
	var (
		seq  = make([]int, nextPow2(seqLen))
		rng  = newReproducibleRNG()
		imax = uint64(max(universe, 2) - 1)
		zipf = rand.NewZipf(rng, skew, bias, imax)
	)
	for i := range seq {
		seq[i] = int(zipf.Uint64())
	}
	return seq
}

func apiOverhead(b *testing.B) {
	type (
		Key   = int
		Value = int
	)
	const (
		capacity   = 1024
		upperBound = capacity
		keyCount   = 1 << 16 // Power-of-two for mask; much larger than capacity to mix hits/misses.
		keyEnd     = keyCount - 1
		keySize    = unsafe.Sizeof(Key(0))
		valueSize  = unsafe.Sizeof(Value(0))
		dataSize   = keySize + valueSize
	)
	var (
		cache = newCache[int, int](b, capacity)
		rng   = newReproducibleRNG()
		keys  = makeRandomSequence(rng, upperBound, keyCount)
	)
	addIncrementingInts(cache, capacity)
	b.ReportAllocs()
	b.SetBytes(int64(dataSize))
	for i := 0; b.Loop(); i++ {
		key := keys[i&keyEnd]
		if value, ok := cache.Get(key); ok {
			_ = value
		}
	}
}

func makeRandomSequence(rng *rand.Rand, upperBound, capacity int) []int {
	keys := make([]int, capacity)
	for i := range keys {
		keys[i] = rng.Intn(upperBound)
	}
	return keys
}

func warmUp(c benchCache[int, int], seq []int) {
	for _, k := range seq {
		if _, ok := c.Get(k); !ok {
			c.Set(k, k)
		}
	}
}

func nextPow2(x int) int {
	if x <= 1 {
		return 1
	}
	return 1 << bits.Len(uint(x)-1)
}

func newReproducibleRNG() *rand.Rand {
	return rand.New(rand.NewSource(rngSeed))
}
