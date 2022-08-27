package main

import (
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
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
	Title  string
	Blocks []Block
}

type Block struct {
	Paragraph   string
	Images      []string
	QuotedTweet *twitter.Tweet
}

func parseThread(name string, thread common.Thread, state state.ThreadState) Thread {
	chain := state.TweetChain()
	r := Thread{
		Title: thread.Title,
	}
	if r.Title == "" {
		r.Title = cases.Title(language.English).String(strings.ReplaceAll(name, "_", " "))
	}
	add := func(b Block) { r.Blocks = append(r.Blocks, b) }
	for _, t := range chain {
		add(Block{Paragraph: t.Text})

		if len(t.Attachments.MediaKeys) > 0 {
			imgs := []string{}
			for _, k := range t.Attachments.MediaKeys {
				for _, m := range t.Includes.Media {
					if m.Key == k && m.Type == "photo" {
						imgs = append(imgs, m.URL)
						break
					}
				}
			}
			add(Block{Images: imgs})
		}

		for _, rt := range t.ReferencedTweets {
			if rt.Type != "quoted" {
				continue
			}
			for _, qt := range t.Includes.Tweets {
				if qt.ID != rt.ID {
					continue
				}
				copied := qt
				for _, u := range t.Includes.Users {
					if u.ID == copied.AuthorID {
						copied.Includes.Users = []twitter.TwitterUser{u}
					}
				}
				add(Block{QuotedTweet: &copied})
			}
		}
	}
	return r
}

func run(cfg *common.Config, state *state.State) error {
	for name, thread := range cfg.ThreadPages() {
		t := parseThread(path.Base(name), thread, state.Threads[thread.ThreadID])
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
