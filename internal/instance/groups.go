package instance

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"

	"quartz/internal/core/jsonfile"
)

// Group is a named set of instances, in the order the user arranged them.
// An instance names its group; groups.json keeps the order and groups that
// are still empty.
type Group struct {
	Name string `json:"name"`
}

type groupsDoc struct {
	Groups []Group `json:"groups"`
}

// ErrGroupExists is returned when a name is already taken.
var ErrGroupExists = errors.New("instance: a group with that name already exists")

func openGroups(dir string, s *Store) (*jsonfile.Store[groupsDoc], error) {
	return jsonfile.Open(jsonfile.Options[groupsDoc]{
		Path:    filepath.Join(dir, "groups.json"),
		Default: func() groupsDoc { return groupsDoc{} },
		Normalize: func(d groupsDoc) groupsDoc {
			var out []Group
			for _, g := range d.Groups {
				if n, err := cleanGroup(g.Name); err == nil && n != "" && !slices.ContainsFunc(out, sameGroup(n)) {
					out = append(out, Group{Name: n})
				}
			}
			d.Groups = out
			return d
		},
		Log: s.log,
	})
}

func sameGroup(name string) func(Group) bool {
	return func(g Group) bool { return strings.EqualFold(g.Name, name) }
}

// Groups lists every group: the arranged ones first, then any an instance
// names that groups.json doesn't know (a hand-edited instance.json).
func (s *Store) Groups() []Group {
	out := slices.Clone(s.groups.Get().Groups)
	for _, in := range s.List() {
		if in.Group != "" && !slices.ContainsFunc(out, sameGroup(in.Group)) {
			out = append(out, Group{Name: in.Group})
		}
	}
	return out
}

// CreateGroup adds an empty group at the end.
func (s *Store) CreateGroup(name string) (Group, error) {
	n, err := cleanGroup(name)
	if err != nil || n == "" {
		return Group{}, ErrBadGroup
	}
	if slices.ContainsFunc(s.Groups(), sameGroup(n)) {
		return Group{}, ErrGroupExists
	}
	_, err = s.groups.Update(func(d *groupsDoc) error {
		d.Groups = append(d.Groups, Group{Name: n})
		return nil
	})
	return Group{Name: n}, err
}

// SetGroup moves an instance into a group ("" for none), creating it if new.
func (s *Store) SetGroup(id, group string) (Instance, error) {
	n, err := cleanGroup(group)
	if err != nil {
		return Instance{}, err
	}
	if n != "" {
		if i := slices.IndexFunc(s.Groups(), sameGroup(n)); i >= 0 {
			n = s.Groups()[i].Name // keep the existing spelling
		}
		s.ensureGroup(n)
	}
	return s.Update(id, func(in *Instance) error { in.Group = n; return nil })
}

// ensureGroup records a group in groups.json if only instances knew it.
func (s *Store) ensureGroup(name string) {
	if name == "" || slices.ContainsFunc(s.groups.Get().Groups, sameGroup(name)) {
		return
	}
	_, _ = s.groups.Update(func(d *groupsDoc) error {
		d.Groups = append(d.Groups, Group{Name: name})
		return nil
	})
}
