package hub

import (
	"sync"

	"github.com/google/uuid"
)

// Hub manages all active WebSocket clients on this server instance.
type Hub struct {
	mu      sync.RWMutex
	clients map[uuid.UUID]*Client // keyed by userID
}

func New() *Hub {
	return &Hub{clients: make(map[uuid.UUID]*Client)}
}

// Register adds a client to the hub.
func (h *Hub) Register(c *Client) {
	h.mu.Lock()
	h.clients[c.UserID] = c
	h.mu.Unlock()
}

// Unregister removes a client and closes its send channel.
func (h *Hub) Unregister(c *Client) {
	h.mu.Lock()
	if _, ok := h.clients[c.UserID]; ok {
		delete(h.clients, c.UserID)
		close(c.Send)
	}
	h.mu.Unlock()
}

// BroadcastRoom delivers an event to all locally-connected clients that are in the given room.
func (h *Hub) BroadcastRoom(roomID uuid.UUID, evt Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, c := range h.clients {
		if c.InRoom(roomID) {
			select {
			case c.Send <- evt:
			default: // drop if buffer full
			}
		}
	}
}

// BroadcastUser delivers an event to a specific user if they are connected locally.
func (h *Hub) BroadcastUser(userID uuid.UUID, evt Event) {
	h.mu.RLock()
	c, ok := h.clients[userID]
	h.mu.RUnlock()
	if ok {
		select {
		case c.Send <- evt:
		default:
		}
	}
}

// BroadcastAll delivers an event to all locally-connected clients.
func (h *Hub) BroadcastAll(evt Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, c := range h.clients {
		select {
		case c.Send <- evt:
		default:
		}
	}
}

// IsOnline reports whether a user has an active connection on this instance.
func (h *Hub) IsOnline(userID uuid.UUID) bool {
	h.mu.RLock()
	_, ok := h.clients[userID]
	h.mu.RUnlock()
	return ok
}
