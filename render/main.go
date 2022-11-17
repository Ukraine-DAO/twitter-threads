package main

import (
	_ "embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html"
	html_template "html/template"
	"io/fs"
	"log"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	text_template "text/template"
	"unicode"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"gopkg.in/yaml.v3"

	"github.com/Ukraine-DAO/twitter-threads/common"
	"github.com/Ukraine-DAO/twitter-threads/state"
	"github.com/Ukraine-DAO/twitter-threads/twitter"
)

var (
	configPath   = flag.String("config", "", "Path to the config file")
	statePath    = flag.String("state", "", "Path to the state file")
	outputDir    = flag.String("output_dir", "", "Path to the output dir")
	mappingsPath = flag.String("mappings", "", "Path to the file storing path mappings for threads (used to track page moves)")
)

//go:embed thread.tmpl
var threadTmplText string

//go:embed quoted_tweet.html.tmpl
var quotedTweetTmplText string

//go:embed index.tmpl
var indexTmplText string

var (
	threadTmpl      = text_template.Must(text_template.New("thread").Parse(threadTmplText))
	quotedTweetTmpl = html_template.Must(html_template.New("quoted").Funcs(
		map[string]any{
			"lines": func(s string) []string { return strings.Split(s, "\n") },
		},
	).Parse(quotedTweetTmplText))
	indexTmpl = text_template.Must(text_template.New("index").Parse(indexTmplText))
)

type Thread struct {
	Title          string
	ConversationID string
	AuthorUsername string
	AuthorName     string
	Blocks         []Block
	OldNames       []string
}

type Block struct {
	Paragraph   string
	Media       []Media
	QuotedTweet string
}

type Media struct {
	DisplayURL string
	TargetURL  string
}

type PathMapping struct {
	Mappings map[string]PathMappingEntry `json:",omitempty"`
}

type PathMappingEntry struct {
	Name     string   `json:",omitempty"`
	OldNames []string `json:",omitempty"`
}

var (
	prefixThreadCounter = regexp.MustCompile("^[0-9]{1,2}([/\\\\]([xn]|[0-9]{1,2})?|\\))")
	suffixThreadCounter = regexp.MustCompile("[0-9]{1,2}[/\\\\]([xn🧵]|[0-9]{1,2})?$")
)

type replacement struct {
	start uint
	end   uint
	text  string
}

func applyReplacements(s string, rs []replacement) string {
	var r strings.Builder
	sort.Slice(rs, func(i, j int) bool {
		return rs[i].start < rs[j].start
	})
	ss := strings.Split(s, "")
	prev := uint(0)
	for _, repl := range rs {
		if repl.start < prev {
			// Either a duplicate or some bug
			continue
		}
		r.WriteString(strings.Join(ss[prev:repl.start], ""))
		r.WriteString(repl.text)
		prev = repl.end
	}
	r.WriteString(strings.Join(ss[prev:], ""))
	return r.String()
}

func tweetTextToMarkdown(t twitter.Tweet, cfg common.RenderConfig) string {
	quotedID := ""
	for _, rt := range t.ReferencedTweets {
		if rt.Type == "quoted" {
			quotedID = rt.ID
		}
	}
	txt := t.Text
	repls := []replacement{}
	for _, u := range t.Entities.URLs {
		if strings.HasPrefix(u.ExpandedURL, "https://twitter.com/") && strings.HasSuffix(u.ExpandedURL, "/"+quotedID) {
			// Link to quoted tweet, simply remove.
			repls = append(repls, replacement{u.Start, u.End, ""})
		} else if strings.HasPrefix(u.ExpandedURL, "https://twitter.com/") && strings.Contains(u.ExpandedURL, "/status/"+t.ID+"/photo/") {
			// Link to the attached picture.
			repls = append(repls, replacement{u.Start, u.End, ""})
		} else {
			linkText := u.DisplayURL
			if u.Title != "" {
				linkText = u.Title
				url, err := url.Parse(u.ExpandedURL)
				if err == nil {
					linkText = fmt.Sprintf("%s: %s", url.Host, linkText)
				}
			}
			repls = append(repls, replacement{u.Start, u.End, fmt.Sprintf("[%s](%s)", linkText, u.ExpandedURL)})
		}
	}
	for _, m := range t.Entities.Mentions {
		repls = append(repls, replacement{m.Start, m.End, fmt.Sprintf("[@%s](https://twitter.com/%s)", m.Username, url.PathEscape(m.Username))})
	}
	for _, h := range t.Entities.Hashtags {
		repls = append(repls, replacement{h.Start, h.End, fmt.Sprintf("[#%s](https://twitter.com/hashtag/%s)", h.Tag, url.PathEscape(h.Tag))})
	}

	txt = applyReplacements(txt, repls)
	txt = strings.TrimLeft(txt, " ")
	txt = strings.TrimRight(txt, " ")
	txt = prefixThreadCounter.ReplaceAllLiteralString(txt, "")
	txt = suffixThreadCounter.ReplaceAllLiteralString(txt, "")
	txt = strings.TrimLeft(txt, " ")
	txt = strings.TrimRight(txt, " ")
	return html.UnescapeString(txt)
}

