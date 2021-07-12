package tree

import (
	"math/rand"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func randStr() []byte {
	buf := make([]byte, 10)
	for i := 0; i < 10; i++ {
		buf[i] = byte(97) + byte(rand.Uint32()%10)
	}
	return buf
}

var _ = Describe("tree package", func() {
	Context("Insert", func() {
		tree := New()
		tree.Insert([]byte("a;b"), uint64(1))
		tree.Insert([]byte("a;c"), uint64(2))

		It("properly sets up a tree", func() {
			Expect(tree.root.ChildrenNodes).To(HaveLen(1))
			Expect(tree.root.ChildrenNodes[0].ChildrenNodes).To(HaveLen(2))
			Expect(tree.root.ChildrenNodes[0].Self).To(Equal(uint64(0)))
			Expect(tree.root.ChildrenNodes[0].Total).To(Equal(uint64(3)))
			Expect(tree.root.ChildrenNodes[0].ChildrenNodes[0].Self).To(Equal(uint64(1)))
			Expect(tree.root.ChildrenNodes[0].ChildrenNodes[1].Self).To(Equal(uint64(2)))
			Expect(tree.root.ChildrenNodes[0].ChildrenNodes[0].Total).To(Equal(uint64(1)))
			Expect(tree.root.ChildrenNodes[0].ChildrenNodes[1].Total).To(Equal(uint64(2)))
			Expect(tree.String()).To(Equal("\"a;b\" 1\n\"a;c\" 2\n"))
		})
	})

	Context("Merge", func() {
		Context("similar trees", func() {
			treeA := New()
			treeA.Insert([]byte("a;b"), uint64(1))
			treeA.Insert([]byte("a;c"), uint64(2))
			It("properly sets up tree A", func() {
				Expect(treeA.String()).To(Equal(treeStr(`"a;b" 1|"a;c" 2|`)))
			})

			treeB := New()
			treeB.Insert([]byte("a;b"), uint64(4))
			treeB.Insert([]byte("a;c"), uint64(8))
			It("properly sets up tree B", func() {
				Expect(treeB.String()).To(Equal(treeStr(`"a;b" 4|"a;c" 8|`)))
			})

			It("properly merges", func() {
				treeA.Merge(treeB)

				Expect(treeA.root.ChildrenNodes).To(HaveLen(1))
				Expect(treeA.root.ChildrenNodes[0].ChildrenNodes).To(HaveLen(2))
				Expect(treeA.root.ChildrenNodes[0].Self).To(Equal(uint64(0)))
				Expect(treeA.root.ChildrenNodes[0].Total).To(Equal(uint64(15)))
				Expect(treeA.root.ChildrenNodes[0].ChildrenNodes[0].Self).To(Equal(uint64(5)))
				Expect(treeA.root.ChildrenNodes[0].ChildrenNodes[1].Self).To(Equal(uint64(10)))
				Expect(treeA.root.ChildrenNodes[0].ChildrenNodes[0].Total).To(Equal(uint64(5)))
				Expect(treeA.root.ChildrenNodes[0].ChildrenNodes[1].Total).To(Equal(uint64(10)))
				Expect(treeA.String()).To(Equal(treeStr(`"a;b" 5|"a;c" 10|`)))
			})
		})

		Context("tree with an extra node", func() {
			treeA := New()
			treeA.Insert([]byte("a;b"), uint64(1))
			treeA.Insert([]byte("a;c"), uint64(2))
			treeA.Insert([]byte("a;e"), uint64(3))
			It("properly sets up tree A", func() {
				Expect(treeA.String()).To(Equal(treeStr(`"a;b" 1|"a;c" 2|"a;e" 3|`)))
			})

			treeB := New()
			treeB.Insert([]byte("a;b"), uint64(4))
			treeB.Insert([]byte("a;d"), uint64(8))
			treeB.Insert([]byte("a;e"), uint64(12))
			It("properly sets up tree B", func() {
				Expect(treeB.String()).To(Equal(treeStr(`"a;b" 4|"a;d" 8|"a;e" 12|`)))
			})

			It("properly merges", func() {
				treeA.Merge(treeB)

				Expect(treeA.root.ChildrenNodes).To(HaveLen(1))
				Expect(treeA.root.ChildrenNodes[0].ChildrenNodes).To(HaveLen(4))
				Expect(treeA.root.ChildrenNodes[0].Self).To(Equal(uint64(0)))
				Expect(treeA.root.ChildrenNodes[0].Total).To(Equal(uint64(30)))
				Expect(treeA.root.ChildrenNodes[0].ChildrenNodes[0].Self).To(Equal(uint64(5)))
				Expect(treeA.root.ChildrenNodes[0].ChildrenNodes[1].Self).To(Equal(uint64(2)))
				Expect(treeA.root.ChildrenNodes[0].ChildrenNodes[2].Self).To(Equal(uint64(8)))
				Expect(treeA.root.ChildrenNodes[0].ChildrenNodes[3].Self).To(Equal(uint64(15)))
				Expect(treeA.root.ChildrenNodes[0].ChildrenNodes[0].Total).To(Equal(uint64(5)))
				Expect(treeA.root.ChildrenNodes[0].ChildrenNodes[1].Total).To(Equal(uint64(2)))
				Expect(treeA.root.ChildrenNodes[0].ChildrenNodes[2].Total).To(Equal(uint64(8)))
				Expect(treeA.root.ChildrenNodes[0].ChildrenNodes[3].Total).To(Equal(uint64(15)))
				Expect(treeA.String()).To(Equal(treeStr(`"a;b" 5|"a;c" 2|"a;d" 8|"a;e" 15|`)))
			})
		})
	})

	Context("Diff", func() {
		Context("similar trees", func() {
			treeA := New()
			treeA.Insert([]byte("a;b"), uint64(1))
			treeA.Insert([]byte("a;c"), uint64(2))
			It("properly sets up tree A", func() {
				Expect(treeA.String()).To(Equal(treeStr(`"a;b" 1|"a;c" 2|`)))
			})

			treeB := New()
			treeB.Insert([]byte("a;b"), uint64(4))
			treeB.Insert([]byte("a;c"), uint64(8))
			It("properly sets up tree B", func() {
				Expect(treeB.String()).To(Equal(treeStr(`"a;b" 4|"a;c" 8|`)))
			})

			It("properly diffs", func() {
				sumTree, diffTree := treeB.Diff(treeA)

				// treeA, treeB do not change
				Expect(treeA.String()).To(Equal(treeStr(`"a;b" 1|"a;c" 2|`)))
				Expect(treeB.String()).To(Equal(treeStr(`"a;b" 4|"a;c" 8|`)))

				// verify sumTree
				Expect(sumTree.root.ChildrenNodes).To(HaveLen(1))
				Expect(sumTree.root.ChildrenNodes[0].ChildrenNodes).To(HaveLen(2))
				Expect(sumTree.root.ChildrenNodes[0].Self).To(Equal(uint64(0)))
				Expect(sumTree.root.ChildrenNodes[0].Total).To(Equal(uint64(15)))
				Expect(sumTree.root.ChildrenNodes[0].ChildrenNodes[0].Self).To(Equal(uint64(5)))
				Expect(sumTree.root.ChildrenNodes[0].ChildrenNodes[1].Self).To(Equal(uint64(10)))
				Expect(sumTree.root.ChildrenNodes[0].ChildrenNodes[0].Total).To(Equal(uint64(5)))
				Expect(sumTree.root.ChildrenNodes[0].ChildrenNodes[1].Total).To(Equal(uint64(10)))
				Expect(sumTree.String()).To(Equal(treeStr(`"a;b" 5|"a;c" 10|`)))

				// verify diffTree
				Expect(diffTree.root.ChildrenNodes).To(HaveLen(1))
				Expect(diffTree.root.ChildrenNodes[0].ChildrenNodes).To(HaveLen(2))
				Expect(diffTree.root.ChildrenNodes[0].Self).To(Equal(uint64(0)))
				Expect(diffTree.root.ChildrenNodes[0].Total).To(Equal(uint64(9)))
				Expect(diffTree.root.ChildrenNodes[0].ChildrenNodes[0].Self).To(Equal(uint64(3)))
				Expect(diffTree.root.ChildrenNodes[0].ChildrenNodes[1].Self).To(Equal(uint64(6)))
				Expect(diffTree.root.ChildrenNodes[0].ChildrenNodes[0].Total).To(Equal(uint64(3)))
				Expect(diffTree.root.ChildrenNodes[0].ChildrenNodes[1].Total).To(Equal(uint64(6)))
				Expect(diffTree.String()).To(Equal(treeStr(`"a;b" 3|"a;c" 6|`)))
			})
		})

		Context("tree with an extra node", func() {
			treeA := New()
			treeA.Insert([]byte("a;b"), uint64(1))
			treeA.Insert([]byte("a;c"), uint64(2))
			treeA.Insert([]byte("a;e"), uint64(3))
			It("properly sets up tree A", func() {
				Expect(treeA.String()).To(Equal(treeStr(`"a;b" 1|"a;c" 2|"a;e" 3|`)))
			})

			treeB := New()
			treeB.Insert([]byte("a;b"), uint64(4))
			treeB.Insert([]byte("a;d"), uint64(8))
			treeB.Insert([]byte("a;e"), uint64(12))
			It("properly sets up tree B", func() {
				Expect(treeB.String()).To(Equal(treeStr(`"a;b" 4|"a;d" 8|"a;e" 12|`)))
			})

			It("properly diffs", func() {
				sumTree, diffTree := treeB.Diff(treeA)

				// treeA, treeB do not change
				Expect(treeA.String()).To(Equal(treeStr(`"a;b" 1|"a;c" 2|"a;e" 3|`)))
				Expect(treeB.String()).To(Equal(treeStr(`"a;b" 4|"a;d" 8|"a;e" 12|`)))

				// verify sumTree
				Expect(sumTree.root.ChildrenNodes).To(HaveLen(1))
				Expect(sumTree.root.ChildrenNodes[0].ChildrenNodes).To(HaveLen(4))
				Expect(sumTree.root.ChildrenNodes[0].Self).To(Equal(uint64(0)))
				Expect(sumTree.root.ChildrenNodes[0].Total).To(Equal(uint64(30)))
				Expect(sumTree.root.ChildrenNodes[0].ChildrenNodes[0].Self).To(Equal(uint64(5)))
				Expect(sumTree.root.ChildrenNodes[0].ChildrenNodes[1].Self).To(Equal(uint64(2)))
				Expect(sumTree.root.ChildrenNodes[0].ChildrenNodes[2].Self).To(Equal(uint64(8)))
				Expect(sumTree.root.ChildrenNodes[0].ChildrenNodes[3].Self).To(Equal(uint64(15)))
				Expect(sumTree.root.ChildrenNodes[0].ChildrenNodes[0].Total).To(Equal(uint64(5)))
				Expect(sumTree.root.ChildrenNodes[0].ChildrenNodes[1].Total).To(Equal(uint64(2)))
				Expect(sumTree.root.ChildrenNodes[0].ChildrenNodes[2].Total).To(Equal(uint64(8)))
				Expect(sumTree.root.ChildrenNodes[0].ChildrenNodes[3].Total).To(Equal(uint64(15)))
				Expect(sumTree.String()).To(Equal(treeStr(`"a;b" 5|"a;c" 2|"a;d" 8|"a;e" 15|`)))

				// verify diffTree
				negTwo := int64(-2) // 18446744073709551614
				Expect(diffTree.root.ChildrenNodes).To(HaveLen(1))
				Expect(diffTree.root.ChildrenNodes[0].ChildrenNodes).To(HaveLen(4))
				Expect(diffTree.root.ChildrenNodes[0].Self).To(Equal(uint64(0)))
				Expect(diffTree.root.ChildrenNodes[0].Total).To(Equal(uint64(18)))
				Expect(diffTree.root.ChildrenNodes[0].ChildrenNodes[0].Self).To(Equal(uint64(3)))
				Expect(diffTree.root.ChildrenNodes[0].ChildrenNodes[1].Self).To(Equal(uint64(negTwo)))
				Expect(diffTree.root.ChildrenNodes[0].ChildrenNodes[2].Self).To(Equal(uint64(8)))
				Expect(diffTree.root.ChildrenNodes[0].ChildrenNodes[3].Self).To(Equal(uint64(9)))
				Expect(diffTree.root.ChildrenNodes[0].ChildrenNodes[0].Total).To(Equal(uint64(3)))
				Expect(diffTree.root.ChildrenNodes[0].ChildrenNodes[1].Total).To(Equal(uint64(negTwo)))
				Expect(diffTree.root.ChildrenNodes[0].ChildrenNodes[2].Total).To(Equal(uint64(8)))
				Expect(diffTree.root.ChildrenNodes[0].ChildrenNodes[3].Total).To(Equal(uint64(9)))
				Expect(diffTree.String()).To(Equal(treeStr(`"a;b" 3|"a;c" 18446744073709551614|"a;d" 8|"a;e" 9|`)))
			})
		})
	})
})

func treeStr(s string) string {
	return strings.ReplaceAll(s, "|", "\n")
}
