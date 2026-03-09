package handler

import (
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"unicode/utf8"
)

const (
	maxUsernameLen    = 32
	maxEmailLen       = 254
	maxPasswordLen    = 72 // bcrypt max
	maxRoomNameLen    = 64
	maxDescriptionLen = 256
	maxMessageLen     = 4096
)

func validateRegister(username, email, password string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("username is required")
	}
	if utf8.RuneCountInString(username) > maxUsernameLen {
		return fmt.Errorf("username must be at most %d characters", maxUsernameLen)
	}
	if strings.ContainsAny(username, " \t\n\r") {
		return errors.New("username must not contain whitespace")
	}
	if len(email) > maxEmailLen {
		return errors.New("email is too long")
	}
	if _, err := mail.ParseAddress(email); err != nil {
		return errors.New("email is invalid")
	}
	if len(password) < 8 {
		return errors.New("password must be at least 8 characters")
	}
	if len(password) > maxPasswordLen {
		return errors.New("password is too long")
	}
	return nil
}

func validateRoom(name, description string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("name is required")
	}
	if utf8.RuneCountInString(name) > maxRoomNameLen {
		return fmt.Errorf("name must be at most %d characters", maxRoomNameLen)
	}
	if utf8.RuneCountInString(description) > maxDescriptionLen {
		return fmt.Errorf("description must be at most %d characters", maxDescriptionLen)
	}
	return nil
}

func validateContent(content string) error {
	if strings.TrimSpace(content) == "" {
		return errors.New("content is required")
	}
	if utf8.RuneCountInString(content) > maxMessageLen {
		return fmt.Errorf("message must be at most %d characters", maxMessageLen)
	}
	return nil
}
