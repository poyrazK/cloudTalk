package hub

import (
	"sync"

	"github.com/google/uuid"
)

// Event is a serialized message delivered to WebSocket clients.
type Event struct {
	Data []byte
}

// Client represents a single WebSocket connection.
type Client struct {
	UserID uuid.UUID
	Send   chan Event
	rooms  map[uuid.UUID]struct{}
	mu     sync.RWMutex
}

func NewClient(userID uuid.UUID) *Client {
	return &Client{
		UserID: userID,
		Send:   make(chan Event, 256),
		rooms:  make(map[uuid.UUID]struct{}),
	}
}

func (c *Client) JoinRoom(roomID uuid.UUID) {
	c.mu.Lock()
	c.rooms[roomID] = struct{}{}
	c.mu.Unlock()
}

func (c *Client) LeaveRoom(roomID uuid.UUID) {
	c.mu.Lock()
	delete(c.rooms, roomID)
	c.mu.Unlock()
}

func (c *Client) InRoom(roomID uuid.UUID) bool {
	c.mu.RLock()
	_, ok := c.rooms[roomID]
	c.mu.RUnlock()
	return ok
}
