// Command release builds and signs Quartz Client releases (development
// only; never part of the launcher):
//
//	go run ./tools/release keygen                       make the signing key pair
//	go run ./tools/release build 1.0.0 [targets…]       build the release files into dist/
//	go run ./tools/release manifest 1.0.0 notes.md      write and sign dist/update.json
//	go run ./tools/release sign <file>                  sign any file (file.minisig)
//	go run ./tools/release verify <file>…               check built programs (tags, size)
//
// Targets default to what this computer can build: linux-amd64,
// windows-amd64 and windows-arm64 on Linux; darwin-universal on a Mac.
// Secrets and IDs come from the environment, never from the repository:
// QUARTZ_MSA_CLIENT_ID, QUARTZ_CF_API_KEY, QUARTZ_DISCORD_CLIENT_ID, and
// QUARTZ_RELEASE_PASSPHRASE (else it is asked for). A release refuses to
// build without the Microsoft and CurseForge IDs unless given
// -allow-missing. The key pair lives in
// ~/.config/quartz-release (QUARTZ_RELEASE_KEYS moves it).
// Publishing is a separate step you run yourself:
//
//	gh release create v1.0.0 dist/* --repo conspiracy1337/quartz
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
)

const usage = `usage:
  release keygen
  release build [-allow-missing] <version> [linux-amd64 linux-arm64 windows-amd64 windows-arm64 darwin-universal]
  release manifest <version> <notes.md>
  release sign <file>
  release verify <file>…
`

func main() {
	// Ctrl+C stops the build tools and still runs the deferred clean-ups
	// (wails.json gets its version back).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	err := run(ctx, os.Args[1:])
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, "release:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no command\n%s", usage)
	}
	switch args[0] {
	case "keygen":
		return keygen()
	case "build":
		args = args[1:]
		allowMissing := len(args) > 0 && args[0] == "-allow-missing"
		if allowMissing {
			args = args[1:]
		}
		if len(args) == 0 {
			return fmt.Errorf("build needs a version\n%s", usage)
		}
		return build(ctx, args[0], args[1:], allowMissing)
	case "manifest":
		if len(args) != 3 {
			return fmt.Errorf("manifest needs a version and a notes file\n%s", usage)
		}
		return manifest(args[1], args[2])
	case "sign":
		if len(args) != 2 {
			return fmt.Errorf("sign needs a file\n%s", usage)
		}
		return signFile(args[1])
	case "verify":
		if len(args) < 2 {
			return fmt.Errorf("verify needs a file\n%s", usage)
		}
		for _, f := range args[1:] {
			summary, err := verifyArtifact(f)
			if err != nil {
				return err
			}
			fmt.Println(summary)
		}
		return nil
	}
	return fmt.Errorf("unknown command %q\n%s", args[0], usage)
}
