package storage

import (
	"encoding/binary"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/structs/sortedmap"
	"github.com/twmb/murmur3"
)

type Key struct {
	labels map[string]string
}

var nameParser = regexp.MustCompile("^(.+)\\{(.+)\\}.*$")

const seed = 6231912

type ParserState int

const (
	nameParserState ParserState = iota
	tagKeyParserState
	tagValueParserState
	doneParserState
)

func NewKey(labels map[string]string) *Key { return &Key{labels: labels} }

// TODO: should rewrite this at some point to not rely on regular expressions & splits
func ParseKey(name string) (*Key, error) {
	k := &Key{
		labels: make(map[string]string),
	}

	p := parser{
		parserState: nameParserState,
		key:         "",
		value:       "",
	}

	for _, r := range name + "{" {
		switch p.parserState {
		case nameParserState:
			p.nameParserCase(r, k)
		case tagKeyParserState:
			p.tagKeyParserCase(r)
		case tagValueParserState:
			p.tagValueParserCase(r, k)
		}
	}
	return k, nil
}

type parser struct {
	parserState ParserState
	key         string
	value       string
}

// ParseKey's nameParserState switch case
func (p *parser) nameParserCase(r int32, k *Key) {
	switch r {
	case '{':
		p.parserState = tagKeyParserState
		k.labels["__name__"] = strings.TrimSpace(p.value)
	default:
		p.value += string(r)
	}
}

// ParseKey's tagKeyParserState switch case
func (p *parser) tagKeyParserCase(r int32) {
	switch r {
	case '}':
		p.parserState = doneParserState
	case '=':
		p.parserState = tagValueParserState
		p.value = ""
	default:
		p.key += string(r)
	}
}

// ParseKey's tagValueParserState switch case
func (p *parser) tagValueParserCase(r int32, k *Key) {
	switch r {
	case ',', '}':
		p.parserState = tagKeyParserState
		k.labels[strings.TrimSpace(p.key)] = strings.TrimSpace(p.value)
		p.key = ""
	default:
		p.value += string(r)
	}
}

func (k *Key) SegmentKey() string {
	return k.Normalized()
}

func segmentKeyToTreeKey(k string, depth int, t time.Time) string {
	return k + ":" + strconv.Itoa(depth) + ":" + strconv.Itoa(int(t.Unix()))
}

func (k *Key) TreeKey(depth int, t time.Time) string {
	return segmentKeyToTreeKey(k.Normalized(), depth, t)
}

func (k *Key) DictKey() string {
	return k.labels["__name__"]
}

// fromTreeToDictKey returns app name from tree key k: given tree key
// "foo{}:0:-62135596790", the call returns "foo".
//
// Before tags support, segment key form (i.e. app name + tags: foo{key=value})
// has been used to reference a dictionary (trie).
func fromTreeToDictKey(k string) string {
	return k[0:strings.IndexAny(k, "{")]
}

func fromTreeToMainKey(k string) string {
	i := strings.LastIndex(k, ":")
	i = strings.LastIndex(k[:i-1], ":")
	return k[:i]
}

func (k *Key) Normalized() string {
	var sb strings.Builder

	sortedMap := sortedmap.New()
	for k, v := range k.labels {
		if k == "__name__" {
			sb.WriteString(v)
		} else {
			sortedMap.Put(k, v)
		}
	}

	sb.WriteString("{")
	for i, k := range sortedMap.Keys() {
		v := sortedMap.Get(k).(string)
		if i != 0 {
			sb.WriteString(",")
		}
		sb.WriteString(k)
		sb.WriteString("=")
		sb.WriteString(v)
	}
	sb.WriteString("}")

	return sb.String()
}

func (k *Key) Hashed() []byte {
	u1, u2 := murmur3.SeedSum128(seed, seed, []byte(k.Normalized()))

	b := make([]byte, 16)
	binary.LittleEndian.PutUint64(b[:8], u1)
	binary.LittleEndian.PutUint64(b[8:16], u2)
	return b
}

func (k *Key) AppName() string {
	return k.labels["__name__"]
}
