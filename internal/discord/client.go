// Package discord shows what the player is playing in Discord (Rich
// Presence). It speaks Discord's local IPC protocol directly: a Unix socket
// or a Windows named pipe carrying frames of a little-endian opcode and
// length followed by JSON. No library, no cgo.
package discord

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
)

// Frame opcodes.
const (
	opHandshake = 0
	opFrame     = 1
	opClose     = 2
	opPing      = 3
	opPong      = 4
)

const (
	maxFrame   = 64 << 10
	ioTimeout  = 5 * time.Second
	ipcVersion = 1
)

var (
	// ErrNotRunning means no Discord client answered.
	ErrNotRunning = errors.New("discord: Discord isn't running")
	// ErrClosed means Discord closed the connection.
	ErrClosed = errors.New("discord: connection closed")
)

// Activity is what the profile shows.
type Activity struct {
	Details    string      `json:"details,omitempty"`
	State      string      `json:"state,omitempty"`
	Timestamps *Timestamps `json:"timestamps,omitempty"`
	Assets     *Assets     `json:"assets,omitempty"`
}

// Timestamps show an elapsed time from Start (Unix milliseconds).
type Timestamps struct {
	Start int64 `json:"start"`
}

// Assets are images uploaded to the Discord application, by key.
type Assets struct {
	LargeImage string `json:"large_image,omitempty"`
	LargeText  string `json:"large_text,omitempty"`
	SmallImage string `json:"small_image,omitempty"`
	SmallText  string `json:"small_text,omitempty"`
}

// Client is one connection to the local Discord client.
type Client struct {
	conn io.ReadWriteCloser
	mu   sync.Mutex // one request at a time
}

type frame struct {
	Cmd   string          `json:"cmd"`
	Evt   string          `json:"evt"`
	Nonce string          `json:"nonce"`
	Data  json.RawMessage `json:"data"`
	Args  any             `json:"args,omitempty"`
}

// Connect finds a running Discord client and completes the handshake.
func Connect(clientID string) (*Client, error) {
	for _, path := range candidates() {
		conn, err := dial(path)
		if err != nil {
			continue
		}
		c := &Client{conn: conn}
		if err := c.handshake(clientID); err != nil {
			_ = conn.Close()
			continue
		}
		return c, nil
	}
	return nil, ErrNotRunning
}

func (c *Client) handshake(clientID string) error {
	if err := c.write(opHandshake, map[string]any{"v": ipcVersion, "client_id": clientID}); err != nil {
		return err
	}
	return c.await(func(f frame) bool { return f.Cmd == "DISPATCH" && f.Evt == "READY" })
}

// SetActivity shows a, or clears the activity when a is nil. pid is the
// process the activity belongs to (Discord clears it when that exits).
func (c *Client) SetActivity(pid int, a *Activity) error {
	nonce, err := newNonce()
	if err != nil {
		return err
	}
	args := map[string]any{"pid": pid}
	if a != nil {
		args["activity"] = a
	}
	if err := c.write(opFrame, frame{Cmd: "SET_ACTIVITY", Nonce: nonce, Args: args}); err != nil {
		return err
	}
	return c.await(func(f frame) bool { return f.Nonce == nonce })
}

// Close ends the connection.
func (c *Client) Close() error { return c.conn.Close() }

func (c *Client) write(op uint32, v any) error {
	payload, err := json.Marshal(v)
	if err != nil {
		return err
	}
	buf := make([]byte, 8, 8+len(payload))
	binary.LittleEndian.PutUint32(buf, op)
	binary.LittleEndian.PutUint32(buf[4:], uint32(len(payload))) //nolint:gosec // our payloads are small
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.timed(func() error {
		_, err := c.conn.Write(append(buf, payload...))
		return err
	})
}

// await reads frames until done matches one, answering pings. An ERROR
// event for the request fails it.
func (c *Client) await(done func(frame) bool) error {
	for {
		op, payload, err := c.read()
		if err != nil {
			return err
		}
		switch op {
		case opPing:
			if err := c.writeRaw(opPong, payload); err != nil {
				return err
			}
		case opClose:
			return ErrClosed
		case opFrame:
			var f frame
			if json.Unmarshal(payload, &f) != nil {
				continue
			}
			if done(f) {
				if f.Evt == "ERROR" {
					var e struct {
						Code    int    `json:"code"`
						Message string `json:"message"`
					}
					_ = json.Unmarshal(f.Data, &e)
					return fmt.Errorf("discord: %s (%d)", e.Message, e.Code)
				}
				return nil
			}
		}
	}
}

func (c *Client) writeRaw(op uint32, payload []byte) error {
	buf := make([]byte, 8, 8+len(payload))
	binary.LittleEndian.PutUint32(buf, op)
	binary.LittleEndian.PutUint32(buf[4:], uint32(len(payload))) //nolint:gosec // capped by read
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.timed(func() error {
		_, err := c.conn.Write(append(buf, payload...))
		return err
	})
}

func (c *Client) read() (op uint32, payload []byte, err error) {
	err = c.timed(func() error {
		var head [8]byte
		if _, err := io.ReadFull(c.conn, head[:]); err != nil {
			return err
		}
		op = binary.LittleEndian.Uint32(head[:])
		n := binary.LittleEndian.Uint32(head[4:])
		if n > maxFrame {
			return fmt.Errorf("discord: frame of %d bytes", n)
		}
		payload = make([]byte, n)
		_, err := io.ReadFull(c.conn, payload)
		return err
	})
	return op, payload, err
}

// timed runs fn and closes the connection if it takes longer than
// ioTimeout, which unblocks it (Windows pipes opened as files have no
// deadlines).
func (c *Client) timed(fn func() error) error {
	done := make(chan error, 1)
	go func() { done <- fn() }()
	select {
	case err := <-done:
		return err
	case <-time.After(ioTimeout):
		_ = c.conn.Close()
		<-done
		return fmt.Errorf("discord: no answer within %s", ioTimeout)
	}
}

func newNonce() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
