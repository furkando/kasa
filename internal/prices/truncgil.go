package prices

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const truncgilURL = "https://finans.truncgil.com/v4/today.json"

type truncgilEntry struct {
	Buying  float64 `json:"Buying"`
	Selling float64 `json:"Selling"`
	Change  float64 `json:"Change"` // percent
}

// gramGoldQuote reads gram altın (GRA) from truncgil v4. The endpoint drops
// connections intermittently, so retry a few times before giving up.
func gramGoldQuote(ctx context.Context) (Quote, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(500 * time.Millisecond):
			case <-ctx.Done():
				return Quote{}, ctx.Err()
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, truncgilURL, nil)
		if err != nil {
			return Quote{}, err
		}
		resp, err := httpClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		var raw map[string]json.RawMessage
		err = json.NewDecoder(resp.Body).Decode(&raw)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		var gra truncgilEntry
		if err := json.Unmarshal(raw["GRA"], &gra); err != nil || gra.Buying == 0 {
			lastErr = fmt.Errorf("truncgil: bad GRA payload")
			continue
		}
		// Mark at Buying (what the market pays you); back out prev close from Change%.
		prev := gra.Buying / (1 + gra.Change/100)
		return Quote{
			Symbol: KeyGram, Name: "Gram Altın",
			Price: gra.Buying, PrevClose: prev,
			Currency: "TRY", At: time.Now(),
		}, nil
	}
	return Quote{}, lastErr
}
