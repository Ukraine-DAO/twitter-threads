package state

import (
	"sort"

	"github.com/rusni-pyzda/twitter-threads/twitter"
)

type ThreadState struct {
	Tweets []twitter.Tweet
}

func (ts *ThreadState) TweetChain() []twitter.Tweet {
	inReplyTo := map[string]string{}
	roots := map[string]bool{}
	// Store graph edges
	for _, t := range ts.Tweets {
		inReplyTo[t.ID] = t.InReplyTo()
		roots[t.ID] = true
	}

	// Find nodes with no outbound edges.
	for k, v := range inReplyTo {
		if _, ok := inReplyTo[v]; ok {
			delete(roots, k)
		}
	}

	// Compute the number of steps in reverse walk to each node from roots
	index := map[string]int{}
	for id := range roots {
		index[id] = 0
	}
	for {
		changed := false
		for k, v := range inReplyTo {
			if _, done := index[k]; done {
				continue
			}
			if i, ok := index[v]; ok {
				index[k] = i + 1
				changed = true
			}
		}
		if !changed {
			break
		}
	}

	// Find max number of steps
	max := 0
	for _, v := range index {
		if v > max {
			max = v
		}
	}

	longest := []string{}
	for id, v := range index {
		if v == max {
			longest = append(longest, id)
		}
	}
	sort.Strings(longest)

	r := []twitter.Tweet{}
	id := longest[0]
	for id != "" {
		for _, t := range ts.Tweets {
			if t.ID == id {
				r = append([]twitter.Tweet{t}, r...)
				break
			}
		}
		id = inReplyTo[id]
	}

	return r
}
