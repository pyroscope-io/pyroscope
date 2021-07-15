package load

import "math/rand"

type TagsGenerator struct {
	seed    int
	appName string

	tags []testTag
	ixs  []int
}

type testTag struct {
	name   string
	values []string
}

func NewTagGenerator(seed int, appName string) *TagsGenerator {
	return &TagsGenerator{seed: seed, appName: appName}
}

func (g *TagsGenerator) Next() map[string]string {
	k := map[string]string{"__name__": g.appName}
	for i := 0; i < len(g.tags); i++ {
		t := g.tags[i]
		k[t.name] = t.values[g.ixs[i]%len(t.values)]
		g.ixs[i]++
	}
	return k
}

func (g *TagsGenerator) Add(name string, card, min, max int) *TagsGenerator {
	g.seed++
	r := newRand(g.seed)
	g.ixs = append(g.ixs, 0)
	g.tags = append(g.tags, testTag{
		name:   name,
		values: g.values(r, card, min, max),
	})
	return g
}

func (g *TagsGenerator) values(r *rand.Rand, count, min, max int) []string {
	values := make([]string, count)
	for i := 0; i < count; i++ {
		values[i] = randString(r, min, max)
	}
	return values
}
