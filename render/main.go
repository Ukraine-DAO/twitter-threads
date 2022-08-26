package main

import (
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"log"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/rusni-pyzda/twitter-threads/common"
	"github.com/rusni-pyzda/twitter-threads/state"
	"github.com/rusni-pyzda/twitter-threads/twitter"
)

var (
	configPath = flag.String("config", "", "Path to the config file")
	statePath  = flag.String("state", "", "Path to the state file")
	outputDir  = flag.String("output_dir", "", "Path to the output dir")
)

//go:embed thread.tmpl
var threadTmplText string

var threadTmpl = template.Must(template.New("thread").Parse(threadTmplText))

type Thread struct {
	Blocks []Block
}

type Block struct {
	Paragraph   string
	Images      []string
	QuotedTweet *twitter.Tweet
}

func parseThread(thread common.Thread, state state.ThreadState) Thread {
	chain := state.TweetChain()
	r := Thread{}
	for _, t := range chain {
		r.Blocks = append(r.Blocks, Block{Paragraph: t.Text})
	}
	return r
}

func run(cfg *common.Config, state *state.State) error {
	for name, thread := range cfg.ThreadPages() {
		t := parseThread(thread, state.Threads[thread.ThreadID])
		fname := fmt.Sprintf("%s.md", filepath.Join(*outputDir, name))
		if err := os.MkdirAll(filepath.Dir(fname), 0755); err != nil {
			return fmt.Errorf("creating directories for %q: %w", fname, err)
		}
		f, err := os.Create(fname)
		if err != nil {
			return fmt.Errorf("failed to open %q: %w", fname, err)
		}
		if err := threadTmpl.Execute(f, t); err != nil {
			return fmt.Errorf("executing template: %w", err)
		}
		f.Close()
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
		log.Fatalf("%s", err)
	}
}
