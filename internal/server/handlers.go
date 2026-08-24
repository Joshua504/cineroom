package server

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Joshua504/cineroom/internal/auth"
	"github.com/Joshua504/cineroom/internal/database"
	"github.com/Joshua504/cineroom/internal/websocket"
	"github.com/google/uuid"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

func (a *Application) homeHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "web/templates/index.html")
}

func (a *Application) registerHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Username string `json:"username"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	email := auth.NormalizeEmail(input.Email)
	username := auth.NormalizeUsername(input.Username)
	if len(email) < 3 || len(email) > 254 || !strings.Contains(email, "@") {
		writeError(w, http.StatusBadRequest, "invalid_email", "a valid email is required")
		return
	}
	if !auth.ValidUsername(username) {
		writeError(w, http.StatusBadRequest, "invalid_username", "username must be 3-32 lowercase letters, numbers, or underscores")
		return
	}
	hash, err := auth.HashPassword(input.Password)
	if err != nil {
		writeError(w, http.StatusBadRequest, "weak_password", err.Error())
		return
	}
	if _, err := a.store.UserByEmail(r.Context(), email); err == nil {
		writeError(w, http.StatusConflict, "email_in_use", "an account already uses that email")
		return
	}
	code, err := otpCode()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "otp_error", "could not create verification code")
		return
	}
	pending := database.PendingRegistration{ID: uuid.NewString(), Email: email, Username: username, PasswordHash: hash, CodeHash: a.otpHash(code), ExpiresAt: time.Now().UTC().Add(10 * time.Minute), CreatedAt: time.Now().UTC()}
	if err := a.store.CreatePendingRegistration(r.Context(), pending); err != nil {
		writeError(w, http.StatusConflict, "registration_error", "could not start registration")
		return
	}
	if err := a.mailer.SendOTP(email, code); err != nil {
		_ = a.store.DeletePendingRegistration(r.Context(), pending.ID)
		a.logger.Printf("sending registration OTP: %v", err)
		writeError(w, http.StatusServiceUnavailable, "email_unavailable", "verification email could not be sent")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"email": email, "message": "verification code sent"})
}

func (a *Application) verifyRegistrationHandler(w http.ResponseWriter, r *http.Request) {
	var input struct{ Email, Code string }
	if !decodeJSON(w, r, &input) {
		return
	}
	email := auth.NormalizeEmail(input.Email)
	if len(input.Code) != 6 {
		writeError(w, http.StatusBadRequest, "invalid_code", "verification code must contain 6 digits")
		return
	}
	pending, err := a.store.PendingRegistration(r.Context(), email)
	if err != nil || time.Now().UTC().After(pending.ExpiresAt) || pending.CodeHash != a.otpHash(input.Code) {
		writeError(w, http.StatusUnauthorized, "invalid_code", "verification code is invalid or expired")
		return
	}
	user, err := a.store.CreateUser(r.Context(), pending.Email, pending.Username, pending.PasswordHash)
	if err != nil {
		writeError(w, http.StatusConflict, "registration_error", "email or username is already in use")
		return
	}
	_ = a.store.DeletePendingRegistration(r.Context(), pending.ID)
	if err := a.auth.SetSession(w, auth.User{ID: user.ID, Email: user.Email, Username: user.Username}); err != nil {
		writeError(w, http.StatusInternalServerError, "session_error", "could not create session")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": user.ID, "email": user.Email, "username": user.Username})
}

func otpCode() (string, error) {
	var bytes [4]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", binary.BigEndian.Uint32(bytes[:])%1000000), nil
}

func (a *Application) otpHash(code string) string {
	sum := sha256.Sum256(append(a.config.SessionSecret, []byte(code)...))
	return hex.EncodeToString(sum[:])
}

func (a *Application) loginHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	user, err := a.store.UserByEmail(r.Context(), auth.NormalizeEmail(input.Email))
	if err != nil || auth.CheckPassword(user.PasswordHash, input.Password) != nil {
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "invalid email or password")
		return
	}
	if err := a.auth.SetSession(w, auth.User{ID: user.ID, Email: user.Email, Username: user.Username}); err != nil {
		writeError(w, http.StatusInternalServerError, "session_error", "could not create session")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": user.ID, "email": user.Email, "username": user.Username})
}

func (a *Application) logoutHandler(w http.ResponseWriter, r *http.Request) {
	a.auth.ClearSession(w)
	w.WriteHeader(http.StatusNoContent)
}

func (a *Application) meHandler(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.FromContext(r.Context())
	writeJSON(w, http.StatusOK, u)
}

func (a *Application) listVideosHandler(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.FromContext(r.Context())
	videos, err := a.store.VideosOwnedBy(r.Context(), u.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "videos_error", "could not load videos")
		return
	}
	result := make([]map[string]any, 0, len(videos))
	for _, v := range videos {
		result = append(result, map[string]any{"id": v.ID, "name": v.OriginalName, "sizeBytes": v.SizeBytes, "createdAt": v.CreatedAt})
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *Application) listRoomsHandler(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.FromContext(r.Context())
	rooms, err := a.store.RoomsForMember(r.Context(), u.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "rooms_error", "could not load rooms")
		return
	}
	result := make([]map[string]any, 0, len(rooms))
	for _, room := range rooms {
		result = append(result, roomResponse(room))
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *Application) chatHistoryHandler(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.FromContext(r.Context())
	messages, err := a.store.ChatHistory(r.Context(), r.PathValue("roomID"), u.ID, 50)
	if err != nil {
		writeError(w, http.StatusForbidden, "chat_forbidden", "room membership required")
		return
	}
	result := make([]map[string]any, 0, len(messages))
	for _, m := range messages {
		result = append(result, map[string]any{"id": m.ID, "text": m.Body, "senderId": m.SenderID, "senderName": m.SenderName, "createdAt": m.CreatedAt})
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *Application) googleStartHandler(w http.ResponseWriter, r *http.Request) {
	if a.config.GoogleClientID == "" || a.config.GoogleClientSecret == "" {
		writeError(w, http.StatusNotImplemented, "oauth_unconfigured", "Google sign-in is not configured")
		return
	}
	stateBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		writeError(w, http.StatusInternalServerError, "oauth_state_error", "could not start sign-in")
		return
	}
	state := hex.EncodeToString(stateBytes)
	http.SetCookie(w, &http.Cookie{Name: "cineroom_oauth_state", Value: state, Path: "/api/auth/google", HttpOnly: true, Secure: strings.HasPrefix(a.config.AppOrigin, "https://"), SameSite: http.SameSiteLaxMode, MaxAge: 600})
	w.Header().Set("Location", a.googleOAuthConfig().AuthCodeURL(state, oauth2.AccessTypeOffline))
	w.WriteHeader(http.StatusFound)
}

func (a *Application) googleCallbackHandler(w http.ResponseWriter, r *http.Request) {
	stateCookie, err := r.Cookie("cineroom_oauth_state")
	if err != nil || stateCookie.Value == "" || r.URL.Query().Get("state") != stateCookie.Value {
		writeError(w, http.StatusBadRequest, "oauth_state_invalid", "invalid sign-in state")
		return
	}
	if r.URL.Query().Get("error") != "" {
		http.Redirect(w, r, "/?oauth=cancelled", http.StatusFound)
		return
	}
	token, err := a.googleOAuthConfig().Exchange(r.Context(), r.URL.Query().Get("code"))
	if err != nil {
		writeError(w, http.StatusUnauthorized, "oauth_exchange_failed", "Google sign-in failed")
		return
	}
	req, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, "https://openidconnect.googleapis.com/v1/userinfo", nil)
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			resp.Body.Close()
		}
		writeError(w, http.StatusUnauthorized, "oauth_profile_failed", "could not read Google profile")
		return
	}
	defer resp.Body.Close()
	var profile struct {
		Sub, Email, Name string
		EmailVerified    bool `json:"email_verified"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil || profile.Sub == "" || !profile.EmailVerified {
		writeError(w, http.StatusUnauthorized, "oauth_profile_invalid", "Google account could not be verified")
		return
	}
	email := auth.NormalizeEmail(profile.Email)
	user, err := a.store.UserByGoogleSubject(r.Context(), profile.Sub)
	if errors.Is(err, database.ErrNotFound) {
		user, err = a.store.UserByEmail(r.Context(), email)
		if errors.Is(err, database.ErrNotFound) {
			username := oauthUsername(email)
			hash, _ := auth.HashPassword(uuid.NewString())
			user, err = a.store.CreateGoogleUser(r.Context(), email, profile.Sub, username, hash)
		} else if err == nil {
			err = a.store.LinkGoogleSubject(r.Context(), user.ID, profile.Sub)
		}
	}
	if err != nil {
		writeError(w, http.StatusConflict, "oauth_account_error", "could not create or load account")
		return
	}
	if err := a.auth.SetSession(w, auth.User{ID: user.ID, Email: user.Email, Username: user.Username}); err != nil {
		writeError(w, http.StatusInternalServerError, "session_error", "could not create session")
		return
	}
	http.Redirect(w, r, "/", http.StatusFound)
}

