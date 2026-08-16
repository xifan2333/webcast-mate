package bilibili

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/xifan2333/webcast-mate/internal/adapter"
	"github.com/xifan2333/webcast-mate/internal/adapter/httpx"
	"github.com/xifan2333/webcast-mate/internal/secrets"
)

const (
	ua = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 Safari/537.36"

	urlQRGenerate   = "https://passport.bilibili.com/x/passport-login/web/qrcode/generate"
	urlQRPoll       = "https://passport.bilibili.com/x/passport-login/web/qrcode/poll"
	urlNav          = "https://api.bilibili.com/x/web-interface/nav"
	urlStartLive    = "https://api.live.bilibili.com/room/v1/Room/startLive"
	urlStopLive     = "https://api.live.bilibili.com/room/v1/Room/stopLive"
	urlUpdateRoom   = "https://api.live.bilibili.com/room/v1/Room/update"
	urlFaceAuth     = "https://api.live.bilibili.com/xlive/app-blink/v1/preLive/IsUserIdentifiedByFaceAuth"
	urlRoomInfo     = "https://api.live.bilibili.com/room/v1/Room/get_info"
	urlBlinkGetInfo = "https://api.live.bilibili.com/xlive/app-blink/v1/room/GetInfo?platform=pc"
)

var (
	// biliSetHosts binds cookies onto these hosts.
	biliSetHosts = []string{
		"https://passport.bilibili.com",
		"https://api.bilibili.com",
		"https://api.live.bilibili.com",
		"https://bilibili.com",
	}
	// biliReadHosts reads cookies back from these hosts.
	biliReadHosts = []string{
		"https://api.bilibili.com",
		"https://passport.bilibili.com",
	}
)

// Client is the bilibili passport/live HTTP client.
type Client struct {
	*httpx.Client
}

func NewClient() *Client {
	return &Client{Client: httpx.New()}
}

func (c *Client) ApplySecrets(f *secrets.File) {
	if f == nil {
		return
	}
	f.Normalize()
	c.SetCookieHeader(f.CookieHeader(), ".bilibili.com", biliSetHosts...)
}

func (c *Client) ExportSecrets(userID, userName string, loginAt time.Time) *secrets.File {
	f := &secrets.File{Version: secrets.Version, UserID: userID, UserName: userName, LoginAt: loginAt,
		Cookies: secrets.ParseCookieHeader(c.cookieString()), Headers: map[string]string{}, Params: map[string]string{}}
	f.Normalize()
	return f
}

func (c *Client) cookieString() string {
	return c.CookieString(biliReadHosts, nil)
}

func (c *Client) csrf() string {
	return c.CookieValue(biliReadHosts, []string{"bili_jct"})
}

func (c *Client) doJSON(method, rawURL string, form url.Values, headers map[string]string) ([]byte, error) {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequest(method, rawURL, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	}
	req.Header.Set("Origin", "https://link.bilibili.com")
	req.Header.Set("Referer", "https://link.bilibili.com/p/center/index")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", adapter.ErrNetwork, err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return b, fmt.Errorf("%w: http %d", adapter.ErrNetwork, resp.StatusCode)
	}
	return b, nil
}
