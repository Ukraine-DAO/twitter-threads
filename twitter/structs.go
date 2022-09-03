package twitter

import (
	"github.com/rusni-pyzda/twitter-threads/common"
)

type ReferencedTweet struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type TwitterUser struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Username string `json:"username"`
}

type Media struct {
	Type       string                 `json:"type"`
	Key        string                 `json:"media_key"`
	URL        string                 `json:"url,omitempty"`
	PreviewURL string                 `json:"preview_image_url,omitempty"`
	Variants   map[string]interface{} `json:"variants,omitempty"`
	AltText    string                 `json:"alt_text,omitempty"`
}

type TweetIncludes struct {
	Users  []TwitterUser `json:"users,omitempty"`
	Media  []Media       `json:"media,omitempty"`
	Tweets []Tweet       `json:"tweets,omitempty"`
}

type Entities struct {
	URLs     []EntityURL     `json:"urls,omitempty"`
	Hashtags []EntityHashtag `json:"hashtags,omitempty"`
	Mentions []EntityMention `json:"mentions,omitempty"`
}

type TextEntity struct {
	Start uint `json:"start"`
	End   uint `json:"end"`
}

type EntityURL struct {
	TextEntity
	URL         string `json:"url,omitempty"`
	ExpandedURL string `json:"expanded_url,omitempty"`
	DisplayURL  string `json:"display_url,omitempty"`
	UnwoundURL  string `json:"unwound_url,omitempty"`
	Title       string `json:"title,omitempty"`
}

type EntityHashtag struct {
	TextEntity
	Tag string `json:"tag,omitempty"`
}

type EntityMention struct {
	TextEntity
	Username string `json:"username,omitempty"`
}

type Attachments struct {
	MediaKeys []string `json:"media_keys,omitempty"`
}

type Tweet struct {
	ID               string            `json:"id"`
	Text             string            `json:"text"`
	ConversationID   string            `json:"conversation_id"`
	AuthorID         string            `json:"author_id"`
	ReferencedTweets []ReferencedTweet `json:"referenced_tweets,omitempty"`
	Entities         Entities          `json:"entities,omitempty"`
	Attachments      Attachments       `json:"attachments,omitempty"`
	Includes         TweetIncludes     `json:"includes,omitempty"`

	RequestConfig common.RequestConfig
}

func (t *Tweet) InReplyTo() string {
	for _, ref := range t.ReferencedTweets {
		if ref.Type == "replied_to" {
			return ref.ID
		}
	}
	return ""
}

func (t *Tweet) CopyIncludes(incl TweetIncludes) {
	wantTweets := map[string]bool{}
	for _, rt := range t.ReferencedTweets {
		wantTweets[rt.ID] = true
	}
	for _, tt := range incl.Tweets {
		if wantTweets[tt.ID] {
			t.Includes.Tweets = append(t.Includes.Tweets, tt)
		}
	}

	wantUserById := map[string]bool{
		t.AuthorID: true,
	}
	wantUserByUsername := map[string]bool{}
	for _, m := range t.Entities.Mentions {
		wantUserByUsername[m.Username] = true
	}
	for _, tt := range t.Includes.Tweets {
		wantUserById[tt.AuthorID] = true
	}
	for _, u := range incl.Users {
		if wantUserById[u.ID] || wantUserByUsername[u.Username] {
			t.Includes.Users = append(t.Includes.Users, u)
		}
	}

	wantMedia := map[string]bool{}
	for _, k := range t.Attachments.MediaKeys {
		wantMedia[k] = true
	}
	for _, tt := range t.Includes.Tweets {
		for _, k := range tt.Attachments.MediaKeys {
			wantMedia[k] = true
		}
	}
	for _, m := range incl.Media {
		if wantMedia[m.Key] {
			t.Includes.Media = append(t.Includes.Media, m)
		}
	}
}
