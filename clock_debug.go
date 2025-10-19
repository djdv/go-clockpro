//go:build clockpro_debug

package clockpro

const debugging = true

func assert(cond bool, message string) {
	if !cond {
		panic(message)
	}
}
