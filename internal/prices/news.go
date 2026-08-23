package prices

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type NewsItem struct {
	Title     string
	Publisher string
	Link      string
	At        time.Time
}

// FetchNews pulls recent headlines for a query from Yahoo's search API.
// When wantTicker is set, items are kept only if Yahoo relates them to that
// symbol — Yahoo falls back to generic front-page news for unknown queries,
// which is worse than showing nothing.
func FetchNews(ctx context.Context, query, wantTicker string, limit int) ([]NewsItem, error) {
	if query == "" {
		return nil, nil
	}
	u := fmt.Sprintf("https://query1.finance.yahoo.com/v1/finance/search?q=%s&newsCount=%d&quotesCount=0",
		url.QueryEscape(query), limit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var out struct {
		News []struct {
			Title               string   `json:"title"`
			Publisher           string   `json:"publisher"`
			Link                string   `json:"link"`
			ProviderPublishTime int64    `json:"providerPublishTime"`
			RelatedTickers      []string `json:"relatedTickers"`
		} `json:"news"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	items := make([]NewsItem, 0, len(out.News))
	for _, n := range out.News {
		if wantTicker != "" {
			related := false
			for _, t := range n.RelatedTickers {
				if strings.EqualFold(t, wantTicker) {
					related = true
					break
				}
			}
			if !related {
				continue
			}
		}
		items = append(items, NewsItem{
			Title: n.Title, Publisher: n.Publisher, Link: n.Link,
			At: time.Unix(n.ProviderPublishTime, 0),
		})
	}
	return items, nil
}

// FetchNewsRSS pulls headlines from Google News RSS — the fallback for assets
// Yahoo can't search (crypto) and for Turkish-language topics (kur, altın).
// lang is "tr" or "en".
func FetchNewsRSS(ctx context.Context, query, lang string, limit int) ([]NewsItem, error) {
	params := "hl=en-US&gl=US&ceid=US:en"
	if lang == "tr" {
		params = "hl=tr&gl=TR&ceid=TR:tr"
	}
	u := fmt.Sprintf("https://news.google.com/rss/search?q=%s&%s", url.QueryEscape(query), params)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var feed struct {
		Items []struct {
			Title   string `xml:"title"`
			Link    string `xml:"link"`
			PubDate string `xml:"pubDate"`
			Source  string `xml:"source"`
		} `xml:"channel>item"`
	}
	if err := xml.NewDecoder(resp.Body).Decode(&feed); err != nil {
		return nil, err
	}
	items := make([]NewsItem, 0, limit)
	for _, it := range feed.Items {
		if len(items) >= limit {
			break
		}
		title := strings.TrimSuffix(it.Title, " - "+it.Source)
		at := time.Time{}
		for _, layout := range []string{time.RFC1123, time.RFC1123Z} {
			if t, err := time.Parse(layout, it.PubDate); err == nil {
				at = t
				break
			}
		}
		items = append(items, NewsItem{Title: title, Publisher: it.Source, Link: it.Link, At: at})
	}
	return items, nil
}
