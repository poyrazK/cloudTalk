//go:build integration

package itest

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	authsvc "github.com/poyrazk/cloudtalk/internal/auth"
	"github.com/poyrazk/cloudtalk/internal/handler"
	"github.com/poyrazk/cloudtalk/internal/kafka"
	"github.com/poyrazk/cloudtalk/internal/repository"
	"github.com/poyrazk/cloudtalk/internal/service"
)

type App struct {
	Router   http.Handler
	Auth     *authsvc.Service
	Users    *repository.UserRepo
	Rooms    *repository.RoomRepo
	Messages *repository.MessageRepo
}

// BuildHTTPApp wires repositories/services/handlers similar to cmd/server/main.go.
// It uses a no-op event publisher to keep HTTP integration tests deterministic.
func BuildHTTPApp(pool *pgxpool.Pool) *App {
	userRepo := repository.NewUserRepo(pool)
	roomRepo := repository.NewRoomRepo(pool)
	msgRepo := repository.NewMessageRepo(pool)

	auth := authsvc.NewService(userRepo, "integration-secret", 15, 7)
	roomSvc := service.NewRoomService(roomRepo)
	msgSvc := service.NewMessageService(roomRepo, msgRepo, nilPublisher{})

	authH := handler.NewAuthHandler(auth)
	roomH := handler.NewRoomHandler(roomSvc, msgSvc)
	dmH := handler.NewDMHandler(msgSvc)

	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)

	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/auth/register", authH.Register)
		r.Post("/auth/login", authH.Login)
		r.Post("/auth/refresh", authH.Refresh)
		r.Post("/auth/logout", authH.Logout)

		r.Group(func(r chi.Router) {
			r.Use(authsvc.Middleware(auth))
			r.Post("/rooms", roomH.Create)
			r.Get("/rooms", roomH.List)
			r.Get("/rooms/{id}", roomH.Get)
			r.Post("/rooms/{id}/join", roomH.Join)
			r.Post("/rooms/{id}/leave", roomH.Leave)
			r.Get("/rooms/{id}/messages", roomH.Messages)
			r.Get("/dms/{userId}/messages", dmH.Messages)
		})
	})

	return &App{
		Router:   r,
		Auth:     auth,
		Users:    userRepo,
		Rooms:    roomRepo,
		Messages: msgRepo,
	}
}

type nilPublisher struct{}

func (nilPublisher) Publish(_ string, _ string, _ kafka.ChatEvent) error { return nil }
