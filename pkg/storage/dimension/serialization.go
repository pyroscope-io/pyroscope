package dimension

import (
	"bufio"
	"bytes"
	"io"

	"github.com/pyroscope-io/pyroscope/pkg/util/varint"
)

// serialization format version. it's not very useful right now, but it will be in the future
const currentVersion = 1

func (s *Dimension) Serialize(w io.Writer) error {
	varint.Write(w, currentVersion)

	for _, k := range s.Keys {
		varint.Write(w, uint64(len(k)))
		w.Write([]byte(k))
	}
	return nil
}

func Deserialize(r io.Reader) (*Dimension, error) {
	s := New()

	br := bufio.NewReader(r) // TODO if it's already a bytereader skip

	// reads serialization format version, see comment at the top
	_, err := varint.Read(br)
	if err != nil {
		return nil, err
	}

	for {
		keyLen, err := varint.Read(br)
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		keyBuf := make([]byte, keyLen) // TODO: there are better ways to do this?
		_, err = io.ReadAtLeast(br, keyBuf, int(keyLen))
		if err != nil {
			return nil, err
		}

		s.Keys = append(s.Keys, Key(keyBuf))
	}

	return s, nil
}

func (s *Dimension) Bytes() ([]byte, error) {
	b := bytes.Buffer{}
	if err := s.Serialize(&b); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func FromBytes(p []byte) (*Dimension, error) {
	return Deserialize(bytes.NewReader(p))
}
