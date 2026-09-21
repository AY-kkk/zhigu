package finance

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	cninfoQueryURL = "http://www.cninfo.com.cn/new/hisAnnouncement/query"
	cninfoFileBase = "http://static.cninfo.com.cn/"
)

type cninfoQueryResp struct {
	Total         int `json:"totalAnnouncement"`
	Announcements []cninfoAnnouncement `json:"announcements"`
}

type cninfoAnnouncement struct {
	SecCode           string `json:"secCode"`
	SecName           string `json:"secName"`
	OrgID             string `json:"orgId"`
	AnnouncementID    string `json:"announcementId"`
	AnnouncementTitle string `json:"announcementTitle"`
	AnnouncementTime  int64  `json:"announcementTime"`
	AdjunctURL        string `json:"adjunctUrl"`
	AdjunctType       string `json:"adjunctType"`
}

func searchCninfo(ctx context.Context, httpc *CountedHTTP, inst ListedInstrument, query string, limit int) ([]cninfoAnnouncement, error) {
	if limit <= 0 || limit > 5 {
		limit = 5
	}
	form := url.Values{}
	form.Set("pageNum", "1")
	form.Set("pageSize", fmt.Sprintf("%d", limit))
	form.Set("column", inst.Column)
	form.Set("tabName", "fulltext")
	form.Set("plate", inst.Plate)
	form.Set("stock", inst.Symbol+","+inst.OrgID)
	form.Set("searchkey", strings.TrimSpace(query))
	form.Set("secid", "")
	form.Set("category", "")
	form.Set("trade", "")
	form.Set("seDate", "")
	form.Set("sortName", "")
	form.Set("sortType", "")
	form.Set("isHLtitle", "true")
	extra := http.Header{}
	extra.Set("Referer", "http://www.cninfo.com.cn/new/disclosure/stock")
	extra.Set("Origin", "http://www.cninfo.com.cn")
	extra.Set("X-Requested-With", "XMLHttpRequest")
	body, err := httpc.PostForm(ctx, cninfoQueryURL, form, extra)
	if err != nil {
		return nil, err
	}
	var resp cninfoQueryResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, NewError(503, "unavailable", "DATA_UNAVAILABLE", "巨潮检索无法解析")
	}
	if len(resp.Announcements) == 0 {
		return nil, NewError(503, "unavailable", "DATA_UNAVAILABLE", "巨潮检索无命中")
	}
	if len(resp.Announcements) > limit {
		resp.Announcements = resp.Announcements[:limit]
	}
	return resp.Announcements, nil
}

func stripHTMLTags(s string) string {
	s = strings.ReplaceAll(s, "<em>", "")
	s = strings.ReplaceAll(s, "</em>", "")
	return strings.TrimSpace(s)
}

func cninfoTime(ms int64) (time.Time, bool) {
	if ms <= 0 {
		return time.Time{}, false
	}
	// Values are Beijing calendar midnights expressed as epoch milliseconds.
	t := time.UnixMilli(ms).In(time.FixedZone("CST", 8*3600))
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	return day, true
}

func locatorFor(sourceURL, text string) string {
	n := utf8.RuneCountInString(text)
	if n > 1200 {
		n = 1200
	}
	return fmt.Sprintf("%s#char=0:%d", sourceURL, n)
}
