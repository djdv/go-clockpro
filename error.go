package clockpro

import "fmt"

type constError string

// ErrInvalidCapacity may be returned from [New].
const ErrInvalidCapacity = constError("invalid capacity")

func (errStr constError) Error() string { return string(errStr) }

func minCapacityError(capacity int) error {
	return fmt.Errorf(
		"%w: must be >=%d but %d was requested",
		ErrInvalidCapacity, MinimumCapacity, capacity)
}
