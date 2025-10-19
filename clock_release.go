//go:build !clockpro_debug

package clockpro

const debugging = false

func assert(bool, string) { /* NOOP */ }
