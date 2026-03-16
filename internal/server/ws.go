package server

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/esnunes/prompter/internal/x"
	"github.com/google/uuid"
)

// WSConn represents a WebSocket connection with its metadata.
type WSConn struct {
	ID   string
	Conn *websocket.Conn
}

// WSCommandContext holds the context for a WebSocket command handler invocation.
type WSCommandContext struct {
	Conn    *WSConn
	Server  *Server
	Headers map[string]string
	Params  map[string]any
}

// WSCommandHandler handles a WebSocket command from a client.
type WSCommandHandler func(ctx *WSCommandContext)

// wsConns tracks all active WebSocket connections for broadcasting.
var wsConns sync.Map // map[string]*WSConn

// wsCommands maps command names to their handlers.
var wsCommands sync.Map // map[string]WSCommandHandler

// HandleWS registers a command handler for the given command name.
func (s *Server) HandleWS(command string, handler WSCommandHandler) {
	wsCommands.Store(command, handler)
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// Local-only app; allow all origins for development.
		InsecureSkipVerify: true,
	})
	if err != nil {
		log.Printf("ws: accept error: %v", err)
		return
	}

	conn := &WSConn{
		ID:   uuid.New().String(),
		Conn: c,
	}
	wsConns.Store(conn.ID, conn)
	defer func() {
		wsConns.Delete(conn.ID)
		c.CloseNow()
	}()

	// Read loop: parse incoming messages and dispatch to command handlers.
	ctx := context.Background()
	for {
		_, data, err := c.Read(ctx)
		if err != nil {
			return
		}
		s.dispatchWSCommand(conn, data)
	}
}

// wsMessage is the wire format for client-to-server WebSocket messages.
type wsMessage struct {
	Command string            `json:"command"`
	Headers map[string]string `json:"HEADERS"`
}

func (s *Server) dispatchWSCommand(conn *WSConn, data []byte) {
	var msg wsMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		log.Printf("ws: invalid message from %s: %v", conn.ID, err)
		return
	}

	if msg.Command == "" {
		log.Printf("ws: message from %s missing command field", conn.ID)
		return
	}

	handler, ok := wsCommands.Load(msg.Command)
	if !ok {
		log.Printf("ws: unknown command %q from %s", msg.Command, conn.ID)
		return
	}

	// Lower case all headers
	if msg.Headers != nil {
		msg.Headers = x.TransformKeys(msg.Headers, strings.ToLower)
	}

	// Extract params: all fields except "command" and "HEADERS".
	var raw map[string]any
	json.Unmarshal(data, &raw)
	params := make(map[string]any, len(raw))
	for k, v := range raw {
		if k != "command" && k != "HEADERS" {
			params[k] = v
		}
	}

	cmdCtx := &WSCommandContext{
		Conn:    conn,
		Server:  s,
		Headers: msg.Headers,
		Params:  params,
	}

	handler.(WSCommandHandler)(cmdCtx)
}

// Send writes a text message to this connection.
func (c *WSConn) Send(msg []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return c.Conn.Write(ctx, websocket.MessageText, msg)
}

// SendEvents sends a JSON event message to this connection.
func (c *WSConn) SendEvents(events []map[string]any) error {
	data, err := json.Marshal(map[string]any{"events": events})
	if err != nil {
		return err
	}
	return c.Send(data)
}

// SendTrigger sends a single HX-Trigger event to this connection.
func (c *WSConn) SendTrigger(name string, detail any) error {
	var trigger any
	if detail != nil {
		trigger = map[string]any{name: detail}
	} else {
		trigger = name
	}
	return c.SendEvents([]map[string]any{
		{"hx-trigger": trigger},
	})
}

// broadcast sends a text message to all connected WebSocket clients.
// Failed connections are removed from the registry.
func (s *Server) broadcast(msg []byte) {
	wsConns.Range(func(key, value any) bool {
		c := value.(*WSConn)
		if err := c.Send(msg); err != nil {
			wsConns.Delete(key)
			c.Conn.CloseNow()
		}
		return true
	})
}

// broadcastEvents sends a JSON event message to all connected clients.
// The message format is: {"events": [{"hx-trigger": {...}}, ...]}
func (s *Server) broadcastEvents(events []map[string]any) {
	msg := map[string]any{"events": events}
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("ws: marshal error: %v", err)
		return
	}
	s.broadcast(data)
}

// broadcastTrigger is a convenience for broadcasting a single HX-Trigger event.
// If detail is nil, the trigger name is sent as a simple string trigger.
func (s *Server) broadcastTrigger(name string, detail any) {
	var trigger any
	if detail != nil {
		trigger = map[string]any{name: detail}
	} else {
		trigger = name
	}
	s.broadcastEvents([]map[string]any{
		{"hx-trigger": trigger},
	})
}
