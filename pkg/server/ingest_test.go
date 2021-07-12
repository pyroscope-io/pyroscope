package server

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/testing"
)

var _ = Describe("server", func() {
	testing.WithConfig(func(cfg **config.Config) {
		BeforeEach(func() {
			(*cfg).Server.APIBindAddr = ":10043"
		})

		Describe("/ingest", func() {
			var buf *bytes.Buffer
			var format string
			var contentType string

			// this is an example of Shared Example pattern
			//   see https://onsi.github.io/ginkgo/#shared-example-patterns
			ItCorrectlyParsesIncomingData := func() {
				It("correctly parses incoming data", func() {
					done := make(chan interface{})
					go func() {
						defer GinkgoRecover()

						s, err := storage.New(&(*cfg).Server)
						Expect(err).ToNot(HaveOccurred())
						c, _ := New(&(*cfg).Server, s, logrus.New())
						httpServer := httptest.NewServer(c.mux())
						defer s.Close()

						name := "test.app{}"

						st := testing.ParseTime("2020-01-01-01:01:00")
						et := testing.ParseTime("2020-01-01-01:01:10")

						u, _ := url.Parse(httpServer.URL + "/ingest")
						q := u.Query()
						q.Add("name", name)
						q.Add("from", strconv.Itoa(int(st.Unix())))
						q.Add("until", strconv.Itoa(int(et.Unix())))
						if format != "" {
							q.Add("format", format)
						}
						u.RawQuery = q.Encode()

						fmt.Println(u.String())

						req, err := http.NewRequest("POST", u.String(), buf)
						Expect(err).ToNot(HaveOccurred())
						if contentType == "" {
							contentType = "text/plain"
						}
						req.Header.Set("Content-Type", contentType)

						res, err := http.DefaultClient.Do(req)
						Expect(err).ToNot(HaveOccurred())
						Expect(res.StatusCode).To(Equal(200))

						sk, _ := storage.ParseKey(name)
						gOut, err := s.Get(&storage.GetInput{
							StartTime: st,
							EndTime:   et,
							Key:       sk,
						})
						Expect(gOut.Tree).ToNot(BeNil())
						Expect(gOut.Tree.String()).To(Equal("\"foo;bar\" 2\n\"foo;baz\" 3\n"))

						close(done)
					}()
					Eventually(done, 2).Should(BeClosed())
				})
			}

			Context("default format", func() {
				BeforeEach(func() {
					buf = bytes.NewBuffer([]byte("foo;bar 2\nfoo;baz 3\n"))
					format = ""
					contentType = ""
				})

				ItCorrectlyParsesIncomingData()
			})

			Context("lines format", func() {
				BeforeEach(func() {
					buf = bytes.NewBuffer([]byte("foo;bar\nfoo;bar\nfoo;baz\nfoo;baz\nfoo;baz\n"))
					format = "lines"
					contentType = ""
				})

				ItCorrectlyParsesIncomingData()
			})

			Context("trie format", func() {
				BeforeEach(func() {
					buf = bytes.NewBuffer([]byte("\x00\x00\x01\x06foo;ba\x00\x02\x01r\x02\x00\x01z\x03\x00"))
					format = "trie"
					contentType = ""
				})

				ItCorrectlyParsesIncomingData()
			})

			Context("tree format", func() {
				BeforeEach(func() {
					buf = bytes.NewBuffer([]byte("\x00\x00\x01\x03foo\x00\x02\x03bar\x02\x00\x03baz\x03\x00"))
					format = "tree"
					contentType = ""
				})

				ItCorrectlyParsesIncomingData()
			})

			Context("trie format", func() {
				BeforeEach(func() {
					buf = bytes.NewBuffer([]byte("\x00\x00\x01\x06foo;ba\x00\x02\x01r\x02\x00\x01z\x03\x00"))
					format = ""
					contentType = "binary/octet-stream+trie"
				})

				ItCorrectlyParsesIncomingData()
			})

			Context("tree format", func() {
				BeforeEach(func() {
					buf = bytes.NewBuffer([]byte("\x00\x00\x01\x03foo\x00\x02\x03bar\x02\x00\x03baz\x03\x00"))
					format = ""
					contentType = "binary/octet-stream+tree"
				})

				ItCorrectlyParsesIncomingData()
			})
		})
	})
})
