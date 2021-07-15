package load

import (
	"bytes"
	"encoding/hex"
	"math/rand"

	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
)

type TreeGenerator struct {
	TreeConfig
	seed int

	trees  []*tree.Tree
	symBuf []byte
	b      *bytes.Buffer
	i      int
}

type TreeConfig struct {
	MaxSymLen int `yaml:"maxSymLen"`
	MaxDepth  int `yaml:"maxDepth"`
	Width     int `yaml:"width"`
}

var rootNode = []byte("root")

const (
	minStackDepth = 2
	minSymLength  = 3
)

func NewTreeGenerator(seed, trees int, c TreeConfig) *TreeGenerator {
	g := TreeGenerator{
		TreeConfig: c,
		seed:       seed,
		symBuf:     make([]byte, c.MaxSymLen),
		trees:      make([]*tree.Tree, trees),
	}
	g.b = bytes.NewBuffer(make([]byte, 128))
	for i := 0; i < trees; i++ {
		seed++
		g.trees[i] = g.generateTree(newRand(seed))
	}
	return &g
}

func (g *TreeGenerator) Next() *tree.Tree {
	g.i++
	return g.trees[g.i%len(g.trees)]
}

func (g *TreeGenerator) generateTree(r *rand.Rand) *tree.Tree {
	t := tree.New()
	for w := 0; w < g.Width; w++ {
		t.Insert(g.generateStack(r), uint64(r.Intn(100)), true)
	}
	return t
}

func (g *TreeGenerator) generateStack(r *rand.Rand) []byte {
	g.b.Reset()
	g.b.Write(rootNode)
	e := hex.NewEncoder(g.b)
	d := randInt(r, minStackDepth, g.MaxDepth)
	for i := 0; i < d; i++ {
		l := randInt(r, minSymLength, g.MaxSymLen)
		r.Read(g.symBuf[:l])
		g.b.WriteString(";")
		_, _ = e.Write(g.symBuf[:l])
	}
	s := make([]byte, g.b.Len())
	copy(s, g.b.Bytes())
	return s
}
