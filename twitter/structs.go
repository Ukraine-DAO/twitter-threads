package twitter

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/Ukraine-DAO/twitter-threads/common"
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
	Type       string                   `json:"type"`
	Key        string                   `json:"media_key"`
	URL        string                   `json:"url,omitempty"`
	PreviewURL string                   `json:"preview_image_url,omitempty"`
	Variants   []map[string]interface{} `json:"variants,omitempty"`
	AltText    string                   `json:"alt_text,omitempty"`
}

type mediaVariant struct {
	BitRate     int    `json:"bit_rate,omitempty"`
	ContentType string `json:"content_type"`
	URL         string `json:"url"`
}

func (m Media) DisplayURLAndFilename() (string, string) {
	switch m.Type {
	case "video":
		url, _ := url.Parse(m.PreviewURL)
		parts := strings.Split(url.Path, ".")
		ext := parts[len(parts)-1]
		return m.PreviewURL, fmt.Sprintf("%s/preview.%s", m.Key, ext)
	default:
		url, _ := url.Parse(m.URL)
		parts := strings.Split(url.Path, ".")
		ext := parts[len(parts)-1]
		return m.URL, fmt.Sprintf("%s.%s", m.Key, ext)
	}
}

func (m Media) TargetURLAndFilename() (string, string) {
	switch m.Type {
	case "video":
		bitrate := 0
		u := ""
		for _, v := range m.Variants {
			mv := &mediaVariant{}
			b, _ := json.Marshal(v)
			if err := json.Unmarshal(b, mv); err != nil {
				continue
			}
			if !strings.HasPrefix(mv.ContentType, "video/") {
				continue
			}
			if mv.BitRate > bitrate {
				bitrate = mv.BitRate
				u = mv.URL
			}
		}
		url, _ := url.Parse(u)
		return u, fmt.Sprintf("%s/%s", m.Key, path.Base(url.Path))
	default:
		url, _ := url.Parse(m.URL)
		parts := strings.Split(url.Path, ".")
		ext := parts[len(parts)-1]
		return m.URL, fmt.Sprintf("%s.%s", m.Key, ext)
	}
}

func (m Media) FetchList() map[string]string {
	r := map[string]string{}
	switch m.Type {
	case "video":
		u, _ := url.Parse(m.PreviewURL)
		parts := strings.Split(u.Path, ".")
		ext := parts[len(parts)-1]
		r[fmt.Sprintf("%s/preview.%s", m.Key, ext)] = m.PreviewURL

		for _, v := range m.Variants {
			mv := &mediaVariant{}
			b, _ := json.Marshal(v)
			if err := json.Unmarshal(b, mv); err != nil {
				continue
			}
			if !strings.HasPrefix(mv.ContentType, "video/") {
				continue
			}
			u, _ := url.Parse(mv.URL)
			r[fmt.Sprintf("%s/%s", m.Key, path.Base(u.Path))] = mv.URL
		}
	default:
		url, _ := url.Parse(m.URL)
		parts := strings.Split(url.Path, ".")
		ext := parts[len(parts)-1]
		r[fmt.Sprintf("%s.%s", m.Key, ext)] = m.URL
	}
	return r
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
	CreatedAt        string            `json:"created_at,omitempty"`
	InReplyToUserID  string            `json:"in_reply_to_user_id,omitempty"`

	RequestConfig common.RequestConfig `json:",omitempty"`
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
