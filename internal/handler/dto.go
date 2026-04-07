package handler

import (
	"time"

	"github.com/poyrazk/cloudtalk/internal/model"
)

type AuthUserResponse struct {
	ID          string  `json:"id"`
	Username    string  `json:"username"`
	DisplayName string  `json:"display_name"`
	AvatarURL   *string `json:"avatar_url"`
	Email       string  `json:"email"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

type PublicProfileResponse struct {
	ID          string  `json:"id"`
	Username    string  `json:"username"`
	DisplayName string  `json:"display_name"`
	AvatarURL   *string `json:"avatar_url"`
}

type UpdateMeRequest struct {
	DisplayName *string `json:"display_name"`
	AvatarURL   *string `json:"avatar_url"`
}

type RoomResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	CreatedBy   string `json:"created_by"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type RoomMemberResponse struct {
	UserID      string  `json:"user_id"`
	Username    string  `json:"username"`
	DisplayName string  `json:"display_name"`
	AvatarURL   *string `json:"avatar_url"`
	Role        string  `json:"role"`
	JoinedAt    string  `json:"joined_at"`
	LastSeen    *string `json:"last_seen"`
	Online      bool    `json:"online"`
}

type RoomUnreadCountResponse struct {
	RoomID string `json:"room_id"`
	Count  int    `json:"count"`
}

type RoomConversationResponse struct {
	RoomID      string           `json:"room_id"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	MemberCount int              `json:"member_count"`
	OnlineCount int              `json:"online_count"`
	UnreadCount int              `json:"unread_count"`
	LastMessage *MessageResponse `json:"last_message"`
}

type MessageResponse struct {
	ID        string  `json:"id"`
	RoomID    string  `json:"room_id"`
	SenderID  string  `json:"sender_id"`
	Content   string  `json:"content"`
	CreatedAt string  `json:"created_at"`
	EditedAt  *string `json:"edited_at"`
	DeletedAt *string `json:"deleted_at"`
}

type DMUnreadCountResponse struct {
	UserID string `json:"user_id"`
	Count  int    `json:"count"`
}

type DMConversationResponse struct {
	UserID      string                 `json:"user_id"`
	Username    string                 `json:"username"`
	DisplayName string                 `json:"display_name"`
	AvatarURL   *string                `json:"avatar_url"`
	Online      bool                   `json:"online"`
	LastSeen    *string                `json:"last_seen"`
	UnreadCount int                    `json:"unread_count"`
	LastMessage *DirectMessageResponse `json:"last_message"`
}

type DirectMessageResponse struct {
	ID          string  `json:"id"`
	SenderID    string  `json:"sender_id"`
	ReceiverID  string  `json:"receiver_id"`
	Content     string  `json:"content"`
	CreatedAt   string  `json:"created_at"`
	DeliveredAt *string `json:"delivered_at"`
	ReadAt      *string `json:"read_at"`
	EditedAt    *string `json:"edited_at"`
	DeletedAt   *string `json:"deleted_at"`
}

func authUserResponseFromModel(u *model.User) AuthUserResponse {
	return AuthUserResponse{
		ID:          u.ID.String(),
		Username:    u.Username,
		DisplayName: u.DisplayName,
		AvatarURL:   u.AvatarURL,
		Email:       u.Email,
		CreatedAt:   u.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:   u.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	}
}

func publicProfileResponseFromModel(u *model.User) PublicProfileResponse {
	return PublicProfileResponse{
		ID:          u.ID.String(),
		Username:    u.Username,
		DisplayName: u.DisplayNameOrUsername(),
		AvatarURL:   u.AvatarURL,
	}
}

func roomResponseFromModel(room *model.Room) RoomResponse {
	return RoomResponse{
		ID:          room.ID.String(),
		Name:        room.Name,
		Description: room.Description,
		CreatedBy:   room.CreatedBy.String(),
		CreatedAt:   room.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:   room.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	}
}

func roomMemberResponseFromModel(member *model.RoomMemberDetail) RoomMemberResponse {
	return RoomMemberResponse{
		UserID:      member.UserID.String(),
		Username:    member.Username,
		DisplayName: member.DisplayName,
		AvatarURL:   member.AvatarURL,
		Role:        member.Role,
		JoinedAt:    member.JoinedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		LastSeen:    formatTimePtr(member.LastSeen),
		Online:      member.Online,
	}
}

func roomUnreadCountResponseFromModel(count *model.RoomUnreadCount) RoomUnreadCountResponse {
	return RoomUnreadCountResponse{RoomID: count.RoomID.String(), Count: count.Count}
}

func roomConversationResponseFromModel(conv *model.RoomConversation) RoomConversationResponse {
	var lastMessage *MessageResponse
	if conv.LastMessage != nil {
		lastMessage = messageResponseFromModel(conv.LastMessage)
	}
	return RoomConversationResponse{
		RoomID:      conv.RoomID.String(),
		Name:        conv.Name,
		Description: conv.Description,
		MemberCount: conv.MemberCount,
		OnlineCount: conv.OnlineCount,
		UnreadCount: conv.UnreadCount,
		LastMessage: lastMessage,
	}
}

func messageResponseFromModel(msg *model.Message) *MessageResponse {
	if msg == nil {
		return nil
	}
	return &MessageResponse{
		ID:        msg.ID.String(),
		RoomID:    msg.RoomID.String(),
		SenderID:  msg.SenderID.String(),
		Content:   msg.Content,
		CreatedAt: msg.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		EditedAt:  formatTimePtr(msg.EditedAt),
		DeletedAt: formatTimePtr(msg.DeletedAt),
	}
}

func dmUnreadCountResponseFromModel(count *model.DMUnreadCount) DMUnreadCountResponse {
	return DMUnreadCountResponse{UserID: count.UserID.String(), Count: count.Count}
}

func dmConversationResponseFromModel(conv *model.DMConversation) DMConversationResponse {
	return DMConversationResponse{
		UserID:      conv.UserID.String(),
		Username:    conv.Username,
		DisplayName: conv.DisplayName,
		AvatarURL:   conv.AvatarURL,
		Online:      conv.Online,
		LastSeen:    formatTimePtr(conv.LastSeen),
		UnreadCount: conv.UnreadCount,
		LastMessage: dmResponseFromModel(conv.LastMessage),
	}
}

func dmResponseFromModel(msg *model.DirectMessage) *DirectMessageResponse {
	if msg == nil {
		return nil
	}
	return &DirectMessageResponse{
		ID:          msg.ID.String(),
		SenderID:    msg.SenderID.String(),
		ReceiverID:  msg.ReceiverID.String(),
		Content:     msg.Content,
		CreatedAt:   msg.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		DeliveredAt: formatTimePtr(msg.DeliveredAt),
		ReadAt:      formatTimePtr(msg.ReadAt),
		EditedAt:    formatTimePtr(msg.EditedAt),
		DeletedAt:   formatTimePtr(msg.DeletedAt),
	}
}

func formatTimePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	formatted := t.UTC().Format("2006-01-02T15:04:05Z07:00")
	return &formatted
}
