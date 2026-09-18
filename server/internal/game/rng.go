package game

import "math/rand/v2"

// RNG is the only source of randomness the engine uses (dice and deck
// shuffles). A seeded RNG makes a whole game reproducible.
type RNG interface {
	IntN(n int) int
}

// NewSeededRNG returns a deterministic RNG.
func NewSeededRNG(seed uint64) RNG {
	return rand.New(rand.NewPCG(seed, seed^0x9E3779B97F4A7C15))
}

func rollDice(r RNG) [2]int {
	return [2]int{r.IntN(6) + 1, r.IntN(6) + 1}
}

func shuffle[T any](r RNG, xs []T) {
	for i := len(xs) - 1; i > 0; i-- {
		j := r.IntN(i + 1)
		xs[i], xs[j] = xs[j], xs[i]
	}
}
