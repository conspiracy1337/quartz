// Package network keeps the shared HTTP client in step with the download
// settings and the proxy password in the keychain.
package network

import (
	"log/slog"
	"sync"

	"quartz/internal/core/logging"
	"quartz/internal/core/netx"
	"quartz/internal/core/secrets"
	"quartz/internal/settings"
)

const passwordKey = "proxy-password"

// Service owns the proxy configuration of one netx.Client.
type Service struct {
	client  *netx.Client
	secrets secrets.Store
	red     *logging.Redactor
	log     *slog.Logger

	mu       sync.Mutex
	current  settings.Downloads
	password string
}

// New configures client from d and returns the service.
func New(client *netx.Client, d settings.Downloads, s secrets.Store, red *logging.Redactor, log *slog.Logger) *Service {
	svc := &Service{client: client, secrets: s, red: red, log: log}
	if pw, err := s.Get(passwordKey); err == nil {
		svc.password = pw
		red.AddSecret(pw)
	}
	svc.Apply(d)
	return svc
}

// Proxy converts settings into a netx proxy. An incomplete custom proxy falls
// back to the system proxy rather than breaking every request.
func Proxy(d settings.Downloads, password string) netx.Proxy {
	switch {
	case d.Proxy == "none":
		return netx.Proxy{Mode: netx.ProxyNone}
	case d.Proxy == "custom" && d.ProxyCustom.Usable():
		c := d.ProxyCustom
		return netx.Proxy{Mode: netx.ProxyCustom, Type: c.Type, Host: c.Host, Port: c.Port, User: c.User, Password: password}
	default:
		return netx.Proxy{Mode: netx.ProxySystem}
	}
}

// Apply reconfigures the client when the proxy settings changed.
func (s *Service) Apply(d settings.Downloads) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if d.Proxy == s.current.Proxy && d.ProxyCustom == s.current.ProxyCustom && s.current.Proxy != "" {
		return
	}
	s.current = d
	s.configure()
}

// configure applies the current settings; s.mu must be held.
func (s *Service) configure() {
	if err := s.client.Configure(Proxy(s.current, s.password)); err != nil {
		s.log.Error("proxy configuration rejected, using the system proxy", "err", err)
		_ = s.client.Configure(netx.Proxy{Mode: netx.ProxySystem})
	}
}

// SetPassword stores the proxy password in the keychain (empty deletes it)
// and applies it at once.
func (s *Service) SetPassword(pw string) error {
	var err error
	if pw == "" {
		err = s.secrets.Delete(passwordKey)
	} else {
		err = s.secrets.Set(passwordKey, pw)
	}
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.password != "" {
		s.red.RemoveSecret(s.password)
	}
	s.password = pw
	s.red.AddSecret(pw)
	s.configure()
	return nil
}

// HasPassword reports whether a proxy password is stored. The value itself
// never leaves this package.
func (s *Service) HasPassword() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.password != ""
}
