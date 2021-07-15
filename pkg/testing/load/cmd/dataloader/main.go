package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"time"

	"gopkg.in/yaml.v2"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/testing/load"
)

type Config struct {
	load.StorageWriteSuiteConfig `yaml:",inline"`

	Apps    map[string]load.AppConfig
	Storage struct{ Path string }
}

func loadConfig(path string) (Config, error) {
	var c Config
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return c, err
	}
	if err = yaml.Unmarshal(b, &c); err != nil {
		return c, err
	}
	return c, nil
}

func openStorage(path string) (*storage.Storage, error) {
	if path == "" {
		return nil, fmt.Errorf("storage path required")
	}
	if err := os.MkdirAll(path, 0755); err != nil {
		return nil, err
	}
	return storage.New(&config.Server{
		StoragePath:           path,
		CacheEvictThreshold:   0.02,
		CacheEvictVolume:      0.10,
		MaxNodesSerialization: 2048,
		MaxNodesRender:        2048,
	})
}

func main() {
	var path string
	flag.StringVar(&path, "path", "config.yml", "config file path")
	flag.Parse()

	c, err := loadConfig(path)
	if err != nil {
		log.Fatal(err)
	}

	x, err := openStorage(c.Storage.Path)
	if err != nil {
		log.Fatal(err)
	}

	c.WriteFn = func(input *storage.PutInput) {
		if err = x.Put(input); err != nil {
			fmt.Println(err)
		}
	}

	s := load.NewStorageWriteSuite(c.StorageWriteSuiteConfig)
	for name, appConfig := range c.Apps {
		s.AddApp(name, appConfig)
	}

	start := time.Now()
	t := time.NewTicker(time.Second)
	defer t.Stop()
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			case <-t.C:
				stats := s.Stats()
				var (
					p float32
					d time.Duration
					e time.Duration
				)
				if stats.RemainingPeriod > 0 {
					p = float32(c.Period-stats.RemainingPeriod) * 100 / float32(c.Period)
					d = time.Since(start)
					e = time.Duration(100/p*float32(d)) - d
				} else {
					p = 100
				}
				fmt.Printf("Progress: %.2f%%, estimated remaining time: %v\n", p, e)
			}
		}
	}()

	s.Start()
	close(done)
	fmt.Println("Closing storage.")
	x.Close()
	fmt.Println("Done.")
}
