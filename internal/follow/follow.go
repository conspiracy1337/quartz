// Package follow offers instances that follow the latest Minecraft release a
// move to it once every mod has a build for it. It only offers: the switch is
// the player's, from the Version tab (decided in M5).
package follow

import (
	"context"
	"slices"
	"sort"
	"sync"

	"quartz/internal/content"
	"quartz/internal/core/events"
	"quartz/internal/instance"
)

// Offer is one instance that can move to the newest release.
type Offer struct {
	InstanceID string `json:"instanceId"`
	Name       string `json:"name"`
	Minecraft  string `json:"minecraft"` // the release it can move to
}

// Deps wires the checker.
type Deps struct {
	Instances func() []instance.Instance
	// Latest is the newest release and whether mc is an older release.
	Latest func(ctx context.Context, mc string) (latest string, behind bool, err error)
	// Compat counts the instance's mods with a build for mc.
	Compat  func(ctx context.Context, id, mc string) (content.Compat, error)
	Running func(id string) bool
	Bus     *events.Bus
}

// Checker is safe for concurrent use.
type Checker struct {
	d      Deps
	mu     sync.Mutex
	offers []Offer
}

// New returns a checker with no offers yet.
func New(d Deps) *Checker { return &Checker{d: d} }

// Offers are the moves the last check found, by instance name.
func (c *Checker) Offers() []Offer {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.offers)
}

// Check looks at every instance that follows the latest release. One that
// isn't running, is on an older release, and whose mods all have a build for
// the newest one (none unknown) gets an offer. An instance whose check fails
// (offline) keeps the offer it had.
func (c *Checker) Check(ctx context.Context) {
	old := c.Offers()
	var next []Offer
	for _, in := range c.d.Instances() {
		if !in.FollowLatest || (c.d.Running != nil && c.d.Running(in.ID)) {
			continue
		}
		latest, behind, err := c.d.Latest(ctx, in.Minecraft)
		if err != nil {
			next = append(next, kept(old, in.ID)...)
			continue
		}
		if !behind {
			continue
		}
		got, err := c.d.Compat(ctx, in.ID, latest)
		if err != nil {
			next = append(next, kept(old, in.ID)...)
			continue
		}
		if got.Matching == got.Total && got.Unknown == 0 {
			next = append(next, Offer{InstanceID: in.ID, Name: in.Name, Minecraft: latest})
		}
	}
	sort.Slice(next, func(i, j int) bool { return next[i].Name < next[j].Name })
	c.mu.Lock()
	changed := !slices.Equal(c.offers, next)
	c.offers = next
	c.mu.Unlock()
	if changed && c.d.Bus != nil {
		c.d.Bus.Emit(events.InstancesChanged, nil)
	}
}

func kept(old []Offer, id string) []Offer {
	for _, o := range old {
		if o.InstanceID == id {
			return []Offer{o}
		}
	}
	return nil
}
