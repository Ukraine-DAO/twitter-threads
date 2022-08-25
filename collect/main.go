package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	configPath = flag.String("config", "", "Path to the config file")
	statePath  = flag.String("state", "", "Path to the state file")
)

type Config struct {
	Root Subdir
}

type Subdir struct {
	Subdirs map[string]Subdir     `yaml:",omitempty"`
	Pages   map[string]YamlThread `yaml:",inline,omitempty"`
}

type Thread struct {
	ThreadID string `yaml:"thread_id"`
}

type YamlThread struct {
	Thread `yaml:",inline"`
}

func (t *YamlThread) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		return value.Decode(&t.ThreadID)
	}
	return value.Decode(&t.Thread)
}

func (t YamlThread) MarshalYAML() (interface{}, error) {
	if true {
		return t.ThreadID, nil
	}
	return t.Thread, nil
}

func (s *Subdir) ThreadIDs() []string {
	r := []string{}
	for _, v := range s.Pages {
		r = append(r, v.ThreadID)
	}
	for _, sd := range s.Subdirs {
		r = append(r, sd.ThreadIDs()...)
	}
	return r
}

func (cfg *Config) ThreadIDs() []string {
	id_list := cfg.Root.ThreadIDs()
	present := map[string]bool{}
	r := []string{}
	for _, id := range id_list {
		if present[id] {
			continue
		}
		present[id] = true
		r = append(r, id)
	}
	sort.Strings(r)
	return r
}

type State struct {
	Threads map[string]ThreadState
	// UserTimelineTail contains the id of the last tweet we've seen on user's timeline.
	UserTimelineTail map[string]string
}

func NewState() *State {
	return &State{
		Threads:          map[string]ThreadState{},
		UserTimelineTail: map[string]string{},
	}
}

type ThreadState struct {
	Tweets []Tweet
}

type RequestConfig struct {
	Expansions  []string `json:",omitempty"`
	TweetFields []string `json:",omitempty"`
	CustomFlags []string `json:",omitmepty"`
}

func (c RequestConfig) QueryParams() url.Values {
	params := url.Values{}
	if len(c.Expansions) > 0 {
		params.Set("expansions", strings.Join(c.Expansions, ","))
	}
	if len(c.TweetFields) > 0 {
		params.Set("tweet.fields", strings.Join(c.TweetFields, ","))

	}
	return params
}

func (c RequestConfig) Equal(other RequestConfig) bool {
	setEqual := func(a []string, b []string) bool {
		if len(a) != len(b) {
			return false
		}
		as := map[string]bool{}
		for _, aa := range a {
			as[aa] = true
		}
		for _, bb := range b {
			if !as[bb] {
				return false
			}
		}
		return true
	}
	return setEqual(c.Expansions, other.Expansions) &&
		setEqual(c.TweetFields, other.TweetFields) &&
		setEqual(c.CustomFlags, other.CustomFlags)
}

type ReferencedTweet struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type TwitterUser struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Username string `json:"username"`
}

type TweetIncludes struct {
	Users []TwitterUser `json:"users,omitempty"`
}

type Entities struct {
	URLs []EntityURL `json:"urls,omitempty"`
}

type EntityURL struct {
	Start       uint   `json:"start"`
	End         uint   `json:"end"`
	URL         string `json:"url,omitempty"`
	ExpandedURL string `json:"expanded_url,omitempty"`
	DisplayURL  string `json:"display_url,omitempty"`
	UnwoundURL  string `json:"unwound_url,omitempty"`
	Title       string `json:"title,omitempty"`
}

type Tweet struct {
	ID               string            `json:"id"`
	Text             string            `json:"text"`
	ConversationID   string            `json:"conversation_id"`
	AuthorID         string            `json:"author_id"`
	ReferencedTweets []ReferencedTweet `json:"referenced_tweets,omitempty"`
	Includes         TweetIncludes     `json:"includes,omitempty"`
	Entities         Entities          `json:"entities,omitempty"`

	RequestConfig RequestConfig
}

func (t *Tweet) InReplyTo() string {
	for _, ref := range t.ReferencedTweets {
		if ref.Type == "replied_to" {
			return ref.ID
		}
	}
	return ""
}

var errThrottled = fmt.Errorf("throttled")

func errIfNotThrottled(err error) error {
	if err == errThrottled {
		return nil
	}
	return err
}

