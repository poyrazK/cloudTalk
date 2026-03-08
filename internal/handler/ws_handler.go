package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	authsvc "github.com/poyrazk/cloudtalk/internal/auth"
	"github.com/poyrazk/cloudtalk/internal/hub"
	"github.com/poyrazk/cloudtalk/internal/kafka"
	"github.com/poyrazk/cloudtalk/internal/service"
)

// incomingMsg is the JSON envelope sent by the client over WebSocket.
type incomingMsg struct {
	Type    string `json:"type"`
	RoomID  string `json:"room_id,omitempty"`
	To      string `json:"to,omitempty"` // DM target user ID
	Content string `json:"content,omitempty"`
	Typing  bool   `json:"typing,omitempty"`
}

// WSHandler upgrades HTTP connections to WebSocket and drives the read/write pumps.
type WSHandler struct {
	upgrader websocket.Upgrader
	auth     *authsvc.Service
	hub      *hub.Hub
	messages *service.MessageService
	presence *service.PresenceService
	producer *kafka.Producer
}

func NewWSHandler(
	auth *authsvc.Service,
	h *hub.Hub,
	messages *service.MessageService,
	presence *service.PresenceService,
	producer *kafka.Producer,
	allowedOrigins []string,
) *WSHandler {
	u := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			if len(allowedOrigins) == 0 {
				return true // dev: allow all
			}
			origin := r.Header.Get("Origin")
			for _, allowed := range allowedOrigins {
				if allowed == origin {
					return true
				}
			}
			return false
		},
	}
	return &WSHandler{
		upgrader: u,
		auth:     auth,
		hub:      h,
		messages: messages,
		presence: presence,
		producer: producer,
	}
}

func (h *WSHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Authenticate via ?token= query param
	tokenStr := r.URL.Query().Get("token")
	if tokenStr == "" {
		http.Error(w, "missing token", http.StatusUnauthorized)
		return
	}
	userID, err := h.auth.ValidateAccessToken(tokenStr)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("ws upgrade error", "err", err)
		return
	}

	client := hub.NewClient(userID)
	h.hub.Register(client)
	h.presence.SetOnline(r.Context(), userID)

	go h.writePump(conn, client)
	h.readPump(conn, client, r.Context())

	h.hub.Unregister(client)
	h.presence.SetOffline(context.Background(), userID)
}

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = (pongWait * 9) / 10
	maxMsgSize = 4096
)

func (h *WSHandler) readPump(conn *websocket.Conn, client *hub.Client, ctx context.Context) {
	defer conn.Close()
	conn.SetReadLimit(maxMsgSize)
	if err := conn.SetReadDeadline(time.Now().Add(pongWait)); err != nil {
		return
	}
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				slog.Error("ws read error", "err", err)
			}
			return
		}
		h.handleClientMessage(ctx, client, raw)
	}
}

func (h *WSHandler) writePump(conn *websocket.Conn, client *hub.Client) {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		conn.Close()
	}()

	for {
		select {
		case evt, ok := <-client.Send:
			if err := conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
				return
			}
			if !ok {
				_ = conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := conn.WriteMessage(websocket.TextMessage, evt.Data); err != nil {
				return
			}
		case <-ticker.C:
			if err := conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
				return
			}
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (h *WSHandler) handleClientMessage(ctx context.Context, client *hub.Client, raw []byte) {
	var msg incomingMsg
	if err := json.Unmarshal(raw, &msg); err != nil {
		return
	}

	switch msg.Type {
	case "join_room":
		if id, err := uuid.Parse(msg.RoomID); err == nil {
			client.JoinRoom(id)
		}

	case "leave_room":
		if id, err := uuid.Parse(msg.RoomID); err == nil {
			client.LeaveRoom(id)
		}

	case "message":
		roomID, err := uuid.Parse(msg.RoomID)
		if err != nil {
			return
		}
		if err := validateContent(msg.Content); err != nil {
			return
		}
		if _, err := h.messages.SendRoomMessage(ctx, roomID, client.UserID, msg.Content); err != nil {
			slog.Error("ws: send room message", "err", err)
		}

	case "dm":
		toID, err := uuid.Parse(msg.To)
		if err != nil {
			return
		}
		if err := validateContent(msg.Content); err != nil {
			return
		}
		if _, err := h.messages.SendDM(ctx, client.UserID, toID, msg.Content); err != nil {
			slog.Error("ws: send dm", "err", err)
		}

	case "typing":
		roomID, err := uuid.Parse(msg.RoomID)
		if err != nil {
			return
		}
		payload, err := json.Marshal(map[string]interface{}{
			"user_id": client.UserID.String(),
			"room_id": roomID.String(),
			"typing":  msg.Typing,
		})
		if err != nil {
			slog.Error("ws: marshal typing payload", "err", err)
			return
		}
		if err := h.producer.Publish(kafka.TopicRoomMessages, roomID.String(), kafka.ChatEvent{
			Type:     "typing",
			RoomID:   roomID.String(),
			SenderID: client.UserID.String(),
			Payload:  payload,
		}); err != nil {
			slog.Error("ws: publish typing to kafka", "err", err)
		}
	}
}
