package clockpro_test

import (
	"fmt"

	clockpro "github.com/djdv/go-clockpro"
)

func ExampleCache() {
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
