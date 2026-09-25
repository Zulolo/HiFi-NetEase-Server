package netease

import (
	"context"
	"fmt"
	"time"

	"github.com/chaunsin/netease-cloud-music/api/weapi"
	"github.com/skip2/go-qrcode"
)

// QR login (FR-1.1): the phone app scans a code that hifid renders, NetEase
// binds the session to this client, and the library's cookie jar persists it.
// No password or captcha ever passes through the server.

const qrTTL = 5 * time.Minute

type qrSession struct {
	png     []byte
	created time.Time
}

// QRStatus is what the UI polls.
type QRStatus struct {
	Key    string `json:"key"`
	Status string `json:"status"` // waiting | scanned | ok | expired
	Code   int64  `json:"code"`
}

// StartQRLogin asks NetEase for a login key and renders it as a 256 px PNG.
func (c *Client) StartQRLogin(ctx context.Context) (key string, png []byte, err error) {
	created, err := c.we.QrcodeCreateKey(ctx, &weapi.QrcodeCreateKeyReq{Type: 1})
	if err != nil {
		return "", nil, fmt.Errorf("netease: qr key: %w", err)
	}
	if created.UniKey == "" {
		return "", nil, fmt.Errorf("netease: qr key: empty key (code %d)", created.Code)
	}
	img, err := c.we.QrcodeGenerate(ctx, &weapi.QrcodeGenerateReq{
		CodeKey:  created.UniKey,
		Level:    qrcode.Medium,
		Platform: "web",
	})
	if err != nil {
		return "", nil, fmt.Errorf("netease: qr image: %w", err)
	}
	c.mu.Lock()
	if c.qr == nil {
		c.qr = make(map[string]qrSession)
	}
	for k, s := range c.qr { // drop stale codes
		if time.Since(s.created) > qrTTL {
			delete(c.qr, k)
		}
	}
	c.qr[created.UniKey] = qrSession{png: img.Qrcode, created: time.Now()}
	c.mu.Unlock()
	return created.UniKey, img.Qrcode, nil
}

// QRImage returns the PNG for a pending login, if it is still valid.
func (c *Client) QRImage(key string) ([]byte, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	s, ok := c.qr[key]
	if !ok || time.Since(s.created) > qrTTL {
		return nil, false
	}
	return s.png, true
}

// CheckQRLogin polls NetEase for the code's state. On success the session
// cookies have already been captured by the client, so cached account state is
// dropped and the next Profile() call sees the new login.
func (c *Client) CheckQRLogin(ctx context.Context, key string) (QRStatus, error) {
	resp, err := c.we.QrcodeCheck(ctx, &weapi.QrcodeCheckReq{Key: key, Type: 1})
	if err != nil {
		return QRStatus{Key: key}, fmt.Errorf("netease: qr check: %w", err)
	}
	st := QRStatus{Key: key, Code: resp.Code}
	switch resp.Code {
	case 803:
		st.Status = "ok"
		c.mu.Lock()
		c.profile = nil
		c.urls = make(map[int64]cachedURL)
		delete(c.qr, key)
		c.mu.Unlock()
	case 802:
		st.Status = "scanned"
	case 801:
		st.Status = "waiting"
	case 800:
		st.Status = "expired"
		c.mu.Lock()
		delete(c.qr, key)
		c.mu.Unlock()
	default:
		st.Status = fmt.Sprintf("code_%d", resp.Code)
	}
	return st, nil
}
