package handler

import (
	"context"
	"encoding/json"
	"errors"
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
	Type      string `json:"type"`
	RoomID    string `json:"room_id,omitempty"`
	MessageID string `json:"message_id,omitempty"`
	DMID      string `json:"dm_id,omitempty"`
	To        string `json:"to,omitempty"` // DM target user ID
	Content   string `json:"content,omitempty"`
	Typing    bool   `json:"typing,omitempty"`
}

type outgoingMsg struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// WSHandler upgrades HTTP connections to WebSocket and drives the read/write pumps.
type WSHandler struct {
	upgrader websocket.Upgrader
	auth     *authsvc.Service
	hub      *hub.Hub
	rooms    *service.RoomService
	messages *service.MessageService
	presence *service.PresenceService
	producer *kafka.Producer
}

func NewWSHandler(
	auth *authsvc.Service,
	h *hub.Hub,
	rooms *service.RoomService,
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
		rooms:    rooms,
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

	go h.writePump(r.Context(), conn, client)
	h.readPump(r.Context(), conn, client)

	h.hub.Unregister(client)
	h.presence.SetOffline(r.Context(), userID)
}

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = (pongWait * 9) / 10
	maxMsgSize = 4096
)

func (h *WSHandler) readPump(ctx context.Context, conn *websocket.Conn, client *hub.Client) {
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

func (h *WSHandler) writePump(ctx context.Context, conn *websocket.Conn, client *hub.Client) {
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
			h.handlePostWriteDMDelivery(ctx, client, evt.Data)
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
		h.handleRoomMessage(ctx, client, msg)

	case "dm":
		h.handleDM(ctx, client, msg)

	case "read_dm":
		h.handleReadDM(ctx, client, msg)

	case "read_room":
		h.handleReadRoom(ctx, client, msg)

	case "edit_message":
		h.handleEditRoomMessage(ctx, client, msg)

	case "delete_message":
		h.handleDeleteRoomMessage(ctx, client, msg)

	case "edit_dm":
		h.handleEditDM(ctx, client, msg)

	case "delete_dm":
		h.handleDeleteDM(ctx, client, msg)

	case "typing":
		h.handleTyping(client, msg)

	case "typing_dm":
		h.handleDMTyping(client, msg)
	}
}

func (h *WSHandler) handlePostWriteDMDelivery(ctx context.Context, client *hub.Client, raw []byte) {
	var out outgoingMsg
	if err := json.Unmarshal(raw, &out); err != nil {
		return
	}
	if out.Type != "dm" {
		return
	}

	var dm struct {
		ID         string `json:"id"`
		ReceiverID string `json:"receiver_id"`
	}
	if err := json.Unmarshal(out.Payload, &dm); err != nil {
		return
	}
	if dm.ReceiverID != client.UserID.String() {
		return
	}

	dmID, err := uuid.Parse(dm.ID)
	if err != nil {
		return
	}

	if _, err := h.messages.MarkDMDelivered(ctx, dmID, client.UserID); err != nil {
		switch {
		case errors.Is(err, service.ErrDMReceiptForbidden), errors.Is(err, service.ErrDMNotFound):
			return
		default:
			slog.Error("ws: mark dm delivered", "err", err)
		}
	}
}

func (h *WSHandler) handleRoomMessage(ctx context.Context, client *hub.Client, msg incomingMsg) {
	roomID, err := uuid.Parse(msg.RoomID)
	if err != nil {
		return
	}
	if err := validateContent(msg.Content); err != nil {
		return
	}
	if _, err := h.messages.SendRoomMessage(ctx, roomID, client.UserID, msg.Content); err != nil {
		switch {
		case errors.Is(err, service.ErrRoomMembershipRequired):
			slog.Warn("ws: room message forbidden", "user_id", client.UserID, "room_id", roomID)
		case errors.Is(err, service.ErrRoomMembershipCheck), errors.Is(err, service.ErrMessagePersistence):
			slog.Error("ws: send room message", "err", err)
		default:
			slog.Error("ws: send room message", "err", err)
		}
	}
}

func (h *WSHandler) handleDM(ctx context.Context, client *hub.Client, msg incomingMsg) {
	toID, err := uuid.Parse(msg.To)
	if err != nil {
		return
	}
	if err := validateContent(msg.Content); err != nil {
		return
	}
	if _, err := h.messages.SendDM(ctx, client.UserID, toID, msg.Content); err != nil {
		switch {
		case errors.Is(err, service.ErrDMToSelfForbidden):
			slog.Warn("ws: self dm forbidden", "user_id", client.UserID)
		case errors.Is(err, service.ErrMessagePersistence):
			slog.Error("ws: send dm", "err", err)
		default:
			slog.Error("ws: send dm", "err", err)
		}
	}
}