func tryMergingParagraphs(th *Thread, cfg common.RenderConfig) {
	firstChar := func(s string) rune {
		for _, r := range s {
			return r
		}
		return 0
	}

	maybeMerge := func(a string, b string) (string, bool) {
		switch {
		case strings.HasSuffix(a, "...") && unicode.IsLower(firstChar(b)):
			return strings.TrimSuffix(a, "...") + " " + b, true
		case strings.HasSuffix(a, "...") && strings.HasPrefix(b, "..."):
			return strings.TrimSuffix(a, "...") + " " + strings.TrimPrefix(b, "..."), true
		case strings.HasSuffix(a, "...") && cfg.MergeUpperCaseAfterEllipsis && unicode.IsUpper(firstChar(b)):
			return strings.TrimSuffix(a, "...") + " " + string([]rune{unicode.ToLower(firstChar(b))}) + strings.TrimPrefix(b, string([]rune{firstChar(b)})), true
		case !strings.HasSuffix(a, ".") && cfg.MergeUnlessEndsWithPeriod:
			return a + " " + b, true
		}
		return "", false
	}

	r := []Block{}
	acc := []Block{}
	for _, b := range th.Blocks {
		if b.Paragraph == "" {
			r = append(r, acc...)
			r = append(r, b)
			acc = nil
			continue
		}
		if len(acc) == 0 {
			acc = []Block{b}
			continue
		}
		if merged, yes := maybeMerge(acc[len(acc)-1].Paragraph, b.Paragraph); yes {
			acc[len(acc)-1].Paragraph = merged
		} else {
			acc = append(acc, b)
		}
	}
	r = append(r, acc...)
	th.Blocks = r
}

func parseThread(name string, thread common.Thread, state state.ThreadState, mediaDir string) Thread {
	chain := state.TweetChain()
	r := Thread{
		Title:          thread.Title,
		ConversationID: state.Tweets[0].ConversationID,
	}
	if r.Title == "" {
		r.Title = cases.Title(language.English).String(strings.ReplaceAll(name, "_", " "))
	}
	for _, u := range state.Tweets[0].Includes.Users {
		if u.ID == state.Tweets[0].AuthorID {
			r.AuthorUsername = u.Username
			r.AuthorName = u.Name
		}
	}

	add := func(b Block) { r.Blocks = append(r.Blocks, b) }
	localOrRemoteURL := func(remote string, local string) string {
		fpath := filepath.Join(*outputDir, mediaDir, thread.ThreadID, local)
		_, err := os.Stat(fpath)
		if errors.Is(err, fs.ErrNotExist) {
			return remote
		}
		return "/" + filepath.Join(mediaDir, thread.ThreadID, local)
	}
	for _, t := range chain {
		add(Block{Paragraph: tweetTextToMarkdown(t, thread.Config)})

		if len(t.Attachments.MediaKeys) > 0 {
			media := []Media{}
			for _, k := range t.Attachments.MediaKeys {
				for _, m := range t.Includes.Media {
					if m.Key != k {
						continue
					}
					media = append(media, Media{
						DisplayURL: localOrRemoteURL(m.DisplayURLAndFilename()),
						TargetURL:  localOrRemoteURL(m.TargetURLAndFilename()),
					})
					break
				}
			}
			add(Block{Media: media})
		}

		for _, rt := range t.ReferencedTweets {
			if rt.Type != "quoted" {
				continue
			}
			for _, qt := range t.Includes.Tweets {
				if qt.ID != rt.ID {
					continue
				}
				copied := twitter.Tweet{TweetNoIncludes: qt}
				for _, u := range t.Includes.Users {
					if u.ID == copied.AuthorID {
						copied.Includes.Users = []twitter.TwitterUser{u}
					}
				}
				copied.Text = html.UnescapeString(copied.Text)

				var value strings.Builder
				if err := quotedTweetTmpl.Execute(&value, copied); err != nil {
					log.Printf("executing quoted tweet template: %s", err)
					break
				}
				add(Block{QuotedTweet: value.String()})
			}
		}
	}
	tryMergingParagraphs(&r, thread.Config)
	return r
}

