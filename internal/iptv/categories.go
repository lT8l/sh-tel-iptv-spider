package iptv

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"iptv-spider-sh/internal/config"
)

// FetchChannelInfosByCategories collects channels from every configured
// cateID/type pair. Quality variants may share the same MixNo, so deduplication
// is deferred to DedupeChannelInfos.
func (c *Client) FetchChannelInfosByCategories(ctx context.Context, categories []config.CategoryConfig, interval time.Duration) ([]ChannelInfo, error) {
	if c.epgHostURL == "" {
		return nil, fmt.Errorf("client is not authenticated")
	}
	if len(categories) == 0 {
		return nil, fmt.Errorf("channel categories are empty")
	}

	endpoint := c.epgHostURL + "/function/ajax/epg7getChannelByAjax.jsp"
	infos := make([]ChannelInfo, 0, 256)

	for i, category := range categories {
		body, _, err := c.request(ctx, http.MethodPost, endpoint, map[string]string{
			"action": "getChannelList",
			"cateID": category.CateID,
			"type":   category.Type,
		})
		if err != nil {
			return nil, fmt.Errorf("channel category %s/%s: %w", category.CateID, categoryTypeLabel(category.Type), err)
		}

		var resp jsonResponse[ChannelInfo]
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("decode channel category %s/%s: %w", category.CateID, categoryTypeLabel(category.Type), err)
		}
		if err := resp.err(); err != nil {
			return nil, fmt.Errorf("channel category %s/%s: %w", category.CateID, categoryTypeLabel(category.Type), err)
		}

		added := 0
		for _, raw := range resp.Data {
			ch := normalizeChannelInfo(raw)
			if ch.MixNo == "" {
				continue
			}
			infos = append(infos, ch)
			added++
		}
		log.Printf("channel category %s/%s returned %d channels, accepted %d", category.CateID, categoryTypeLabel(category.Type), len(resp.Data), added)

		if i < len(categories)-1 && interval > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(interval):
			}
		}
	}

	if len(infos) == 0 {
		return nil, fmt.Errorf("channel list is empty across %d categories", len(categories))
	}

	sortChannelInfos(infos)
	log.Printf("channel categories fetched: %d, rows: %d", len(categories), len(infos))
	return infos, nil
}

func categoryTypeLabel(value string) string {
	if value == "" {
		return "-"
	}
	return value
}
