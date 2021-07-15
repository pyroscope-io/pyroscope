package load

import (
	"encoding/hex"
	"math/rand"
)

func newRand(seed int) *rand.Rand {
	return rand.New(rand.NewSource(int64(seed)))
}

func randInt(r *rand.Rand, min, max int) int {
	if max == min {
		return max
	}
	return r.Intn(max-min) + min
}

func randString(r *rand.Rand, min, max int) string {
	l := randInt(r, min, max)
	buf := make([]byte, l)
	r.Read(buf)
	return hex.EncodeToString(buf)
}
