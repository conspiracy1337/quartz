package accounts

import (
	"context"
	"errors"

	"rsc.io/qr"

	"quartz/internal/auth"
	"quartz/internal/auth/msa"
	"quartz/internal/core/events"
)

// SignInEvent is what accounts:signin carries: the progress of the one
// interactive sign-in. Flow numbers each sign-in, so the dialog can ignore
// events of one it already left. It never carries a token.
type SignInEvent struct {
	Flow      int64         `json:"flow"`
	Mode      string        `json:"mode"`           // browser | code
	Step      string        `json:"step,omitempty"` // "" while waiting for the user
	Code      string        `json:"code,omitempty"` // the device code to type
	URL       string        `json:"url,omitempty"`  // where to type it
	QR        [][]bool      `json:"qr,omitempty"`   // dark modules of the QR code
	ExpiresAt int64         `json:"expiresAt,omitempty"`
	Done      bool          `json:"done"`
	AccountID string        `json:"accountId,omitempty"`
	Error     *auth.Problem `json:"error,omitempty"`
}

type flow struct {
	id     int64
	mode   string
	cancel context.CancelFunc
}

// begin starts a new interactive sign-in and cancels the previous one: the
// dialog shows one at a time.
func (m *Manager) begin(mode string) (*flow, context.Context) {
	ctx, cancel := context.WithCancel(context.Background())
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.signin != nil {
		m.signin.cancel()
	}
	m.nextID++
	f := &flow{id: m.nextID, mode: mode, cancel: cancel}
	m.signin = f
	return f, ctx
}

// CancelSignIn stops the interactive sign-in, if any.
func (m *Manager) CancelSignIn() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.signin != nil {
		m.signin.cancel()
		m.signin = nil
	}
}

func (m *Manager) emit(f *flow, e SignInEvent) {
	e.Flow, e.Mode = f.id, f.mode
	m.cfg.Bus.Emit(events.AccountsSignIn, e)
}

// end reports the outcome and forgets the flow.
func (m *Manager) end(ctx context.Context, f *flow, step func(string), m2 msa.Token, err error) {
	var a Account
	if err == nil {
		var res auth.Result
		res, err = m.cfg.Auth.Complete(ctx, m2, step)
		if err == nil {
			a, err = m.save(ctx, res)
		}
	}
	m.mu.Lock()
	if m.signin == f {
		m.signin.cancel()
		m.signin = nil
	}
	m.mu.Unlock()
	if errors.Is(err, context.Canceled) {
		return // the dialog closed or started another sign-in
	}
	if err != nil {
		m.cfg.Log.Info("sign-in failed", "mode", f.mode, "err", err)
		p := auth.Describe(err)
		m.emit(f, SignInEvent{Done: true, Error: &p})
		return
	}
	m.cfg.Log.Info("signed in", "account", a.Name)
	m.emit(f, SignInEvent{Done: true, AccountID: a.ID})
}

func (m *Manager) stepper(f *flow) func(string) {
	return func(s string) { m.emit(f, SignInEvent{Step: s}) }
}

// SignInBrowser opens Microsoft's sign-in page with open (the system
// browser) and finishes in the background. It returns the flow number.
func (m *Manager) SignInBrowser(open func(url string) error) (int64, error) {
	b, err := m.cfg.Auth.MSA.StartBrowser()
	if err != nil {
		return 0, err
	}
	if err := open(b.URL()); err != nil {
		b.Close()
		return 0, err
	}
	f, ctx := m.begin("browser")
	go func() {
		tok, err := b.Wait(ctx)
		if err == nil {
			m.cfg.Focus()
		}
		m.end(ctx, f, m.stepper(f), tok, err)
	}()
	return f.id, nil
}

// SignInCode starts a device code sign-in. The code and its QR code arrive
// as an accounts:signin event; the rest finishes in the background.
func (m *Manager) SignInCode() (int64, error) {
	if !m.Configured() {
		return 0, msa.ErrNotConfigured
	}
	f, ctx := m.begin("code")
	go func() {
		dc, err := m.cfg.Auth.MSA.StartDevice(ctx)
		if err != nil {
			m.end(ctx, f, nil, msa.Token{}, err)
			return
		}
		m.emit(f, SignInEvent{Code: dc.UserCode, URL: dc.VerificationURI, QR: qrMatrix(dc.QRURL), ExpiresAt: dc.ExpiresAt.UnixMilli()})
		tok, err := m.cfg.Auth.MSA.PollDevice(ctx, dc)
		m.end(ctx, f, m.stepper(f), tok, err)
	}()
	return f.id, nil
}

// qrMatrix encodes text as QR modules, nil if it can't (the dialog then shows
// only the code).
func qrMatrix(text string) [][]bool {
	c, err := qr.Encode(text, qr.M)
	if err != nil {
		return nil
	}
	out := make([][]bool, c.Size)
	for y := range c.Size {
		out[y] = make([]bool, c.Size)
		for x := range c.Size {
			out[y][x] = c.Black(x, y)
		}
	}
	return out
}
