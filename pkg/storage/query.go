package storage

import (
	"context"

	"github.com/pyroscope-io/pyroscope/pkg/pyroql"
	"github.com/pyroscope-io/pyroscope/pkg/storage/dimension"
)

func (s *Storage) exec(_ context.Context, qry *pyroql.Query) []dimension.Key {
	app, found := s.lookupAppDimension(qry.AppName)
	if !found {
		return nil
	}

	r := []*dimension.Dimension{app}
	var n []*dimension.Dimension // Keys to be removed from the result.

	for _, m := range qry.Matchers {
		switch m.Op {
		case pyroql.EQL:
			if d, ok := s.lookupDimension(m); ok {
				r = append(r, d)
			}
		case pyroql.NEQ:
			if d, ok := s.lookupDimension(m); ok {
				n = append(n, d)
			}
		case pyroql.EQL_REGEX:
			if d, ok := s.lookupDimensionRegex(m); ok {
				r = append(r, d)
			}
		case pyroql.NEQ_REGEX:
			if d, ok := s.lookupDimensionRegex(m); ok {
				n = append(n, d)
			}
		}
	}

	i := dimension.Intersection(r...)
	if len(n) == 0 {
		return i
	}

	return dimension.AndNot(
		&dimension.Dimension{Keys: i},
		&dimension.Dimension{Keys: dimension.Union(n...)})
}

func (s *Storage) lookupAppDimension(app string) (*dimension.Dimension, bool) {
	return s.lookupDimensionKV("__name__", app)
}

func (s *Storage) lookupDimension(m *pyroql.TagMatcher) (*dimension.Dimension, bool) {
	return s.lookupDimensionKV(m.Key, m.Value)
}

func (s *Storage) lookupDimensionRegex(m *pyroql.TagMatcher) (*dimension.Dimension, bool) {
	d := dimension.New()
	s.labels.GetValues(m.Key, func(v string) bool {
		if m.R.MatchString(v) {
			if x, ok := s.lookupDimensionKV(m.Key, v); ok {
				d.Keys = append(d.Keys, x.Keys...)
			}
		}
		return true
	})
	if len(d.Keys) > 0 {
		return d, true
	}
	return nil, false
}

func (s *Storage) lookupDimensionKV(k, v string) (*dimension.Dimension, bool) {
	r, ok := s.dimensions.Lookup(k + ":" + v)
	if ok {
		return r.(*dimension.Dimension), true
	}
	return nil, false
}
