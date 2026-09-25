package netease

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/chaunsin/netease-cloud-music/api/weapi"
)

// Daily returns today's recommended songs (每日推荐) for the logged-in
// account, in the same row shape as playlist tracks.
func (c *Client) Daily(ctx context.Context) ([]Track, error) {
	reply, err := c.we.RecommendSongs(ctx, &weapi.RecommendSongsReq{})
	if err != nil {
		return nil, fmt.Errorf("netease: daily: %w", err)
	}
	if reply.Code != 200 {
		return nil, fmt.Errorf("netease: daily: code %d %s", reply.Code, reply.Message)
	}
	out := make([]Track, 0, len(reply.Data.DailySongs))
	for _, s := range reply.Data.DailySongs {
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
	return out, nil
}
