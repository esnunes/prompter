package gotk

import (
	"encoding/json"
	"log"
	"log/slog"
	"net/http"

	"github.com/coder/websocket"
)

// wsCommand is the JSON shape sent from client to server.
type wsCommand struct {
	Cmd     string         `json:"cmd"`
	Payload map[string]any `json:"payload"`
	Ref     string         `json:"ref"`
	URL     string         `json:"url"`
}

// wsResponse is the JSON shape sent from server to client.
type wsResponse struct {
	Ref          string        `json:"ref,omitempty"`
	Instructions []Instruction `json:"ins"`
	Error        string        `json:"error,omitempty"`
}

// ServeWebSocket upgrades an HTTP request to a WebSocket connection
// and starts the command read/dispatch loop.
func (m *Mux) ServeWebSocket(w http.ResponseWriter, r *http.Request) {
	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"localhost:*", "127.0.0.1:*", "[::1]:*"},
	})
	if err != nil {
		log.Printf("gotk: websocket accept: %v", err)
		return
	}
	defer ws.CloseNow()

	conn := newConn(ws)

	// Create ConnEntry. If the client provided a ?url= query param,
	// use it as the initial lastURL so broadcasts reach this connection
	// even before the first command arrives.
	entry := &ConnEntry{
		Conn:    conn,
		views:   make(map[string]*viewEntry),
		lastURL: r.URL.Query().Get("url"),
	}

	// Lazily create the view for the initial URL so broadcasts work immediately.
	m.mu.RLock()
	viewFactory := m.viewFactory
	tmpl := m.templates
	m.mu.RUnlock()
	if entry.lastURL != "" && viewFactory != nil {
		slog.Debug("gotk: creating view (connect)", "url", entry.lastURL, "conn", conn.ID())
		entry.getOrCreateView(entry.lastURL, viewFactory, tmpl)
	}

	m.connRegistry.Store(conn.ID(), entry)

	// Notify connect handler
	m.mu.RLock()
	connectFn := m.connectFn
	disconnFn := m.disconnFn
	m.mu.RUnlock()

	if connectFn != nil {
		connectFn(conn)
	}

	defer func() {
		m.connRegistry.Delete(conn.ID())
		if disconnFn != nil {
			disconnFn(conn)
		}
	}()

	// Read loop: one goroutine per connection, sequential command processing
	for {
		_, data, err := ws.Read(r.Context())
		if err != nil {
			// Connection closed (normal or abnormal)
			if websocket.CloseStatus(err) == websocket.StatusNormalClosure ||
				websocket.CloseStatus(err) == websocket.StatusGoingAway ||
				r.Context().Err() != nil {
				return
			}
			log.Printf("gotk: ws read: %v", err)
			return
		}

		var cmd wsCommand
		if err := json.Unmarshal(data, &cmd); err != nil {
			log.Printf("gotk: ws unmarshal: %v", err)
			continue
		}

		ins, errMsg := m.dispatch(conn, cmd.Cmd, cmd.URL, cmd.Payload)

		resp := wsResponse{
			Ref:          cmd.Ref,
			Instructions: ins,
			Error:        errMsg,
		}

		if err := conn.writeJSON(resp); err != nil {
			log.Printf("gotk: ws write: %v", err)
			return
		}
	}
}
