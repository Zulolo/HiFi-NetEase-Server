package netease

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/chaunsin/netease-cloud-music/api"
	"github.com/chaunsin/netease-cloud-music/api/types"
)

// The library (v0.8.1) offers only keyword suggestions, not keyword search, so
// the cloudsearch endpoint is called directly through its authenticated weapi
// client, the same way its own methods are written.

type cloudSearchReq struct {
	types.ReqCommon
	S      string `json:"s"`
	Type   int    `json:"type"` // 1 = songs
	Limit  int    `json:"limit"`
	Offset int    `json:"offset"`
	Total  bool   `json:"total"`
}

type cloudSearchResp struct {
	types.RespCommon[any]
	Result struct {
		SongCount int64 `json:"songCount"`
		Songs     []struct {
			Id   int64          `json:"id"`
			Name string         `json:"name"`
			Ar   []types.Artist `json:"ar"`
			Al   types.Album    `json:"al"`
			Dt   int64          `json:"dt"`
			Fee  int64          `json:"fee"`
		} `json:"songs"`
	} `json:"result"`
}

// Search finds songs by keyword. It returns the page and the total match count.
func (c *Client) Search(ctx context.Context, query string, offset, limit int) ([]Track, int, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []Track{}, 0, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	const url = "https://music.163.com/weapi/cloudsearch/pc"
	req := cloudSearchReq{S: query, Type: 1, Limit: limit, Offset: offset, Total: true}
	if csrf, ok := c.raw.GetCSRF(url); ok {
		req.CSRFToken = csrf
	}
	var reply cloudSearchResp
	if _, err := c.raw.Request(ctx, url, &req, &reply, api.NewOptions("weapi.CloudSearch")); err != nil {
		return nil, 0, fmt.Errorf("netease: search: %w", err)
	}
	if reply.Code != 200 {
		return nil, 0, fmt.Errorf("netease: search: code %d %s", reply.Code, reply.Message)
	}
	out := make([]Track, 0, len(reply.Result.Songs))
	for _, s := range reply.Result.Songs {
		names := make([]string, 0, len(s.Ar))
		for _, a := range s.Ar {
			if a.Name != "" {
				names = append(names, a.Name)
			}
		}
		t := Track{
			ID:       s.Id,
			Ref:      "ncm:" + strconv.FormatInt(s.Id, 10),
			Title:    s.Name,
			Artist:   strings.Join(names, ", "),
			Album:    s.Al.Name,
			Cover:    s.Al.PicUrl,
			Duration: float64(s.Dt) / 1000.0,
		}
		_, t.OnDisk = c.LocalPath(t.ID)
		out = append(out, t)
	}
	return out, int(reply.Result.SongCount), nil
}
