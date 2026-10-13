// Package home gathers what Home shows that isn't another service's list:
// this week's play time across every instance, the latest Java Edition
// article from the official launcher's news feed, and whether the data
// folder's drive is running out of space.
package home

import (
	"context"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"quartz/internal/core/httpcache"
	"quartz/internal/core/remoteimg"
	"quartz/internal/core/sysinfo"
	"quartz/internal/instance"
	"quartz/internal/launch"
	"quartz/internal/meta"
)

// Feed is the official launcher's news feed.
const Feed = "https://launchercontent.mojang.com/v2/news.json"

const (
	feedHost = "https://launchercontent.mojang.com"
	newsTTL  = time.Hour
	// LowDisk is when the drive counts as nearly full.
	LowDisk = 5 << 30
)

// Day is one bar of "Played this week".
type Day struct {
	Day     int  `json:"day"` // 0 Monday … 6 Sunday
	Minutes int  `json:"minutes"`
	Today   bool `json:"today"`
}

// News is the latest article.
type News struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Source    string `json:"source"`
	Published int64  `json:"published"`
	URL       string `json:"url"`
	Image     string `json:"image,omitempty"`
}

// Disk is the free space where the launcher keeps its data.
type Disk struct {
	Free int64 `json:"free"`
	Low  bool  `json:"low"`
}

// Deps wires a Service.
type Deps struct {
	Instances *instance.Store
	Runs      func(id string) []launch.Record
	Meta      *meta.Service
	Pictures  *remoteimg.Cache
	DataDir   string
	Now       func() time.Time
}

// Service is safe for concurrent use.
type Service struct {
	d Deps

	mu   sync.Mutex
	week []Day
	key  string // the day the cached week was counted on
}

// New returns a Service.
func New(d Deps) *Service {
	if d.Now == nil {
		d.Now = time.Now
	}
	return &Service{d: d}
}

// Invalidate drops the counted week (a game just exited).
func (s *Service) Invalidate() {
	s.mu.Lock()
	s.week = nil
	s.mu.Unlock()
}

// Week is the minutes played on each day of this calendar week (Monday to
// Sunday, local time), over every instance. A session that crosses midnight
// counts on both days.
func (s *Service) Week() []Day {
	now := s.d.Now()
	key := now.Format(time.DateOnly)
	s.mu.Lock()
	if s.week != nil && s.key == key {
		w := slices.Clone(s.week)
		s.mu.Unlock()
		return w
	}
	s.mu.Unlock()
	var records []launch.Record
	for _, in := range s.d.Instances.List() {
		records = append(records, s.d.Runs(in.ID)...)
	}
	w := Count(records, now)
	s.mu.Lock()
	s.week, s.key = w, key
	s.mu.Unlock()
	return slices.Clone(w)
}

// Count buckets runs into the calendar week that holds now.
func Count(records []launch.Record, now time.Time) []Day {
	loc := now.Location()
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	today := (int(now.Weekday()) + 6) % 7 // Monday is 0
	monday := midnight.AddDate(0, 0, -today)
	days := make([]Day, 7)
	var secs [7]float64
	for i := range days {
		days[i] = Day{Day: i, Today: i == today}
	}
	for _, r := range records {
		start := time.UnixMilli(r.StartedAt).In(loc)
		end := start.Add(time.Duration(r.LengthMin) * time.Minute)
		for i := range 7 {
			from := monday.AddDate(0, 0, i)
			to := monday.AddDate(0, 0, i+1)
			if overlap := earlier(end, to).Sub(later(start, from)); overlap > 0 {
				secs[i] += overlap.Seconds()
			}
		}
	}
	for i := range days {
		days[i].Minutes = int(secs[i]/60 + 0.5)
	}
	return days
}

func earlier(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func later(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

// feed is the part of news.json Home reads.
type feed struct {
	Entries []struct {
		ID            string   `json:"id"`
		Title         string   `json:"title"`
		Category      string   `json:"category"`
		Date          string   `json:"date"`
		ReadMoreLink  string   `json:"readMoreLink"`
		NewsType      []string `json:"newsType"`
		NewsPageImage struct {
			URL string `json:"url"`
		} `json:"newsPageImage"`
	} `json:"entries"`
}

// News returns the newest Java Edition article, or nil when there is none
// (or the feed can't be reached).
func (s *Service) News(ctx context.Context) (*News, error) {
	f, err := meta.Fetch[feed](ctx, s.d.Meta, Feed, httpcache.Policy{TTL: newsTTL})
	if err != nil {
		return nil, err
	}
	return pick(f, s.d.Pictures), nil
}

func pick(f feed, pics *remoteimg.Cache) *News {
	var best *News
	for _, e := range f.Entries {
		if !slices.Contains(e.NewsType, "Java") && !strings.Contains(e.Category, "Java Edition") {
			continue
		}
		at, err := time.Parse(time.DateOnly, e.Date)
		if err != nil || strings.TrimSpace(e.Title) == "" || !articleURL(e.ReadMoreLink) {
			continue
		}
		if best != nil && at.UnixMilli() <= best.Published {
			continue
		}
		n := &News{ID: e.ID, Title: strings.TrimSpace(e.Title), Source: "minecraft.net", Published: at.UnixMilli(), URL: e.ReadMoreLink}
		if strings.HasPrefix(e.NewsPageImage.URL, "/") && pics != nil {
			n.Image = pics.Src(feedHost + e.NewsPageImage.URL)
		}
		best = n
	}
	return best
}

// articleURL allows only https links on minecraft.net, which the UI opens
// in the browser.
func articleURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil {
		return false
	}
	h := strings.ToLower(u.Hostname())
	return h == "minecraft.net" || h == "www.minecraft.net"
}

// Disk reports the free space on the data folder's drive.
func (s *Service) Disk() Disk {
	free, err := sysinfo.DiskFree(s.d.DataDir)
	if err != nil {
		return Disk{Free: -1}
	}
	n := int64(free) //nolint:gosec // free bytes fit an int64
	return Disk{Free: n, Low: free < LowDisk}
}
