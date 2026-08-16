package douyin

// account_sdk_source_info — companion account SDK (18610):
//
//	browserInfo = await P({useBitReport})
//	params.account_sdk_source_info = c(JSON.stringify(browserInfo))
//	c(s) = lowercase hex( each UTF-8 byte XOR 0x05 )
//
// Field set mirrors P()'s return object (Wine Electron login page shape).
// Pure browser APIs → stable Electron-like defaults; cpu/geometry from host.

import (
	"encoding/json"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const sdkInfoXOR byte = 0x05

// companion login page path shape (request_pathname / performance.name).
const companionLoginPath = "/C:/Program%20Files%20(x86)/webcast_mate/" + appVersion +
	".0/resources/app/app.content/pages/login/index.html"

var (
	sdkInfoOnce   sync.Once
	sdkInfoCached string
)

// accountSDKSourceInfo returns c(JSON.stringify(browserInfo)).
// Cached once per process (companion keeps this.browserInfo after first P()).
func accountSDKSourceInfo() string {
	sdkInfoOnce.Do(func() {
		sdkInfoCached = encodeAccountSDKSourceInfo(buildBrowserInfo())
	})
	return sdkInfoCached
}

func encodeAccountSDKSourceInfo(info any) string {
	b, err := json.Marshal(info)
	if err != nil {
		return ""
	}
	const hexdigits = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, c := range b {
		x := c ^ sdkInfoXOR
		out[i*2] = hexdigits[x>>4]
		out[i*2+1] = hexdigits[x&0xf]
	}
	return string(out)
}

// Wire shape — json tags match companion keys (incl. stoargeStatus typo).

type browserInfo struct {
	HardwareConcurrency int                  `json:"hardwareConcurrency"`
	Webdriver           bool                 `json:"webdriver"`
	Chromedriver        bool                 `json:"chromedriver"`
	Shelldriver         bool                 `json:"shelldriver"`
	Plugins             int                  `json:"plugins"`
	Permissions         []browserPermission  `json:"permissions"`
	InnerHeight         int                  `json:"innerHeight"`
	InnerWidth          int                  `json:"innerWidth"`
	OuterHeight         int                  `json:"outerHeight"`
	OuterWidth          int                  `json:"outerWidth"`
	StoargeStatus       browserStorageStatus `json:"stoargeStatus"`
	WebGL               map[string]any       `json:"webgl"`
	NotificationPerm    string               `json:"notificationPermission"`
	Performance         browserPerf          `json:"performance"`
	RequestHost         string               `json:"request_host"`
	RequestPathname     string               `json:"request_pathname"`
	Browser             browserMeta          `json:"browser"`
}

type browserPermission struct {
	Name  string `json:"name"`
	State string `json:"state"`
}

type browserStorageStatus struct {
	IndexedDB          browserIDB   `json:"indexedDB"`
	LocalStorage       browserLS    `json:"localStorage"`
	StorageQuotaStatus browserQuota `json:"storageQuotaStatus"`
}

type browserIDB struct {
	IDB          string `json:"idb"`
	Open         string `json:"open"`
	IndexedDB    string `json:"indexedDB"`
	IDBKeyRange  string `json:"IDBKeyRange"`
	OpenDatabase string `json:"openDatabase"`
	IsSafari     bool   `json:"isSafari"`
	HasFetch     bool   `json:"hasFetch"`
}

type browserLS struct {
	IsSupportLStorage bool `json:"isSupportLStorage"`
	Size              int  `json:"size"`
	Write             bool `json:"write"`
}

type browserQuota struct {
	Usage     int64 `json:"usage"`
	Quota     int64 `json:"quota"`
	IsPrivate bool  `json:"isPrivate"`
}

type browserPerf struct {
	TimeOrigin       float64          `json:"timeOrigin"`
	UsedJSHeapSize   int64            `json:"usedJSHeapSize"`
	NavigationTiming browserNavTiming `json:"navigationTiming"`
}

type browserNavTiming struct {
	DecodedBodySize      int    `json:"decodedBodySize"`
	EntryType            string `json:"entryType"`
	InitiatorType        string `json:"initiatorType"`
	Name                 string `json:"name"`
	RenderBlockingStatus string `json:"renderBlockingStatus"`
	ServerTiming         string `json:"serverTiming"`
	GuleStart            string `json:"guleStart"`
	GuleDuration         string `json:"guleDuration"`
}

type browserMeta struct {
	T           string `json:"t"`
	BitProtocol string `json:"bit_protocol"`
	BitHelper   bool   `json:"bit_helper"`
}

func buildBrowserInfo() browserInfo {
	h := localHostFingerprint()
	w, ht := parseRes(h.Resolution)
	if w == 0 {
		w = 1200
	}
	if ht == 0 {
		ht = 800
	}
	innerW, innerH := w, ht
	if innerW > 1200 {
		innerW = 1200
	}
	if innerH > 800 {
		innerH = 800
	}

	now := time.Now()
	// browser.t = String(Date.now()).split("").reverse().join("")
	tRev := reverseASCII(strconv.FormatInt(now.UnixMilli(), 10))
	path := companionLoginPath

	return browserInfo{
		HardwareConcurrency: runtime.NumCPU(),
		Webdriver:           false,
		Chromedriver:        false,
		Shelldriver:         false,
		Plugins:             5,
		Permissions: []browserPermission{
			{Name: "notifications", State: "granted"},
		},
		InnerHeight: innerH,
		InnerWidth:  innerW,
		OuterHeight: ht,
		OuterWidth:  w,
		StoargeStatus: browserStorageStatus{
			IndexedDB: browserIDB{
				IDB: "object", Open: "function", IndexedDB: "object",
				IDBKeyRange: "function", OpenDatabase: "undefined",
			},
			LocalStorage: browserLS{IsSupportLStorage: true, Size: 387, Write: true},
			StorageQuotaStatus: browserQuota{
				Usage: 0, Quota: 64 << 30, IsPrivate: false,
			},
		},
		WebGL:            map[string]any{},
		NotificationPerm: "granted",
		Performance: browserPerf{
			TimeOrigin:     float64(now.UnixMilli()),
			UsedJSHeapSize: 50_400_000,
			NavigationTiming: browserNavTiming{
				DecodedBodySize:      103704,
				EntryType:            "navigation",
				InitiatorType:        "navigation",
				Name:                 "file://" + path,
				RenderBlockingStatus: "non-blocking",
				ServerTiming:         "",
				GuleStart:            "none",
				GuleDuration:         "none",
			},
		},
		RequestHost:     "",
		RequestPathname: path,
		Browser: browserMeta{
			T:           tRev,
			BitProtocol: "false",
			BitHelper:   false,
		},
	}
}

func parseRes(res string) (w, h int) {
	res = strings.ToLower(strings.TrimSpace(res))
	res = strings.ReplaceAll(res, "*", "x")
	a, b, ok := strings.Cut(res, "x")
	if !ok {
		return 0, 0
	}
	w, _ = strconv.Atoi(strings.TrimSpace(a))
	h, _ = strconv.Atoi(strings.TrimSpace(b))
	return w, h
}

func reverseASCII(s string) string {
	b := []byte(s)
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
	return string(b)
}
