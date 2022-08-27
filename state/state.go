package state

import (
	"fmt"
	"math/big"

	"github.com/rusni-pyzda/twitter-threads/common"
	"github.com/rusni-pyzda/twitter-threads/twitter"
)

var requestCfg = common.RequestConfig{
	Expansions: []string{
		"author_id",
		"attachments.media_keys",
		"referenced_tweets.id",
		"referenced_tweets.id.author_id",
	},
	TweetFields: []string{
		"author_id",
		"conversation_id",
		"entities",
		"referenced_tweets",
		"text",
		"attachments",
	},
	MediaFields: []string{
		"media_key",
		"type",
		"url",
		"preview_image_url",
	},
}

func errIfNotThrottled(err error) error {
	if err == twitter.ErrThrottled {
		return nil
	}
	return err
}

type State struct {
	Threads map[string]ThreadState
	// UserTimelineTail contains the id of the last tweet we've seen on user's timeline.
	UserTimelineTail map[string]string
}

func New() *State {
	return &State{
		Threads:          map[string]ThreadState{},
		UserTimelineTail: map[string]string{},
	}
}

func (state *State) Update(cfg *common.Config) error {
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
			tt, err := twitter.FetchTweet(t.ID, requestCfg)
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

func (state *State) WalkThreadsUp(cfg *common.Config) error {
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
			t, err := twitter.FetchTweet(parent, requestCfg)
			if err != nil {
				return err
			}
			ts.Tweets = append([]twitter.Tweet{t}, ts.Tweets...)
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
	t, err := twitter.FetchTweet(id, requestCfg)
	if err != nil {
		return err
	}
	state.Threads[id] = ThreadState{Tweets: []twitter.Tweet{t}}
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

	tweets, err := twitter.FetchUserTimeline(authorID, requestCfg, state.UserTimelineTail[authorID])

	byConvId := map[string][]twitter.Tweet{}
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

func (ts *ThreadState) Update(tweets []twitter.Tweet) {
	existing := map[string]bool{}
	for _, t := range ts.Tweets {
		existing[t.ID] = true
	}

	byId := map[string]twitter.Tweet{}
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
