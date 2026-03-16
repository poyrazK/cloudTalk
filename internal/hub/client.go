package hub

import (
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/time/rate"
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
	limits map[string]*rate.Limiter
	mu     sync.RWMutex
}

type ThrottleConfig struct {
	ChatRate    rate.Limit
	ChatBurst   int
	TypingRate  rate.Limit
	TypingBurst int
	ReadRate    rate.Limit
	ReadBurst   int
	RoomRate    rate.Limit
	RoomBurst   int
}

func NewClient(userID uuid.UUID, throttle ...ThrottleConfig) *Client {
	config := ThrottleConfig{}
	if len(throttle) > 0 {
		config = throttle[0]
	}
	return &Client{
		UserID: userID,
		Send:   make(chan Event, 256),
		rooms:  make(map[uuid.UUID]struct{}),
		limits: map[string]*rate.Limiter{
			"chat":   rate.NewLimiter(config.ChatRate, config.ChatBurst),
			"typing": rate.NewLimiter(config.TypingRate, config.TypingBurst),
			"read":   rate.NewLimiter(config.ReadRate, config.ReadBurst),
			"room":   rate.NewLimiter(config.RoomRate, config.RoomBurst),
		},
	}
}

func (c *Client) Allow(group string) bool {
	c.mu.RLock()
	limiter, ok := c.limits[group]
	c.mu.RUnlock()
	if !ok {
		return true
	}
	return limiter.AllowN(time.Now(), 1)
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