func (state *State) Update(cfg *Config) error {
	newThreads := []string{}
	for _, id := range cfg.ThreadIDs() {
		if _, ok := state.Threads[id]; !ok {
			newThreads = append(newThreads, id)
		}
	}

	for _, id := range newThreads {
		if err := state.CreateThread(id); err != nil {
			return errIfNotThrottled(err)
		}
	}

	if err := state.WalkThreadsUp(cfg); err != nil {
		return errIfNotThrottled(err)
	}

	updated := map[string]ThreadState{}
	for id, ts := range state.Threads {
		for i, t := range ts.Tweets {
			if t.RequestConfig.Equal(requestCfg) {
				continue
			}
			tt, err := fetchTweet(t.ID, requestCfg)
			if err != nil {
				// TODO: skip deleted/unavailable tweets instead of crashing
				return err
			}
			ts.Tweets[i] = tt
			updated[id] = ts
		}
	}
	for id, ts := range updated {
		state.Threads[id] = ts
	}

	byAuthorID := map[string][]string{}
	for id, ts := range state.Threads {
		byAuthorID[ts.Tweets[0].AuthorID] = append(byAuthorID[ts.Tweets[0].AuthorID], id)
	}

	for authorID, threads := range byAuthorID {
		if err := state.UpdateThreads(authorID, threads); err != nil {
			return errIfNotThrottled(err)
		}
	}

	return nil
}

var requestCfg = RequestConfig{
	Expansions: []string{"author_id"},
	TweetFields: []string{
		"author_id",
		"conversation_id",
		"entities",
		"referenced_tweets",
		"text",
	},
}

func (state *State) WalkThreadsUp(cfg *Config) error {
	for _, id := range cfg.ThreadIDs() {
		gotHead := false
		for _, t := range state.Threads[id].Tweets {
			if t.ConversationID == t.ID {
				gotHead = true
				break
			}
		}
		if gotHead {
			continue
		}

		ts := state.Threads[id]
		if ts.Tweets[0].ConversationID == id {
			continue
		}
		parent := ts.Tweets[0].InReplyTo()
		for {
			t, err := fetchTweet(parent, requestCfg)
			if err != nil {
				return err
			}
			ts.Tweets = append([]Tweet{t}, ts.Tweets...)
			state.Threads[id] = ts
			if t.ConversationID == t.ID {
				break
			}
			parent = t.InReplyTo()
			if parent == "" {
				return fmt.Errorf("have no parent for a tweet %#v", t)
			}
		}
	}
	return nil
}

func (state *State) CreateThread(id string) error {
	t, err := fetchTweet(id, requestCfg)
	if err != nil {
		return err
	}
	state.Threads[id] = ThreadState{Tweets: []Tweet{t}}
	if state.UserTimelineTail[t.AuthorID] == "" {
		state.UserTimelineTail[t.AuthorID] = id
	} else {
		// Reset timeline tail so we can fetch the newly created thread.
		var tail, convId big.Int
		if _, ok := tail.SetString(state.UserTimelineTail[t.AuthorID], 10); !ok {
			return fmt.Errorf("failed to convert tail tweet ID %q into big.Int", state.UserTimelineTail[t.AuthorID])
		}
		if _, ok := convId.SetString(id, 10); !ok {
			return fmt.Errorf("failed to convert conversation_id %q into big.Int", id)
		}
		if convId.Cmp(&tail) < 0 { // convId < tail
			state.UserTimelineTail[t.AuthorID] = id
		}
	}
	return nil
}

func (state *State) UpdateThreads(authorID string, threads []string) error {
	max := func(a string, b string) string {
		var aa, bb big.Int
		if _, ok := aa.SetString(a, 10); !ok {
			return a
		}
		if _, ok := bb.SetString(b, 10); !ok {
			return a
		}
		if bb.Cmp(&aa) > 0 {
			return b
		}
		return a
	}

	convId := map[string]string{}
	haveConvId := map[string]bool{}
	for _, id := range threads {
		convId[id] = state.Threads[id].Tweets[0].ConversationID
		haveConvId[state.Threads[id].Tweets[0].ConversationID] = true
	}

	tweets, err := fetchUserTimeline(authorID, requestCfg, state.UserTimelineTail[authorID])

	byConvId := map[string][]Tweet{}
	for _, t := range tweets {
		if t.AuthorID != authorID {
			// Should never happen, but whatever.
			continue
		}
		state.UserTimelineTail[authorID] = max(state.UserTimelineTail[authorID], t.ID)
		if !haveConvId[t.ConversationID] {
			continue
		}
		c := t.ConversationID
		byConvId[c] = append(byConvId[c], t)
	}

	for _, id := range threads {
		if newTweets := byConvId[convId[id]]; len(newTweets) > 0 {
			ts := state.Threads[id]
			ts.Update(newTweets)
			state.Threads[id] = ts
		}
	}

	if err != nil {
		return err
	}
	return nil
}

