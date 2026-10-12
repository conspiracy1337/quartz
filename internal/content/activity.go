package content

import (
	"path/filepath"
	"time"

	"quartz/internal/core/jsonfile"
)

// Activity is one content change for the Overview feed.
type Activity struct {
	At     int64  `json:"at"` // unix seconds
	Kind   string `json:"kind"`
	Title  string `json:"title"`
	Detail string `json:"detail,omitempty"`
}

const maxActivity = 100

func (s *Service) activityStore(id string) (*jsonfile.Store[[]Activity], error) {
	return jsonfile.Open(jsonfile.Options[[]Activity]{
		Path: filepath.Join(s.d.Instances.Dir(id), "activity.json"), Default: func() []Activity { return nil },
		Delay: 100 * time.Millisecond, Log: s.d.Log,
	})
}

// record adds a change to an instance's activity log.
func (s *Service) record(id string, a Activity) {
	st, err := s.activityStore(id)
	if err != nil {
		return
	}
	if a.At == 0 {
		a.At = s.now().Unix()
	}
	_, _ = st.Update(func(list *[]Activity) error {
		*list = append([]Activity{a}, *list...)
		if len(*list) > maxActivity {
			*list = (*list)[:maxActivity]
		}
		return nil
	})
	_ = st.Flush()
}

// Activity lists an instance's content changes, newest first.
func (s *Service) Activity(id string) []Activity {
	st, err := s.activityStore(id)
	if err != nil {
		return nil
	}
	return st.Get()
}

// Record adds an entry to an instance's activity feed (for other services,
// such as modpack updates).
func (s *Service) Record(id string, a Activity) { s.record(id, a) }
