package kv

type Storage interface {
	Get(key []byte) ([]byte, error)
	Set(key []byte, value []byte) error
	Del(key []byte) error
	IterateKeys(prefix []byte, fn func(key []byte) bool) error
	Close() error
}
