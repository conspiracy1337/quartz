// Package auth chains the sign-in steps: a Microsoft token (package msa), then
// Xbox Live (package xbox), then Minecraft (package mcapi). Each step is
// small and ordered, like the launch pipeline, and reports its name so the
// sign-in dialog can show progress.
package auth

import (
	"context"
	"errors"
	"net/http"

	"quartz/internal/auth/msa"
	"quartz/internal/auth/xbox"
	"quartz/internal/core/netx"
	"quartz/internal/mcapi"
)

// Step names, shown in the sign-in dialog.
const (
	StepXbox      = "Signing in to Xbox Live"
	StepMinecraft = "Signing in to Minecraft"
	StepProfile   = "Loading your profile"
)

// Service signs in to Minecraft through a Microsoft account.
type Service struct {
	MSA  *msa.Client
	Xbox *xbox.Client
	MC   *mcapi.Client
}

// New wires the live endpoints. clientID "" leaves Microsoft sign-in off.
func New(h *netx.Client, clientID string) *Service {
	return &Service{MSA: msa.New(h, clientID), Xbox: xbox.New(h), MC: mcapi.New(h)}
}

// Configured reports whether this build can sign in with Microsoft.
func (s *Service) Configured() bool { return s.MSA.ClientID != "" }

// Result is everything a finished sign-in produces. MSA.Refresh is the
// long-lived secret; MC.Access is what the game gets.
type Result struct {
	MSA     msa.Token
	MC      mcapi.Token
	Profile mcapi.Profile
}

// Complete runs the steps after Microsoft: Xbox Live, XSTS, Minecraft
// sign-in, then the profile (and ownership when there is no profile).
func (s *Service) Complete(ctx context.Context, m msa.Token, step func(string)) (Result, error) {
	if step == nil {
		step = func(string) {}
	}
	step(StepXbox)
	x, err := s.Xbox.Authenticate(ctx, m.Access)
	if err != nil {
		return Result{}, err
	}
	step(StepMinecraft)
	mc, err := s.MC.LoginWithXbox(ctx, x.UHS, x.Token)
	if err != nil {
		return Result{}, err
	}
	step(StepProfile)
	p, err := s.MC.Profile(ctx, mc.Access)
	if err != nil {
		return Result{}, err
	}
	return Result{MSA: m, MC: mc, Profile: p}, nil
}

// Refresh renews every token from a stored Microsoft refresh token.
func (s *Service) Refresh(ctx context.Context, refresh string) (Result, error) {
	m, err := s.MSA.Refresh(ctx, refresh)
	if err != nil {
		return Result{}, err
	}
	return s.Complete(ctx, m, nil)
}

// NeedsReauth reports whether err means the stored sign-in is gone for good
// and only a new interactive sign-in helps. Everything else (network trouble,
// Microsoft or Mojang being down) keeps the stored tokens.
func NeedsReauth(err error) bool {
	var xe *xbox.Error
	return errors.Is(err, msa.ErrReauth) || errors.As(err, &xe) ||
		errors.Is(err, mcapi.ErrNotOwned) || errors.Is(err, mcapi.ErrNoProfile)
}

// Transient reports whether err is worth retrying later on its own.
func Transient(err error) bool {
	var oe *msa.OAuthError
	switch {
	case err == nil, NeedsReauth(err), errors.As(err, &oe),
		errors.Is(err, msa.ErrNotConfigured), errors.Is(err, msa.ErrDenied),
		errors.Is(err, msa.ErrExpired), errors.Is(err, msa.ErrTimeout):
		return false
	}
	cls, _ := netx.Classify(err)
	return cls != netx.Permanent
}

// Problem is a sign-in error as the user sees it.
type Problem struct {
	Reason  string `json:"reason"`
	Message string `json:"message"`
	FixURL  string `json:"fixUrl"`
}

// Describe turns any sign-in error into a message and, when one exists, a
// page that fixes it. It is the one place these texts live.
func Describe(err error) Problem {
	var (
		xe *xbox.Error
		se *netx.StatusError
	)
	switch {
	case errors.As(err, &xe):
		return Problem{Reason: xe.Reason, Message: xe.Message, FixURL: xe.FixURL}
	case errors.Is(err, msa.ErrNotConfigured):
		return Problem{Reason: "not-configured", Message: "Microsoft sign-in isn't set up in this build."}
	case errors.Is(err, msa.ErrReauth):
		return Problem{Reason: "reauth", Message: "Your sign-in expired. Sign in again."}
	case errors.Is(err, msa.ErrDenied), errors.Is(err, context.Canceled):
		return Problem{Reason: "cancelled", Message: "Sign-in was cancelled."}
	case errors.Is(err, msa.ErrExpired):
		return Problem{Reason: "expired", Message: "The code expired. Get a new one and try again."}
	case errors.Is(err, msa.ErrTimeout):
		return Problem{Reason: "timeout", Message: "Sign-in timed out. Try again."}
	case errors.Is(err, mcapi.ErrNotOwned):
		return Problem{Reason: "not-owned", Message: "This account doesn't own Minecraft: Java Edition.", FixURL: "https://www.minecraft.net/store/minecraft-java-bedrock-edition-pc"}
	case errors.Is(err, mcapi.ErrNoProfile):
		return Problem{Reason: "no-profile", Message: "Pick your Minecraft name at minecraft.net first.", FixURL: "https://www.minecraft.net/msaprofile/mygames/editprofile"}
	case errors.As(err, &se) && se.Code == http.StatusTooManyRequests:
		return Problem{Reason: "rate-limited", Message: "Too many sign-in attempts. Wait a minute and try again."}
	case Transient(err):
		return Problem{Reason: "network", Message: "Couldn't reach Microsoft or Mojang. Check your connection and try again."}
	default:
		return Problem{Reason: "failed", Message: "Sign-in failed. Try again."}
	}
}
