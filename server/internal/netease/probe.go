package netease

import (
	"context"
	"strconv"

	"github.com/chaunsin/netease-cloud-music/api/types"
	"github.com/chaunsin/netease-cloud-music/api/weapi"
)

// LevelProbe is what NetEase answered for one requested quality level.
type LevelProbe struct {
	Asked   string `json:"asked"`
	Code    int64  `json:"code"`
	Granted string `json:"granted,omitempty"`
	Type    string `json:"type,omitempty"`
	Size    int64  `json:"size,omitempty"`
	Bitrate int64  `json:"bitrate,omitempty"`
	HasURL  bool   `json:"has_url"`
	Error   string `json:"error,omitempty"`
}

// Probe is a diagnostic: the account's privileges for one song as NetEase
// reports them, and the raw answer for every level of the download ladder
// (no short-circuit, no cache). It answers "why did this track fail".
type Probe struct {
	ID     int64  `json:"id"`
	Title  string `json:"title,omitempty"`
	Artist string `json:"artist,omitempty"`
	// St < 0 is a greyed-out song (removed or no copyright); Pl / Dl are the
	// highest bitrates the account may play / download (0 = not at all);
	// Toast is "unavailable in your region".
	St         int64        `json:"st"`
	Pl         int64        `json:"pl"`
	Dl         int64        `json:"dl"`
	Fee        int64        `json:"fee"`
	MaxBr      int64        `json:"maxbr"`
	MaxBrLevel string       `json:"max_br_level,omitempty"`
	Toast      bool         `json:"region_blocked"`
	Levels     []LevelProbe `json:"levels"`
}

func (c *Client) Probe(ctx context.Context, id int64) (Probe, error) {
	p := Probe{ID: id}
	det, err := c.we.SongDetail(ctx, &weapi.SongDetailReq{
		C: []weapi.SongDetailReqList{{Id: strconv.FormatInt(id, 10)}},
	})
	if err != nil {
		return p, err
	}
	if len(det.Songs) > 0 {
		p.Title = det.Songs[0].Name
		if len(det.Songs[0].Ar) > 0 {
			p.Artist = det.Songs[0].Ar[0].Name
		}
	}
	if len(det.Privileges) > 0 {
		v := det.Privileges[0]
		p.St, p.Pl, p.Dl, p.Fee, p.MaxBr, p.MaxBrLevel, p.Toast = v.St, v.Pl, v.Dl, v.Fee, v.Maxbr, v.MaxBrLevel, v.Toast
	}
	for _, level := range c.levels {
		lp := LevelProbe{Asked: string(level)}
		resp, err := c.we.SongPlayerV1(ctx, &weapi.SongPlayerV1Req{
			Ids: types.IntsString{id}, Level: level, EncodeType: "flac",
		})
		switch {
		case err != nil:
			lp.Error = err.Error()
		case len(resp.Data) == 0:
			lp.Error = "no data"
		default:
			d := resp.Data[0]
			lp.Code, lp.Granted, lp.Type, lp.Size, lp.Bitrate, lp.HasURL = d.Code, string(d.Level), d.Type, d.Size, d.Br, d.Url != ""
		}
		p.Levels = append(p.Levels, lp)
	}
	return p, nil
}
