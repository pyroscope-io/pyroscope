package server

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"runtime"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/testing"
)

var _ = Describe("server", func() {
	testing.WithConfig(func(cfg **config.Config) {
		Describe("/build", func() {
			It("works properly", func() {
				done := make(chan interface{})
				go func() {
					defer GinkgoRecover()

					(*cfg).Server.APIBindAddr = ":10044"
					s, err := storage.New(&(*cfg).Server)
					Expect(err).ToNot(HaveOccurred())
					c, _ := New(&(*cfg).Server, s, logrus.New())
					httpServer := httptest.NewServer(c.mux())
					defer httpServer.Close()

					res, err := http.Get(httpServer.URL + "/build")
					Expect(err).ToNot(HaveOccurred())
					Expect(res.StatusCode).To(Equal(200))

					b, err := ioutil.ReadAll(res.Body)
					Expect(err).ToNot(HaveOccurred())

					actual := make(map[string]interface{})
					err = json.Unmarshal(b, &actual)

					Expect(err).ToNot(HaveOccurred())
					Expect(actual["goos"]).To(Equal(runtime.GOOS))

					close(done)
				}()
				Eventually(done, 2).Should(BeClosed())
			})
		})
	})
})