type IndexData struct {
	Subdir       common.Subdir
	Prefix       string
	SubdirPath   string
	FileToThread map[string]Thread
	Indent       string
}

func (d *IndexData) Descend(name string, subdir common.Subdir) *IndexData {
	return &IndexData{
		Subdir:       subdir,
		Prefix:       d.Prefix,
		SubdirPath:   filepath.Join(d.SubdirPath, name),
		FileToThread: d.FileToThread,
		Indent:       d.Indent + "  ",
	}
}

func (d *IndexData) PagePath(name string) string {
	return filepath.Join(d.SubdirPath, name) + ".md"
}

func (d *IndexData) Thread(name string) Thread {
	return d.FileToThread[filepath.Join(d.Prefix, d.SubdirPath, name)+".md"]
}

func forEachSubdir(d common.Subdir, callback func(string, common.Subdir) error, path string) error {
	if err := callback(path, d); err != nil {
		return err
	}
	for _, e := range d.Subdirs {
		if err := forEachSubdir(e.Subdir, callback, filepath.Join(path, e.Name)); err != nil {
			return err
		}
	}
	return nil
}

func writeIndexPages(cfg *common.Config, fileToThread map[string]Thread) error {
	return forEachSubdir(cfg.Root, func(path string, sd common.Subdir) error {
		idxPath := filepath.Join(*outputDir, path, "index.md")
		f, err := os.Create(idxPath)
		if err != nil {
			return fmt.Errorf("opening %q: %w", idxPath, err)
		}
		defer f.Close()
		d := &IndexData{
			Subdir:       sd,
			Prefix:       filepath.Join(*outputDir, path),
			FileToThread: fileToThread,
		}
		if err := indexTmpl.Execute(f, d); err != nil {
			return fmt.Errorf("executing template: %w", err)
		}
		return nil
	}, "")
}

func run(cfg *common.Config, state *state.State) error {
	mappings := &PathMapping{Mappings: map[string]PathMappingEntry{}}
	f, err := os.Open(*mappingsPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read mappings file: %w", err)
	}
	if err == nil {
		if err := json.NewDecoder(f).Decode(mappings); err != nil {
			return fmt.Errorf("failed to unmarshal mappings: %w", err)
		}
		f.Close()
	}

	fileToThread := map[string]Thread{}
	for name, thread := range cfg.ThreadPages() {
		t := parseThread(path.Base(name), thread, state.Threads[thread.ThreadID], cfg.MediaDir)

		m := mappings.Mappings[t.ConversationID]
		if m.Name == "" {
			m.Name = name
		}
		if m.Name != name {
			m.OldNames = append(m.OldNames, m.Name)
			m.Name = name
		}
		mappings.Mappings[t.ConversationID] = m
		t.OldNames = m.OldNames

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

		fileToThread[fname] = t
	}

	if err := writeIndexPages(cfg, fileToThread); err != nil {
		return err
	}

	f, err = os.Create(*mappingsPath)
	if err != nil {
		return fmt.Errorf("failed to open mappings file for writing: %s", err)
	}
	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(mappings); err != nil {
		return fmt.Errorf("failed to marshal mappings: %s", err)
	}
	f.Close()

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
