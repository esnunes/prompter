package gotk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestServeWebSocket_RoundTrip(t *testing.T) {
	m := NewMux()
	m.Handle("echo", func(ctx *Context) error {
		msg := ctx.Payload.String("msg")
		ctx.HTML("#out", msg)
		return nil
	})

	srv := httptest.NewServer(http.HandlerFunc(m.ServeWebSocket))
	defer srv.Close()

	wsURL := "ws" + srv.URL[4:] // http -> ws

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ws, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer ws.CloseNow()

	// Send command
	cmd := wsCommand{Cmd: "echo", Payload: map[string]any{"msg": "hello"}, Ref: "1"}
	data, _ := json.Marshal(cmd)
	if err := ws.Write(ctx, websocket.MessageText, data); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Read response
	_, respData, err := ws.Read(ctx)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	var resp wsResponse
	if err := json.Unmarshal(respData, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if resp.Ref != "1" {
		t.Errorf("ref = %q, want 1", resp.Ref)
	}
	if len(resp.Instructions) != 1 {
		t.Fatalf("expected 1 instruction, got %d", len(resp.Instructions))
	}
	if resp.Instructions[0].HTML != "hello" {
		t.Errorf("HTML = %q, want hello", resp.Instructions[0].HTML)
	}
}

func TestServeWebSocket_UnknownCommand(t *testing.T) {
	m := NewMux()

	srv := httptest.NewServer(http.HandlerFunc(m.ServeWebSocket))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ws, _, err := websocket.Dial(ctx, "ws"+srv.URL[4:], nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer ws.CloseNow()

	cmd := wsCommand{Cmd: "nope", Ref: "2"}
	data, _ := json.Marshal(cmd)
	ws.Write(ctx, websocket.MessageText, data)

	_, respData, err := ws.Read(ctx)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	var resp wsResponse
	json.Unmarshal(respData, &resp)

	if resp.Error == "" {
		t.Error("expected error for unknown command")
	}
	if resp.Ref != "2" {
		t.Errorf("ref = %q, want 2", resp.Ref)
	}
}

func TestServeWebSocket_ConnectDisconnect(t *testing.T) {
	m := NewMux()

	connected := make(chan int64, 1)
	disconnected := make(chan int64, 1)

	m.HandleConnect(func(conn *Conn) {
		connected <- conn.ID()
	})
	m.HandleDisconnect(func(conn *Conn) {
		disconnected <- conn.ID()
	})

	srv := httptest.NewServer(http.HandlerFunc(m.ServeWebSocket))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ws, _, err := websocket.Dial(ctx, "ws"+srv.URL[4:], nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	select {
	case id := <-connected:
		if id <= 0 {
			t.Errorf("expected positive conn ID, got %d", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for connect")
	}

	ws.Close(websocket.StatusNormalClosure, "bye")

	select {
	case <-disconnected:
		// ok
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for disconnect")
	}
}