func (h *WSHandler) handleReadDM(ctx context.Context, client *hub.Client, msg incomingMsg) {
	dmID, err := uuid.Parse(msg.DMID)
	if err != nil {
		return
	}

	if _, err := h.messages.MarkDMRead(ctx, dmID, client.UserID); err != nil {
		switch {
		case errors.Is(err, service.ErrDMReceiptForbidden):
			slog.Warn("ws: dm read forbidden", "user_id", client.UserID, "dm_id", dmID)
		case errors.Is(err, service.ErrDMNotFound):
			slog.Warn("ws: dm not found", "user_id", client.UserID, "dm_id", dmID)
		default:
			slog.Error("ws: mark dm read", "err", err)
		}
	}
}

func (h *WSHandler) handleReadRoom(ctx context.Context, client *hub.Client, msg incomingMsg) {
	roomID, err := uuid.Parse(msg.RoomID)
	if err != nil {
		return
	}
	if err := h.rooms.MarkRead(ctx, roomID, client.UserID); err != nil {
		slog.Warn("ws: room read failed", "user_id", client.UserID, "room_id", roomID, "err", err)
	}
}

func (h *WSHandler) handleEditRoomMessage(ctx context.Context, client *hub.Client, msg incomingMsg) {
	messageID, err := uuid.Parse(msg.MessageID)
	if err != nil {
		return
	}
	if err := validateContent(msg.Content); err != nil {
		return
	}
	if _, err := h.messages.EditRoomMessage(ctx, messageID, client.UserID, msg.Content); err != nil {
		switch {
		case errors.Is(err, service.ErrMessageEditForbidden):
			slog.Warn("ws: room edit forbidden", "user_id", client.UserID, "message_id", messageID)
		case errors.Is(err, service.ErrMessageNotFound), errors.Is(err, service.ErrMessageDeleted):
			slog.Warn("ws: room edit rejected", "user_id", client.UserID, "message_id", messageID, "err", err)
		default:
			slog.Error("ws: edit room message", "err", err)
		}
	}
}

func (h *WSHandler) handleDeleteRoomMessage(ctx context.Context, client *hub.Client, msg incomingMsg) {
	messageID, err := uuid.Parse(msg.MessageID)
	if err != nil {
		return
	}
	if _, err := h.messages.DeleteRoomMessage(ctx, messageID, client.UserID); err != nil {
		switch {
		case errors.Is(err, service.ErrMessageDeleteForbidden):
			slog.Warn("ws: room delete forbidden", "user_id", client.UserID, "message_id", messageID)
		case errors.Is(err, service.ErrMessageNotFound):
			slog.Warn("ws: room delete missing", "user_id", client.UserID, "message_id", messageID)
		default:
			slog.Error("ws: delete room message", "err", err)
		}
	}
}

func (h *WSHandler) handleEditDM(ctx context.Context, client *hub.Client, msg incomingMsg) {
	dmID, err := uuid.Parse(msg.DMID)
	if err != nil {
		return
	}
	if err := validateContent(msg.Content); err != nil {
		return
	}
	if _, err := h.messages.EditDM(ctx, dmID, client.UserID, msg.Content); err != nil {
		switch {
		case errors.Is(err, service.ErrMessageEditForbidden):
			slog.Warn("ws: dm edit forbidden", "user_id", client.UserID, "dm_id", dmID)
		case errors.Is(err, service.ErrDMNotFound), errors.Is(err, service.ErrMessageDeleted):
			slog.Warn("ws: dm edit rejected", "user_id", client.UserID, "dm_id", dmID, "err", err)
		default:
			slog.Error("ws: edit dm", "err", err)
		}
	}
}

func (h *WSHandler) handleDeleteDM(ctx context.Context, client *hub.Client, msg incomingMsg) {
	dmID, err := uuid.Parse(msg.DMID)
	if err != nil {
		return
	}
	if _, err := h.messages.DeleteDM(ctx, dmID, client.UserID); err != nil {
		switch {
		case errors.Is(err, service.ErrMessageDeleteForbidden):
			slog.Warn("ws: dm delete forbidden", "user_id", client.UserID, "dm_id", dmID)
		case errors.Is(err, service.ErrDMNotFound):
			slog.Warn("ws: dm delete missing", "user_id", client.UserID, "dm_id", dmID)
		default:
			slog.Error("ws: delete dm", "err", err)
		}
	}
}

func (h *WSHandler) handleTyping(client *hub.Client, msg incomingMsg) {
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

func (h *WSHandler) handleDMTyping(client *hub.Client, msg incomingMsg) {
	toID, err := uuid.Parse(msg.To)
	if err != nil {
		return
	}
	if toID == client.UserID {
		slog.Warn("ws: self dm typing forbidden", "user_id", client.UserID)
		return
	}

	payload, err := json.Marshal(map[string]interface{}{
		"user_id":    client.UserID.String(),
		"to_user_id": toID.String(),
		"typing":     msg.Typing,
	})
	if err != nil {
		slog.Error("ws: marshal dm typing payload", "err", err)
		return
	}
	if err := h.producer.Publish(kafka.TopicDMMessages, toID.String(), kafka.ChatEvent{
		Type:     "typing_dm",
		SenderID: client.UserID.String(),
		ToUserID: toID.String(),
		Payload:  payload,
	}); err != nil {
		slog.Error("ws: publish dm typing to kafka", "err", err)
	}
}
