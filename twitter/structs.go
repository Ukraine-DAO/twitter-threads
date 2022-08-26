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
	Type       string `json:"type"`
	Key        string `json:"media_key"`
	URL        string `json:"url,omitempty"`
	PreviewURL string `json:"preview_image_url,omitempty"`
}

type TweetIncludes struct {
	Users  []TwitterUser `json:"users,omitempty"`
	Media  []Media       `json:"media,omitempty"`
	Tweets []Tweet       `json:"tweets,omitempty"`
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
