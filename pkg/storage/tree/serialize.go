package tree

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"io"

	"github.com/pyroscope-io/pyroscope/pkg/storage/dict"
	"github.com/pyroscope-io/pyroscope/pkg/util/varint"
)

// serialization format version. it's not very useful right now, but it will be in the future
const currentVersion = 1

func (t *Tree) Serialize(d *dict.Dict, maxNodes int, w io.Writer) error {
	t.RLock()
	defer t.RUnlock()

	varint.Write(w, currentVersion)

	nodes := []*treeNode{t.root}
	minVal := t.minValue(maxNodes)
	j := 0

	for len(nodes) > 0 {
		j++
		tn := nodes[0]
		nodes = nodes[1:]

		labelLink := d.Put([]byte(tn.Name))
		_, err := varint.Write(w, uint64(len(labelLink)))
		if err != nil {
			return err
		}
		_, err = w.Write(labelLink)
		if err != nil {
			return err
		}

		val := tn.Self
		_, err = varint.Write(w, uint64(val))
		if err != nil {
			return err
		}
		cnl := uint64(0)
		if tn.Total > minVal {
			cnl = uint64(len(tn.ChildrenNodes))
			nodes = append(tn.ChildrenNodes, nodes...)
		}
		_, err = varint.Write(w, cnl)
		if err != nil {
			return err
		}
	}
	return nil
}

func (t *Tree) SerializeNoDict(maxNodes int, w io.Writer) error {
	t.RLock()
	defer t.RUnlock()

	nodes := []*treeNode{t.root}
	minVal := t.minValue(maxNodes)
	j := 0

	for len(nodes) > 0 {
		j++
		tn := nodes[0]
		nodes = nodes[1:]

		_, err := varint.Write(w, uint64(len(tn.Name)))
		if err != nil {
			return err
		}
		_, err = w.Write(tn.Name)
		if err != nil {
			return err
		}

		val := tn.Self
		_, err = varint.Write(w, uint64(val))
		if err != nil {
			return err
		}
		cnl := uint64(0)
		if tn.Total > minVal {
			cnl = uint64(len(tn.ChildrenNodes))
			nodes = append(tn.ChildrenNodes, nodes...)
		}
		_, err = varint.Write(w, cnl)
		if err != nil {
			return err
		}
	}
	return nil
}

type parentNode struct {
	node   *treeNode
	parent *parentNode
}

func Deserialize(d *dict.Dict, r io.Reader) (*Tree, error) {
	t := New()
	br := bufio.NewReader(r) // TODO if it's already a bytereader skip

	// reads serialization format version, see comment at the top
	_, err := varint.Read(br)
	if err != nil {
		return nil, err
	}

	parents := []*parentNode{{t.root, nil}}
	j := 0

	for len(parents) > 0 {
		j++
		parent := parents[0]
		parents = parents[1:]

		labelLen, err := varint.Read(br)
		// if err == io.EOF {
		// 	return t, nil
		// }
		labelLinkBuf := make([]byte, labelLen) // TODO: there are better ways to do this?
		_, err = io.ReadAtLeast(br, labelLinkBuf, int(labelLen))
		if err != nil {
			return nil, err
		}
		nameBuf, ok := d.Get(labelLinkBuf)
		if !ok {
			// these strings has to be at least slightly different, hence base64 Addon
			nameBuf = []byte("label not found " + base64.URLEncoding.EncodeToString(labelLinkBuf))
		}
		tn := parent.node.insert(nameBuf)

		tn.Self, err = varint.Read(br)
		tn.Total = tn.Self
		if err != nil {
			return nil, err
		}

		pn := parent
		for pn != nil {
			pn.node.Total += tn.Self
			pn = pn.parent
		}

		childrenLen, err := varint.Read(br)
		if err != nil {
			return nil, err
		}

		for i := uint64(0); i < childrenLen; i++ {
			parents = append([]*parentNode{{tn, parent}}, parents...)
		}
	}

	t.root = t.root.ChildrenNodes[0]

	return t, nil
}

func DeserializeNoDict(r io.Reader) (*Tree, error) {
	t := New()
	br := bufio.NewReader(r) // TODO if it's already a bytereader skip

	parents := []*parentNode{{t.root, nil}}
	j := 0

	for len(parents) > 0 {
		j++
		parent := parents[0]
		parents = parents[1:]

		nameLen, err := varint.Read(br)
		// if err == io.EOF {
		// 	return t, nil
		// }
		nameBuf := make([]byte, nameLen) // TODO: there are better ways to do this?
		_, err = io.ReadAtLeast(br, nameBuf, int(nameLen))
		if err != nil {
			return nil, err
		}
		tn := parent.node.insert(nameBuf)

		tn.Self, err = varint.Read(br)
		tn.Total = tn.Self
		if err != nil {
			return nil, err
		}

		pn := parent
		for pn != nil {
			pn.node.Total += tn.Self
			pn = pn.parent
		}

		childrenLen, err := varint.Read(br)
		if err != nil {
			return nil, err
		}

		for i := uint64(0); i < childrenLen; i++ {
			parents = append([]*parentNode{{tn, parent}}, parents...)
		}
	}

	t.root = t.root.ChildrenNodes[0]

	return t, nil
}

func (t *Tree) Bytes(d *dict.Dict, maxNodes int) ([]byte, error) {
	b := bytes.Buffer{}
	if err := t.Serialize(d, maxNodes, &b); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func FromBytes(d *dict.Dict, p []byte) (*Tree, error) {
	return Deserialize(d, bytes.NewReader(p))
}
