package server

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/Joshua504/cineroom/internal/auth"
	"github.com/Joshua504/cineroom/internal/config"
	"github.com/Joshua504/cineroom/internal/database"
	"github.com/Joshua504/cineroom/internal/mail"
	"github.com/Joshua504/cineroom/internal/media"
	"github.com/Joshua504/cineroom/internal/websocket"
)

type Application struct {
	config config.Config
	store  *database.Store
	auth   *auth.Manager
	media  *media.Storage
	mailer mail.Mailer
	hub    *websocket.Hub
	logger *log.Logger
}

func New(cfg config.Config, logger *log.Logger) (*http.Server, *Application, error) {
	store, err := database.Open(cfg.DatabasePath)
	if err != nil {
		return nil, nil, err
	}
	mediaStorage, err := media.NewStorage(cfg.UploadDir, cfg.MaxUploadBytes)
	if err != nil {
		store.Close()
		return nil, nil, err
	}
	app := &Application{config: cfg, store: store, auth: auth.New(cfg.SessionSecret, strings.HasPrefix(cfg.AppOrigin, "https://")), media: mediaStorage, mailer: mail.Mailer{Host: cfg.SMTPHost, Port: cfg.SMTPPort, Username: cfg.SMTPUsername, Password: cfg.SMTPPassword, From: cfg.SMTPFrom}, hub: websocket.NewHub(), logger: logger}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", app.homeHandler)
	mux.HandleFunc("POST /api/auth/register", app.registerHandler)
	mux.HandleFunc("POST /api/auth/verify", app.verifyRegistrationHandler)
	mux.HandleFunc("POST /api/auth/login", app.loginHandler)
	mux.HandleFunc("POST /api/auth/logout", app.logoutHandler)
	mux.HandleFunc("GET /api/auth/google", app.googleStartHandler)
	mux.HandleFunc("GET /api/auth/google/callback", app.googleCallbackHandler)
	mux.Handle("GET /api/me", app.auth.Require(http.HandlerFunc(app.meHandler)))
	mux.Handle("GET /api/videos", app.auth.Require(http.HandlerFunc(app.listVideosHandler)))
	mux.Handle("GET /api/rooms", app.auth.Require(http.HandlerFunc(app.listRoomsHandler)))
	mux.Handle("GET /api/rooms/{roomID}/chat", app.auth.Require(http.HandlerFunc(app.chatHistoryHandler)))
	mux.Handle("POST /api/videos", app.auth.Require(http.HandlerFunc(app.uploadVideoHandler)))
	mux.Handle("POST /api/rooms", app.auth.Require(http.HandlerFunc(app.createRoomHandler)))
	mux.Handle("POST /api/invites/{token}/join", app.auth.Require(http.HandlerFunc(app.joinRoomHandler)))
	mux.Handle("POST /api/rooms/{roomID}/members/{memberID}/kick", app.auth.Require(http.HandlerFunc(app.kickMemberHandler)))
	mux.Handle("POST /api/rooms/{roomID}/lock", app.auth.Require(http.HandlerFunc(app.lockRoomHandler)))
	mux.Handle("POST /api/rooms/{roomID}/host/{memberID}", app.auth.Require(http.HandlerFunc(app.transferHostHandler)))
	mux.Handle("GET /api/rooms/{roomID}/video", app.auth.Require(http.HandlerFunc(app.videoHandler)))
	mux.Handle("GET /ws", app.auth.Require(http.HandlerFunc(app.websocketHandler)))
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))
	// Uploads and range-based video streaming can outlive a fixed whole-request
	// timeout. ReadHeaderTimeout remains enabled, and upload size is bounded by
	// the configured MaxUploadBytes limit.
	return &http.Server{Addr: cfg.Addr, Handler: securityHeaders(sameOrigin(cfg.AllowedOrigins, mux)), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}, app, nil
}

func (a *Application) Close() error { return a.store.Close() }

func (a *Application) CloseConnections() {
	a.hub.Close()
}
