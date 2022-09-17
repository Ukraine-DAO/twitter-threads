package main

import (
	"encoding/json"
	"errors"
	"flag"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/Ukraine-DAO/twitter-threads/common"
	"github.com/Ukraine-DAO/twitter-threads/state"
	"github.com/Ukraine-DAO/twitter-threads/twitter"
)

var (
	configPath = flag.String("config", "", "Path to the config file")
	statePath  = flag.String("state", "", "Path to the state file")
	outputDir  = flag.String("output_dir", "", "Path to the output dir")
)

func fetch(fpath string, u string) error {
	log.Printf("GET %q -> %q", u, fpath)
	if err := os.MkdirAll(filepath.Dir(fpath), 0755); err != nil {
		return err
	}
	out, err := os.Create(fpath)
	if err != nil {
		return err
	}
	defer out.Close()
	resp, err := http.Get(u)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		os.Remove(fpath)
	}
	return nil
}

func run(cfg *common.Config, state *state.State) error {
	for tid, ts := range state.Threads {
		media := map[string]twitter.Media{}
		for _, t := range ts.Tweets {
			for _, m := range t.Includes.Media {
				media[m.Key] = m
			}
		}

		for _, m := range media {
			for fname, u := range m.FetchList() {
				fpath := filepath.Join(*outputDir, cfg.MediaDir, tid, fname)
				_, err := os.Stat(fpath)
				if !errors.Is(err, fs.ErrNotExist) {
					continue
				}
				if err := fetch(fpath, u); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func main() {
	flag.Parse()

	b, err := os.ReadFile(*configPath)
	if err != nil {
		log.Fatalf("Failed to read config file: %s", err)
	}

	cfg := &common.Config{}
	if err := yaml.Unmarshal(b, cfg); err != nil {
		log.Fatalf("Failed to unmarshal config: %s", err)
	}

	state := state.New()
	f, err := os.Open(*statePath)
	if err != nil {
		log.Fatalf("Failed to read state file: %s", err)
	}

	if err := json.NewDecoder(f).Decode(state); err != nil {
		log.Fatalf("Failed to unmarshal state: %s", err)
	}
	f.Close()

	if err := run(cfg, state); err != nil {
		log.Printf("%s", err)
	}
}