func (ts *ThreadState) Update(tweets []Tweet) {
	existing := map[string]bool{}
	for _, t := range ts.Tweets {
		existing[t.ID] = true
	}

	byId := map[string]Tweet{}
	for _, t := range tweets {
		byId[t.ID] = t
	}

	for {
		added := 0
		for id := range existing {
			delete(byId, id)
		}
		for id, t := range byId {
			if existing[t.InReplyTo()] {
				ts.Tweets = append(ts.Tweets, t)
				existing[id] = true
				added++
			}
		}
		if added == 0 {
			break
		}
	}
}

func fetchTweet(id string, config RequestConfig) (Tweet, error) {
	token := os.Getenv("TWITTER_BEARER_TOKEN")
	if token == "" {
		return Tweet{}, fmt.Errorf("missing twitter bearer token")
	}
	url := fmt.Sprintf("https://api.twitter.com/2/tweets/%s", id)
	if encoded := config.QueryParams().Encode(); len(encoded) > 0 {
		url += "?" + encoded
	}
	log.Printf("GET %s", url)
	req, _ := http.NewRequest("GET", url, nil)

	req.Header.Add("Accept", "application/json")
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", token))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Tweet{}, fmt.Errorf("sending the request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return Tweet{}, errThrottled
	}

	if resp.StatusCode != http.StatusOK {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return Tweet{}, fmt.Errorf("reading body from an error response (code %d): %w", resp.StatusCode, err)
		}
		return Tweet{}, fmt.Errorf("request failed with code %d: %s", resp.StatusCode, body)
	}

	d := struct {
		Data     Tweet         `json:"data"`
		Includes TweetIncludes `json:"includes"`
	}{}
	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return Tweet{}, fmt.Errorf("decoding response: %w", err)
	}
	log.Printf("%#v", d)
	t := d.Data
	t.RequestConfig = config
	t.Includes = d.Includes
	return t, nil
}

// fetchUserTimeline fetches all tweets written by a user after tweet sinceID.
// Returns valid []Tweet even along with an error.
func fetchUserTimeline(userID string, config RequestConfig, sinceID string) ([]Tweet, error) {
	token := os.Getenv("TWITTER_BEARER_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("missing twitter bearer token")
	}

	r := []Tweet{}
	params := config.QueryParams()

	for {
		url := fmt.Sprintf("https://api.twitter.com/2/users/%s/tweets", userID)
		params.Set("since_id", sinceID)
		params.Set("max_results", "100")
		if encoded := params.Encode(); len(encoded) > 0 {
			url += "?" + encoded
		}
		log.Printf("GET %s", url)
		req, _ := http.NewRequest("GET", url, nil)

		req.Header.Add("Accept", "application/json")
		req.Header.Add("Content-Type", "application/json")
		req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", token))

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return r, fmt.Errorf("sending the request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusTooManyRequests {
			resp.Body.Close()
			return r, errThrottled
		}

		if resp.StatusCode != http.StatusOK {
			body, err := ioutil.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				return r, fmt.Errorf("reading body from an error response (code %d): %w", resp.StatusCode, err)
			}
			return r, fmt.Errorf("request failed with code %d: %s", resp.StatusCode, body)
		}

		d := struct {
			Data     []Tweet       `json:"data"`
			Includes TweetIncludes `json:"includes"`
			Meta     struct {
				NextToken string `json:"next_token"`
			} `json:"meta"`
		}{}
		if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
			resp.Body.Close()
			return r, fmt.Errorf("decoding response: %w", err)
		}
		resp.Body.Close()

		log.Printf("%#v", d)
		for _, t := range d.Data {
			t.RequestConfig = config
			t.Includes = d.Includes
			r = append(r, t)
		}

		if resp.Header.Get("x-rate-limit-remaining") == "0" {
			break
		}
		if d.Meta.NextToken == "" {
			break
		}
		params.Set("pagination_token", d.Meta.NextToken)
	}
	return r, nil
}

func main() {
	flag.Parse()

	b, err := os.ReadFile(*configPath)
	if err != nil {
		log.Fatalf("Failed to read config file: %s", err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(b, cfg); err != nil {
		log.Fatalf("Failed to unmarshal config: %s", err)
	}

	state := NewState()
	f, err := os.Open(*statePath)
	if err != nil && !os.IsNotExist(err) {
		log.Fatalf("Failed to read state file: %s", err)
	}

	if err == nil {
		if err := json.NewDecoder(f).Decode(state); err != nil {
			log.Fatalf("Failed to unmarshal state: %s", err)
		}
		f.Close()
	}

	if err := state.Update(cfg); err != nil {
		log.Fatalf("Failed to update state: %s", err)
	}

	f, err = os.Create(*statePath)
	if err != nil {
		log.Fatalf("Failed to open state file for writing: %s", err)
	}
	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(state); err != nil {
		log.Fatalf("Failed to marshal state: %s", err)
	}
	f.Close()
}
