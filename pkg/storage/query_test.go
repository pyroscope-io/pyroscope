package storage

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/pyroql"
	"github.com/pyroscope-io/pyroscope/pkg/storage/dimension"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"github.com/pyroscope-io/pyroscope/pkg/testing"
)

var _ = Describe("Querying", func() {
	testing.WithConfig(func(cfg **config.Config) {
		JustBeforeEach(func() {
			var err error
			s, err = New(&(*cfg).Server)
			Expect(err).ToNot(HaveOccurred())
			keys := []string{
				"app.name{foo=bar,baz=qux}",
				"app.name{foo=bar,baz=xxx}",
				"app.name{waldo=fred,baz=xxx}",
			}
			for _, k := range keys {
				t := tree.New()
				t.Insert([]byte("a;b"), uint64(1))
				t.Insert([]byte("a;c"), uint64(2))
				st := testing.SimpleTime(10)
				et := testing.SimpleTime(19)
				key, err := ParseKey(k)
				Expect(err).ToNot(HaveOccurred())
				err = s.Put(&PutInput{
					StartTime:  st,
					EndTime:    et,
					Key:        key,
					Val:        t,
					SpyName:    "testspy",
					SampleRate: 100,
				})
				Expect(err).ToNot(HaveOccurred())
			}
		})

		Context("basic queries", func() {
			It("get returns result with query", func() {
				qry, err := pyroql.ParseQuery(`app.name{foo="bar"}`)
				Expect(err).ToNot(HaveOccurred())
				output, err := s.Get(&GetInput{
					StartTime: time.Time{},
					EndTime:   maxTime,
					Query:     qry,
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(output).ToNot(BeNil())
				Expect(output.Tree).ToNot(BeNil())
				Expect(output.Tree.Samples()).To(Equal(uint64(6)))
			})

			It("get returns a particular tree for a fully qualified key", func() {
				k, err := ParseKey(`app.name{foo=bar,baz=qux}`)
				Expect(err).ToNot(HaveOccurred())
				output, err := s.Get(&GetInput{
					StartTime: time.Time{},
					EndTime:   maxTime,
					Key:       k,
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(output).ToNot(BeNil())
				Expect(output.Tree).ToNot(BeNil())
				Expect(output.Tree.Samples()).To(Equal(uint64(3)))
			})

			It("get returns all results for a key containing only app name", func() {
				k, err := ParseKey(`app.name`)
				Expect(err).ToNot(HaveOccurred())
				output, err := s.Get(&GetInput{
					StartTime: time.Time{},
					EndTime:   maxTime,
					Key:       k,
				})
				Expect(err).ToNot(HaveOccurred())
				Expect(output).ToNot(BeNil())
				Expect(output.Tree).ToNot(BeNil())
				Expect(output.Tree.Samples()).To(Equal(uint64(9)))
			})

			It("query returns expected results", func() {
				type testCase struct {
					query       string
					segmentKeys []dimension.Key
				}

				testCases := []testCase{
					{`app.name`, []dimension.Key{
						dimension.Key("app.name{baz=qux,foo=bar}"),
						dimension.Key("app.name{baz=xxx,foo=bar}"),
						dimension.Key("app.name{baz=xxx,waldo=fred}"),
					}},
					{`app.name{foo="bar"}`, []dimension.Key{
						dimension.Key("app.name{baz=qux,foo=bar}"),
						dimension.Key("app.name{baz=xxx,foo=bar}"),
					}},
					{`app.name{foo=~"^b.*"}`, []dimension.Key{
						dimension.Key("app.name{baz=qux,foo=bar}"),
						dimension.Key("app.name{baz=xxx,foo=bar}"),
					}},
					{`app.name{baz=~"xxx|qux"}`, []dimension.Key{
						dimension.Key("app.name{baz=qux,foo=bar}"),
						dimension.Key("app.name{baz=xxx,foo=bar}"),
						dimension.Key("app.name{baz=xxx,waldo=fred}"),
					}},
					{`app.name{baz!="xxx"}`, []dimension.Key{
						dimension.Key("app.name{baz=qux,foo=bar}"),
					}},
					{`app.name{foo!="bar"}`, []dimension.Key{
						dimension.Key("app.name{baz=xxx,waldo=fred}"),
					}},
					{`app.name{foo!~".*"}`, []dimension.Key{
						dimension.Key("app.name{baz=xxx,waldo=fred}"),
					}},
					{`app.name{baz!~"^x.*"}`, []dimension.Key{
						dimension.Key("app.name{baz=qux,foo=bar}"),
					}},
					{`app.name{foo="bar",baz!~"^x.*"}`, []dimension.Key{
						dimension.Key("app.name{baz=qux,foo=bar}"),
					}},
					{`app.name{foo=~"b.*",foo!~".*r"}`, nil},
					{`not.an.app{}`, nil},
				}

				for _, tc := range testCases {
					qry, err := pyroql.ParseQuery(tc.query)
					Expect(err).ToNot(HaveOccurred())
					r := s.exec(context.TODO(), qry)
					if tc.segmentKeys == nil {
						Expect(r).To(BeEmpty())
						continue
					}
					Expect(r).To(Equal(tc.segmentKeys))
				}
			})
		})
	})
})
