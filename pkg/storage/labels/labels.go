package labels

import (
	"bytes"

	"github.com/pyroscope-io/pyroscope/pkg/storage/kv"
)

type Labels struct {
	db kv.Storage
}

func New(db kv.Storage) *Labels {
	ll := &Labels{
		db: db,
	}
	return ll
}

func (ll *Labels) Put(key, val string) error {
	kk := "l:" + key
	kv := "v:" + key + ":" + val
	if err := ll.db.Set([]byte(kk), []byte{}); err != nil {
		return err
	}
	if err := ll.db.Set([]byte(kv), []byte{}); err != nil {
		return err
	}
	return nil
}

func (ll *Labels) GetKeys(cb func(string) bool) error {
	prefix := "l:"
	return ll.db.IterateKeys([]byte(prefix), func(key []byte) bool {
		return cb(string(key[2:]))
	})
}

func (ll *Labels) GetValues(k string, cb func(v string) bool) error {
	prefix := "v:" + k + ":"
	return ll.db.IterateKeys([]byte(prefix), func(key []byte) bool {
		idx := bytes.LastIndex(key, []byte{':'}) + 1
		return cb(string(key[idx:]))
	})
}
