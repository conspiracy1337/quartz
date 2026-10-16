package modplatform

import (
	"context"
	"time"
)

// ProjectType is a kind of project both platforms list.
type ProjectType string

const (
	TypeMod          ProjectType = "mod"
	TypeModpack      ProjectType = "modpack"
	TypeResourcePack ProjectType = "resourcepack"
	TypeShader       ProjectType = "shader"
	TypeDataPack     ProjectType = "datapack"
	TypeWorld        ProjectType = "world" // CurseForge only: Modrinth hosts no worlds
)

// Types lists every project type, in the order Browse shows them.
var Types = []ProjectType{TypeMod, TypeModpack, TypeResourcePack, TypeShader, TypeDataPack, TypeWorld}

// Valid reports whether t is a known type.
func (t ProjectType) Valid() bool {
	for _, v := range Types {
		if v == t {
			return true
		}
	}
	return false
}

// Sort orders search results.
type Sort string

const (
	SortRelevance Sort = "relevance"
	SortDownloads Sort = "downloads"
	SortUpdated   Sort = "updated"
)

// SearchQuery is one page of a search on one platform.
type SearchQuery struct {
	Type    ProjectType
	Text    string
	Sort    Sort
	Game    string   // one Minecraft version, or "" for any
	Loaders []string // any of these; empty for any
	// Categories are groups of the platform's own ids (Modrinth slugs,
	// CurseForge numeric ids): a project must match one id of every group.
	Categories [][]string
	// Side keeps projects that run on one side only: SideClient or
	// SideServer ("" for any). Modrinth only; CurseForge publishes no sides.
	Side string
	// Exclude lists Modrinth content disclosures to hide (ignored by
	// CurseForge, which publishes none).
	Exclude []string
	Offset  int
	Limit   int
}

// Hit is one search result.
type Hit struct {
	Source    Source    `json:"source"`
	ID        string    `json:"id"`
	Slug      string    `json:"slug"`
	Title     string    `json:"title"`
	Author    string    `json:"author"`
	Summary   string    `json:"summary"`
	Icon      string    `json:"icon"` // image URL on the platform CDN, or ""
	Downloads int64     `json:"downloads"`
	Updated   time.Time `json:"updated"`
}

// SearchPage is one page of hits and how many there are in all.
type SearchPage struct {
	Hits  []Hit
	Total int
	// Read is how many results the page used up when the platform's answer
	// was filtered further (0: as many as Hits); the next page starts after it.
	Read int
}

// Used is how far the next page's offset moves.
func (p SearchPage) Used() int {
	return max(p.Read, len(p.Hits))
}

// Search sides.
const (
	SideClient = "client"
	SideServer = "server"
)

// Image is a gallery picture.
type Image struct {
	URL   string `json:"url"`
	Title string `json:"title"`
}

// Links are a project's pages elsewhere.
type Links struct {
	Page    string `json:"page"` // the project on the platform's website
	Source  string `json:"source,omitempty"`
	Issues  string `json:"issues,omitempty"`
	Wiki    string `json:"wiki,omitempty"`
	Website string `json:"website,omitempty"`
}

// Detail is everything a project page shows from one platform.
type Detail struct {
	Project
	Type      ProjectType
	Summary   string
	Body      string // Markdown on Modrinth, HTML on CurseForge
	BodyHTML  bool
	Icon      string
	License   string
	Loaders   []string
	Game      []string
	Client    string // required | optional | unsupported | "" (unknown)
	Server    string
	Downloads int64
	Updated   time.Time
	Gallery   []Image
	Links     Links
	Versions  int // how many versions exist, 0 when unknown
	// Distributable is false when the author blocks downloads outside the
	// platform's own website (CurseForge).
	Distributable bool
}

// Category is one of a platform's project categories.
type Category struct {
	ID   string // Modrinth: the slug; CurseForge: the numeric id
	Slug string
	Name string
}

// Catalog is the browsing side of a platform.
type Catalog interface {
	Source() Source
	Ready() bool
	Search(ctx context.Context, q SearchQuery) (SearchPage, error)
	// Project reads one project by id (or Modrinth slug).
	Project(ctx context.Context, id string) (Detail, error)
	// Slug finds a project's id from its website slug.
	Slug(ctx context.Context, t ProjectType, slug string) (string, error)
	// Version reads one version (CurseForge: one file) of a project.
	Version(ctx context.Context, project, id string) (Version, error)
	// Files reads many versions by id at once, keyed by id.
	Files(ctx context.Context, ids []string) (map[string]Version, error)
	Categories(ctx context.Context, t ProjectType) ([]Category, error)
}

// Platform is a provider with both sides.
type Platform interface {
	Provider
	Catalog
}

// VersionListCap is the JSON size cap for a project's full version list.
const VersionListCap = 16 << 20
