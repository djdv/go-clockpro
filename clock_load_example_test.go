package clockpro_test

import (
	"fmt"

	clockpro "github.com/djdv/go-clockpro"
)

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

func ExampleCache_Load() {
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
