package gotk

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

var connIDCounter atomic.Int64

// Conn wraps a WebSocket connection with thread-safe writes.
type Conn struct {
	id int64
	ws *websocket.Conn
	mu sync.Mutex
}

func newConn(ws *websocket.Conn) *Conn {
	return &Conn{
		id: connIDCounter.Add(1),
		ws: ws,
	}
}

// ID returns the unique connection identifier.
func (c *Conn) ID() int64 {
	return c.id
}

// Push sends server-initiated instructions (no ref).
func (c *Conn) Push(ins []Instruction) error {
	msg := wsResponse{Instructions: ins}
	return c.writeJSON(msg)
}

func (c *Conn) writeJSON(v any) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return c.ws.Write(ctx, websocket.MessageText, data)
}
