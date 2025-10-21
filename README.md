# clockpro

[![Go Reference](https://pkg.go.dev/badge/github.com/djdv/go-clockpro.svg)](https://pkg.go.dev/github.com/djdv/go-clockpro)

A cache implementation that utilizes the **CLOCK‑Pro+** cache replacement algorithm in Go.

Example:
```go
import (
	"fmt"

	clockpro "github.com/djdv/go-clockpro"
)

func GetSet() {
	const (
		capacity = 1024 // TODO(Anyone): Use contextual capacity.
		key      = "name"
		value    = 1
	)
	cache, err := clockpro.New[string, int](capacity)
	if err != nil {
		panic(err) // TODO(Anyone): Handle error.
	}
	cache.Set(key, value)
	if got, ok := cache.Get(key); ok {
		fmt.Printf("%s: %d\n", key, got)
	}
	// Output:
	// name: 1
}

func LoadValue() {
	const (
		capacity = 1024 // TODO(Anyone): Use contextual capacity.
		key      = "load"
		value    = 1
	)
	cache, err := clockpro.New[string, int](capacity)
	if err != nil {
		panic(err) // TODO(Anyone): Handle error.
	}
	got, err := cache.Load(key, makeValue)
	if err != nil {
		panic(err) // TODO(Anyone): Handle error.
	}
	fmt.Printf("%s: %d\n", key, got)
	if got, err = cache.Load(key, makeValue); err != nil {
		panic(err) // TODO(Anyone): Handle error.
	}
	fmt.Printf("cached: %d\n", got)
	// Output:
	// initialized value: 1
	// load: 1
	// cached: 1
}

func makeValue() (int, error) {
	const (
		someValue = 1
		initError = false
	)
	if initError {
		return 0, fmt.Errorf(
			"could not initialize...",
		)
	}
	fmt.Println("initialized value:", someValue)
	return someValue, nil
}
```

Papers:  
[2005 USENIX CLOCK-Pro](https://www.usenix.org/conference/2005-usenix-annual-technical-conference/clock-pro-effective-improvement-clock-replacement)  
[CLOCK-PRO+](https://dl.acm.org/doi/10.1145/3319647.3325838)


Contributing:  
Please see [CONTRIBUTING.md](CONTRIBUTING.md) for the terms that apply to all contributions.