func (a *Application) googleOAuthConfig() *oauth2.Config {
	return &oauth2.Config{ClientID: a.config.GoogleClientID, ClientSecret: a.config.GoogleClientSecret, Endpoint: google.Endpoint, RedirectURL: strings.TrimRight(a.config.AppOrigin, "/") + "/api/auth/google/callback", Scopes: []string{"openid", "email", "profile"}}
}

var nonUsername = regexp.MustCompile(`[^a-z0-9_]+`)

func oauthUsername(email string) string {
	base := strings.Split(email, "@")[0]
	base = nonUsername.ReplaceAllString(strings.ToLower(base), "_")
	base = strings.Trim(base, "_")
	if len(base) < 3 {
		base = "google_user"
	}
	if len(base) > 24 {
		base = base[:24]
	}
	return base + "_" + uuid.NewString()[:7]
}

func (a *Application) uploadVideoHandler(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.FromContext(r.Context())
	r.Body = http.MaxBytesReader(w, r.Body, a.config.MaxUploadBytes+1024*1024)
	if err := r.ParseMultipartForm(1024 * 1024); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_upload", "could not read upload")
		return
	}
	file, header, err := r.FormFile("video")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing_video", "video file is required")
		return
	}
	defer file.Close()
	saved, err := a.media.Save(file, header.Filename, header.Header.Get("Content-Type"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_video", err.Error())
		return
	}
	v := database.Video{ID: uuid.NewString(), OwnerID: user.ID, OriginalName: filepath.Base(header.Filename), StorageKey: saved.StorageKey, ContentType: saved.ContentType, SizeBytes: saved.Size, CreatedAt: time.Now().UTC()}
	if err := a.store.CreateVideo(r.Context(), v); err != nil {
		writeError(w, http.StatusInternalServerError, "video_save_error", "could not save video metadata")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": v.ID, "name": v.OriginalName, "sizeBytes": v.SizeBytes})
}

func (a *Application) createRoomHandler(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.FromContext(r.Context())
	var input struct {
		VideoID string `json:"videoId"`
		Title   string `json:"title"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Title = strings.TrimSpace(input.Title)
	if input.Title == "" || len(input.Title) > 120 {
		writeError(w, http.StatusBadRequest, "invalid_title", "title must contain 1 to 120 characters")
		return
	}
	now := time.Now().UTC()
	room := database.Room{ID: uuid.NewString(), HostID: user.ID, VideoID: input.VideoID, Title: input.Title, InviteToken: uuid.NewString(), InviteExpiresAt: now.Add(24 * time.Hour), Version: 1, UpdatedAt: now, CreatedAt: now}
	if err := a.store.CreateRoom(r.Context(), room); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, database.ErrNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, "room_create_error", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, roomResponse(room))
}

func (a *Application) joinRoomHandler(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.FromContext(r.Context())
	room, err := a.store.JoinRoom(r.Context(), r.PathValue("token"), user.ID)
	if err != nil {
		writeError(w, http.StatusNotFound, "invalid_invite", "invite not found")
		return
	}
	writeJSON(w, http.StatusOK, roomResponse(room))
}

func (a *Application) kickMemberHandler(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.FromContext(r.Context())
	roomID, memberID := r.PathValue("roomID"), r.PathValue("memberID")
	if err := a.store.KickMember(r.Context(), roomID, user.ID, memberID); err != nil {
		writeError(w, http.StatusForbidden, "kick_failed", err.Error())
		return
	}
	a.hub.DisconnectMember(roomID, memberID)
	w.WriteHeader(http.StatusNoContent)
}

func (a *Application) lockRoomHandler(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.FromContext(r.Context())
	var input struct {
		Locked bool `json:"locked"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	room, err := a.store.SetRoomLocked(r.Context(), r.PathValue("roomID"), user.ID, input.Locked)
	if err != nil {
		writeError(w, http.StatusForbidden, "lock_failed", "only the host can change the room lock")
		return
	}
	a.hub.BroadcastRoomState(room)
	writeJSON(w, http.StatusOK, roomResponse(room))
}

func (a *Application) transferHostHandler(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.FromContext(r.Context())
	room, err := a.store.TransferHost(r.Context(), r.PathValue("roomID"), user.ID, r.PathValue("memberID"))
	if err != nil {
		writeError(w, http.StatusForbidden, "transfer_failed", "host transfer requires a room member")
		return
	}
	a.hub.BroadcastRoomState(room)
	writeJSON(w, http.StatusOK, roomResponse(room))
}

func (a *Application) videoHandler(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.FromContext(r.Context())
	video, err := a.store.VideoForRoomMember(r.Context(), r.PathValue("roomID"), user.ID)
	if err != nil {
		writeError(w, http.StatusForbidden, "video_forbidden", "room membership required")
		return
	}
	file, err := a.media.Open(video.StorageKey)
	if err != nil {
		writeError(w, http.StatusNotFound, "video_missing", "video file is unavailable")
		return
	}
	defer file.Close()
	w.Header().Set("Content-Type", video.ContentType)
	w.Header().Set("Content-Disposition", "inline; filename="+strconv.Quote(video.OriginalName))
	http.ServeContent(w, r, video.OriginalName, video.CreatedAt, file)
}

func (a *Application) websocketHandler(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.FromContext(r.Context())
	websocket.Handle(a.hub, a.store, user.ID, user.Username, a.config.AllowedOrigins, w, r)
}
func roomResponse(room database.Room) map[string]any {
	return map[string]any{"id": room.ID, "hostId": room.HostID, "videoId": room.VideoID, "title": room.Title, "inviteToken": room.InviteToken, "inviteExpiresAt": room.InviteExpiresAt, "locked": room.Locked, "playing": room.Playing, "position": room.Position, "version": room.Version}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request must contain valid JSON")
		return false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid_json", "request must contain one JSON object")
		return false
	}
	return true
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]string{"error": code, "message": message})
}
