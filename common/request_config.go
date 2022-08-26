package common

import (
	"net/url"
	"strings"
)

type RequestConfig struct {
	Expansions  []string `json:",omitempty"`
	TweetFields []string `json:",omitempty"`
	MediaFields []string `json:",omitempty"`
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
	if len(c.MediaFields) > 0 {
		params.Set("media.fields", strings.Join(c.MediaFields, ","))
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
