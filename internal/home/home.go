// Package home gathers what Home shows that isn't another service's list:
// this week's play time across every instance, the latest Java Edition
// article or release from the official launcher's feeds followed by the
// launcher's own posts, and whether the
// data folder's drive is running out of space.
package home

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"quartz/internal/brand"
	"quartz/internal/core/httpcache"
	"quartz/internal/core/netx"
	"quartz/internal/core/paths"
	"quartz/internal/core/remoteimg"
	"quartz/internal/core/sysinfo"
	"quartz/internal/instance"
	"quartz/internal/launch"
	"quartz/internal/meta"
)

// The official launcher's feeds. The news feed leaves releases and
// snapshots out; the Java patch notes list every one of them.
const (
	Feed       = "https://launchercontent.mojang.com/v2/news.json"
	PatchNotes = "https://launchercontent.mojang.com/v2/javaPatchNotes.json"
	// QuartzNews is the launcher's own posts, shown after Mojang's.
	QuartzNews = brand.WebAPI + "news"
)

const (
	feedHost = "https://launchercontent.mojang.com"
	// articleBase is where minecraft.net keeps a patch note's article.
	articleBase = "https://www.minecraft.net/en-us/article/"
	newsTTL     = time.Hour
	// maxPosts and maxTitle bound what the launcher's own feed can put in
	// the rotation.
	maxPosts = 5
	maxTitle = 200
	maxID    = 100 // fits settings' hidden news entries with the prefix
	// LowDisk is when the drive counts as nearly full.
	LowDisk = 5 << 30
)

// Day is one bar of "Played this week".
type Day struct {
	Day     int  `json:"day"` // 0 Monday … 6 Sunday
	Minutes int  `json:"minutes"`
	Today   bool `json:"today"`
}

// News is one article in Home's news strip.
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

	postsOnce sync.Once
	posts     []News
	postsURL  string // tests only
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

// patchNotes is the part of javaPatchNotes.json Home reads.
type patchNotes struct {
	Entries []struct {
		ID    string `json:"id"`
		Title string `json:"title"`
		Date  string `json:"date"`
		Image struct {
			URL string `json:"url"`
		} `json:"image"`
	} `json:"entries"`
}

// posts is the launcher's own news feed.
type posts struct {
	Entries []struct {
		ID        string `json:"id"`
		Title     string `json:"title"`
		Published string `json:"published"`
		URL       string `json:"url"`
		Image     string `json:"image"`
	} `json:"entries"`
}

// News is the newest Java Edition article or patch note, followed by the
// launcher's own posts, newest first. It fails only when nothing could be
// reached.
func (s *Service) News(ctx context.Context) ([]News, error) {
	var (
		f          feed
		notes      patchNotes
		own        []News
		ferr, nerr error
		wg         sync.WaitGroup
		policy     = httpcache.Policy{TTL: newsTTL}
	)
	wg.Go(func() { f, ferr = meta.Fetch[feed](ctx, s.d.Meta, Feed, policy) })
	wg.Go(func() { notes, nerr = meta.Fetch[patchNotes](ctx, s.d.Meta, PatchNotes, policy) })
	wg.Go(func() { own = s.Posts(ctx) })
	wg.Wait()
	var list []News
	if n := newer(pick(f, s.d.Pictures), pickNote(notes, s.d.Pictures)); n != nil {
		list = append(list, *n)
	}
	list = append(list, own...)
	if len(list) == 0 && ferr != nil && nerr != nil {
		return nil, ferr
	}
	return list, nil
}

// Posts is the launcher's own news. It is fetched once per run (the first
// call waits for it); a failure leaves it empty until the next start.
func (s *Service) Posts(ctx context.Context) []News {
	s.postsOnce.Do(func() {
		// Revalidated every start; the cached copy covers being offline.
		p, err := meta.Fetch[posts](ctx, s.d.Meta, cmp.Or(s.postsURL, QuartzNews), httpcache.Policy{MaxSize: 64 << 10})
		if err == nil {
			s.posts = pickPosts(p, s.d.Pictures)
		}
	})
	return s.posts
}

func newer(a, b *News) *News {
	if a == nil || (b != nil && b.Published > a.Published) {
		return b
	}
	return a
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
		best = newer(best, article(e.ID, e.Title, at, e.ReadMoreLink, e.NewsPageImage.URL, pics))
	}
	return best
}

// pickNote is the newest patch note. The feed has no links, but
// minecraft.net names each article after its title ("Minecraft 26.4
// Snapshot 4" is /article/minecraft-26-4-snapshot-4).
func pickNote(n patchNotes, pics *remoteimg.Cache) *News {
	var best *News
	for _, e := range n.Entries {
		at, err := time.Parse(time.RFC3339, e.Date)
		slug := articleSlug(e.Title)
		if err != nil || slug == "" {
			continue
		}
		best = newer(best, article(e.ID, e.Title, at, articleBase+slug, e.Image.URL, pics))
	}
	return best
}

// pickPosts keeps the launcher's own posts that have a title, a date and a
// safe link, newest first.
func pickPosts(p posts, pics *remoteimg.Cache) []News {
	var list []News
	for _, e := range p.Entries {
		at, err := time.Parse(time.RFC3339, e.Published)
		title := strings.TrimSpace(e.Title)
		if _, safe := netx.SafeHTTPS(e.URL); err != nil || e.ID == "" || len(e.ID) > maxID || title == "" || utf8.RuneCountInString(title) > maxTitle || !safe {
			continue
		}
		list = append(list, News{ID: "quartz:" + e.ID, Title: title, Source: brand.ShortName, Published: at.UnixMilli(), URL: e.URL, Image: pics.Src(e.Image)})
	}
	slices.SortStableFunc(list, func(a, b News) int { return cmp.Compare(b.Published, a.Published) })
	return list[:min(len(list), maxPosts)]
}

func article(id, title string, at time.Time, link, image string, pics *remoteimg.Cache) *News {
	n := &News{ID: id, Title: strings.TrimSpace(title), Source: "minecraft.net", Published: at.UnixMilli(), URL: link}
	if strings.HasPrefix(image, "/") && pics != nil {
		n.Image = pics.Src(feedHost + image)
	}
	return n
}

// articleSlug turns a patch note title into its article's name: lower case
// letters and digits joined by dashes, always starting with "minecraft-"
// (a few titles leave it out). "" when the title has nothing to use.
func articleSlug(title string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(title) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(r)
			dash = false
		} else {
			dash = true
		}
	}
	slug := b.String()
	if slug == "" || slug == "minecraft" || len(slug) > 100 {
		return ""
	}
	if !strings.HasPrefix(slug, "minecraft-") {
		slug = "minecraft-" + slug
	}
	return slug
}

// articleURL allows only https links on minecraft.net, which the UI opens
// in the browser.
func articleURL(raw string) bool {
	u, ok := netx.SafeHTTPS(raw)
	if !ok {
		return false
	}
	h := strings.ToLower(u.Hostname())
	return h == "minecraft.net" || h == "www.minecraft.net"
}

// FolderProblem names what is wrong with where the data folder is ("bang",
// "temp"), or "" (see paths.Problem).
func (s *Service) FolderProblem() string { return paths.Problem(s.d.DataDir) }

// Disk reports the free space on the data folder's drive.
func (s *Service) Disk() Disk {
	free, err := sysinfo.DiskFree(s.d.DataDir)
	if err != nil {
		return Disk{Free: -1}
	}
	n := int64(free) //nolint:gosec // free bytes fit an int64
	return Disk{Free: n, Low: free < LowDisk}
}
