package server

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
)

const wsMagicKey = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// TelemetryHub orchestrates real-time subscribers over WebSocket and Server-Sent Events.
type TelemetryHub struct {
	mu          sync.RWMutex
	subscribers map[chan TelemetryFrame]struct{}
	wsConns     map[net.Conn]struct{}
	lastFrame   *TelemetryFrame
}

// NewTelemetryHub constructs a new broadcasting hub.
func NewTelemetryHub() *TelemetryHub {
	return &TelemetryHub{
		subscribers: make(map[chan TelemetryFrame]struct{}),
		wsConns:     make(map[net.Conn]struct{}),
	}
}

// Subscribe returns a channel of telemetry frames and an unsubscription callback.
func (h *TelemetryHub) Subscribe() (<-chan TelemetryFrame, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()

	ch := make(chan TelemetryFrame, 32)
	h.subscribers[ch] = struct{}{}

	if h.lastFrame != nil {
		select {
		case ch <- *h.lastFrame:
		default:
		}
	}

	unsubscribe := func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if _, exists := h.subscribers[ch]; exists {
			delete(h.subscribers, ch)
			close(ch)
		}
	}

	return ch, unsubscribe
}

// Broadcast distributes a new frame to all active WebSocket and channel subscribers.
func (h *TelemetryHub) Broadcast(frame TelemetryFrame) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.lastFrame = &frame

	// Broadcast to channel subscribers
	for ch := range h.subscribers {
		select {
		case ch <- frame:
		default:
		}
	}

	// Broadcast to active WebSocket conns
	payload, err := json.Marshal(frame)
	if err == nil {
		for conn := range h.wsConns {
			if err := writeWSTextFrame(conn, payload); err != nil {
				_ = conn.Close()
				delete(h.wsConns, conn)
			}
		}
	}
}

// HandleWebSocket upgrades standard HTTP connection into RFC 6455 WebSocket.
func (h *TelemetryHub) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		http.Error(w, "missing Sec-WebSocket-Key header", http.StatusBadRequest)
		return
	}

	hash := sha1.Sum([]byte(key + wsMagicKey))
	accept := base64.StdEncoding.EncodeToString(hash[:])

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "websocket hijacking not supported", http.StatusInternalServerError)
		return
	}

	conn, bufrw, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	respHeader := fmt.Sprintf("HTTP/1.1 101 Switching Protocols\r\n"+
		"Upgrade: websocket\r\n"+
		"Connection: Upgrade\r\n"+
		"Sec-WebSocket-Accept: %s\r\n\r\n", accept)

	if _, err := bufrw.WriteString(respHeader); err != nil {
		_ = conn.Close()
		return
	}
	_ = bufrw.Flush()

	h.mu.Lock()
	h.wsConns[conn] = struct{}{}
	if h.lastFrame != nil {
		if data, err := json.Marshal(h.lastFrame); err == nil {
			_ = writeWSTextFrame(conn, data)
		}
	}
	h.mu.Unlock()
}

// HandleSSE serves live Server-Sent Events stream as a reliable web fallback.
func (h *TelemetryHub) HandleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch, unsub := h.Subscribe()
	defer unsub()

	for {
		select {
		case <-r.Context().Done():
			return
		case frame, ok := <-ch:
			if !ok {
				return
			}
			data, err := json.Marshal(frame)
			if err != nil {
				continue
			}
			_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

func writeWSTextFrame(w io.Writer, payload []byte) error {
	length := len(payload)
	header := make([]byte, 0, 10)
	header = append(header, 0x81) // FIN + text opcode

	if length <= 125 {
		header = append(header, byte(length))
	} else if length <= 65535 {
		header = append(header, 126, byte(length>>8), byte(length&0xFF))
	} else {
		header = append(header, 127)
		for i := 7; i >= 0; i-- {
			header = append(header, byte((length>>(i*8))&0xFF))
		}
	}

	if _, err := w.Write(header); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}
