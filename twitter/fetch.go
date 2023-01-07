package twitter

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/Ukraine-DAO/twitter-threads/common"
)

var token = os.Getenv("TWITTER_BEARER_TOKEN")
var ErrThrottled = fmt.Errorf("throttled")

func FetchTweet(id string, config common.RequestConfig) (Tweet, error) {
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
		return Tweet{}, ErrThrottled
	}

	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return Tweet{}, fmt.Errorf("reading body from an error response (code %d): %w", resp.StatusCode, err)
		}
		return Tweet{}, fmt.Errorf("request failed with code %d: %s", resp.StatusCode, body)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Tweet{}, fmt.Errorf("reading response: %w", err)
	}
	log.Printf("%s", string(body))
	d := struct {
		Data     Tweet         `json:"data"`
		Includes TweetIncludes `json:"includes"`
	}{}
	if err := json.Unmarshal(body, &d); err != nil {
		return Tweet{}, fmt.Errorf("decoding response: %w", err)
	}
	t := d.Data
	t.RequestConfig = config
	t.Includes = d.Includes
	return t, nil
}

func FetchTweets(ids []string, config common.RequestConfig) ([]Tweet, error) {
	if token == "" {
		return nil, fmt.Errorf("missing twitter bearer token")
	}
	url := "https://api.twitter.com/2/tweets"
	params := config.QueryParams()
	params.Set("ids", strings.Join(ids, ","))
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
		return nil, fmt.Errorf("sending the request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		if body, err := io.ReadAll(resp.Body); err == nil {
			log.Printf("Throttled:\n%s", string(body))
		}
		return nil, ErrThrottled
	}

	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("reading body from an error response (code %d): %w", resp.StatusCode, err)
		}
		return nil, fmt.Errorf("request failed with code %d: %s", resp.StatusCode, body)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}
	log.Printf("%s", string(body))
	d := struct {
		Data     []Tweet       `json:"data"`
		Includes TweetIncludes `json:"includes"`
	}{}
	if err := json.Unmarshal(body, &d); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	t := d.Data
	for i, tw := range t {
		tw := tw
		tw.RequestConfig = config
		tw.CopyIncludes(d.Includes)
		t[i] = tw
	}
	return t, nil
}

// FetchUserTimeline fetches all tweets written by a user after tweet sinceID.
// Returns valid []Tweet even along with an error.
func FetchUserTimeline(userID string, config common.RequestConfig, sinceID string) ([]Tweet, error) {
	if token == "" {
		return nil, fmt.Errorf("missing twitter bearer token")
	}

	r := []Tweet{}
	params := config.QueryParams()

	for {
		url := fmt.Sprintf("https://api.twitter.com/2/users/%s/tweets", userID)
		if sinceID != "" {
			params.Set("since_id", sinceID)
		}
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
			return r, ErrThrottled
		}

		if resp.StatusCode != http.StatusOK {
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				return r, fmt.Errorf("reading body from an error response (code %d): %w", resp.StatusCode, err)
			}
			return r, fmt.Errorf("request failed with code %d: %s", resp.StatusCode, body)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return r, fmt.Errorf("reading response: %w", err)
		}
		resp.Body.Close()
		log.Printf("%s", string(body))
		d := struct {
			Data     []Tweet       `json:"data"`
			Includes TweetIncludes `json:"includes"`
			Meta     struct {
				NextToken string `json:"next_token"`
			} `json:"meta"`
		}{}
		if err := json.Unmarshal(body, &d); err != nil {
			return r, fmt.Errorf("decoding response: %w", err)
		}

		for _, t := range d.Data {
			t := t
			t.RequestConfig = config
			t.CopyIncludes(d.Includes)
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

func GetUserID(username string) (string, error) {
	if token == "" {
		return "", fmt.Errorf("missing twitter bearer token")
	}

	url := fmt.Sprintf("https://api.twitter.com/2/users/by/username/%s", username)
	log.Printf("GET %s", url)
	req, _ := http.NewRequest("GET", url, nil)

	req.Header.Add("Accept", "application/json")
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", token))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("sending the request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return "", ErrThrottled
	}

	if resp.StatusCode != http.StatusOK {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return "", fmt.Errorf("reading body from an error response (code %d): %w", resp.StatusCode, err)
		}
		return "", fmt.Errorf("request failed with code %d: %s", resp.StatusCode, body)
	}

	v := struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}{}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return "", fmt.Errorf("decoding response: %w", err)
	}
	return v.Data.ID, nil
}

func Search(query string, config common.RequestConfig, sinceID string) ([]Tweet, error) {
	if token == "" {
		return nil, fmt.Errorf("missing twitter bearer token")
	}

	r := []Tweet{}
	params := config.QueryParams()

	for {
		url := "https://api.twitter.com/2/tweets/search/recent"
		params.Set("query", query)
		if sinceID != "" {
			params.Set("since_id", sinceID)
		}
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
			return r, ErrThrottled
		}

		if resp.StatusCode != http.StatusOK {
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				return r, fmt.Errorf("reading body from an error response (code %d): %w", resp.StatusCode, err)
			}
			return r, fmt.Errorf("request failed with code %d: %s", resp.StatusCode, body)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return r, fmt.Errorf("reading response: %w", err)
		}
		resp.Body.Close()
		log.Printf("%s", string(body))
		d := struct {
			Data     []Tweet       `json:"data"`
			Includes TweetIncludes `json:"includes"`
			Meta     struct {
				NextToken string `json:"next_token"`
			} `json:"meta"`
		}{}
		if err := json.Unmarshal(body, &d); err != nil {
			return r, fmt.Errorf("decoding response: %w", err)
		}

		for _, t := range d.Data {
			t := t
			t.RequestConfig = config
			t.CopyIncludes(d.Includes)
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
