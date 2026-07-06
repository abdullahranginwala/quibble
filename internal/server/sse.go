package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
)

// event is one server-sent event: a name and a JSON payload.
type event struct {
	name string
	data any
}

// hub fans server events out to every connected SSE client. Each client has a
// buffered channel; a slow client that fills its buffer drops events rather
// than blocking the watcher (it will refetch on the next event it does get).
type hub struct {
	mu      sync.Mutex
	clients map[chan event]struct{}
}

func newHub() *hub {
	return &hub{clients: map[chan event]struct{}{}}
}

func (h *hub) subscribe() chan event {
	ch := make(chan event, 16)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *hub) unsubscribe(ch chan event) {
	h.mu.Lock()
	if _, ok := h.clients[ch]; ok {
		delete(h.clients, ch)
		close(ch)
	}
	h.mu.Unlock()
}

func (h *hub) broadcast(name string, data any) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.clients {
		select {
		case ch <- event{name: name, data: data}:
		default: // client buffer full; drop (it refetches on the next event)
		}
	}
}

// clientCount reports the number of connected SSE clients (test aid).
func (h *hub) clientCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients)
}

// handleEvents streams events to one client until the request is cancelled.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	// Opening comment so proxies/clients see the stream is live immediately.
	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	ch := s.hub.subscribe()
	defer s.hub.unsubscribe(ch)

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			payload, err := json.Marshal(ev.data)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.name, payload)
			flusher.Flush()
		}
	}
}
