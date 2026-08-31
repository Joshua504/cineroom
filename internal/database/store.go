package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("record not found")

type Store struct{ db *sql.DB }

type User struct {
	ID            string
	Email         string
	Username      string
	GoogleSubject string
	PasswordHash  string
	CreatedAt     time.Time
}

type Video struct {
	ID           string
	OwnerID      string
	OriginalName string
	StorageKey   string
	ContentType  string
	SizeBytes    int64
	CreatedAt    time.Time
}

type Room struct {
	ID              string
	HostID          string
	VideoID         string
	Title           string
	InviteToken     string
	InviteExpiresAt time.Time
	Locked          bool
	Playing         bool
	Position        float64
	Version         int64
	UpdatedAt       time.Time
	CreatedAt       time.Time
}

type ChatMessage struct {
	ID         string
	RoomID     string
	SenderID   string
	SenderName string
	Body       string
	CreatedAt  time.Time
}

type PendingRegistration struct {
	ID, Email, Username, PasswordHash, CodeHash string
	ExpiresAt, CreatedAt                        time.Time
}

func (s *Store) UserByGoogleSubject(ctx context.Context, subject string) (User, error) {
	var u User
	err := s.db.QueryRowContext(ctx, `SELECT id,email,username,google_subject,password_hash,created_at FROM users WHERE google_subject=?`, subject).Scan(&u.ID, &u.Email, &u.Username, &u.GoogleSubject, &u.PasswordHash, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return u, err
}

func (s *Store) CreateGoogleUser(ctx context.Context, email, subject, username, passwordHash string) (User, error) {
	u := User{ID: uuid.NewString(), Email: email, Username: username, GoogleSubject: subject, PasswordHash: passwordHash, CreatedAt: time.Now().UTC()}
	_, err := s.db.ExecContext(ctx, `INSERT INTO users(id,email,username,google_subject,password_hash,created_at) VALUES(?,?,?,?,?,?)`, u.ID, u.Email, u.Username, u.GoogleSubject, u.PasswordHash, u.CreatedAt)
	return u, err
}

func (s *Store) LinkGoogleSubject(ctx context.Context, userID, subject string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET google_subject=? WHERE id=? AND (google_subject IS NULL OR google_subject='')`, subject, userID)
	return err
}

func (s *Store) VideosOwnedBy(ctx context.Context, userID string) ([]Video, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,owner_id,original_name,storage_key,content_type,size_bytes,created_at FROM videos WHERE owner_id=? ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var videos []Video
	for rows.Next() {
		var v Video
		if err := rows.Scan(&v.ID, &v.OwnerID, &v.OriginalName, &v.StorageKey, &v.ContentType, &v.SizeBytes, &v.CreatedAt); err != nil {
			return nil, err
		}
		videos = append(videos, v)
	}
	return videos, rows.Err()
}

func (s *Store) RoomsForMember(ctx context.Context, userID string) ([]Room, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT r.id,r.host_id,r.video_id,r.title,r.invite_token,r.playing,r.position,r.version,r.updated_at,r.created_at,r.invite_expires_at,r.locked FROM rooms r JOIN room_members m ON m.room_id=r.id WHERE m.user_id=? ORDER BY r.updated_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var rooms []Room
	for rows.Next() {
		var r Room
		if err := rows.Scan(&r.ID, &r.HostID, &r.VideoID, &r.Title, &r.InviteToken, &r.Playing, &r.Position, &r.Version, &r.UpdatedAt, &r.CreatedAt, &r.InviteExpiresAt, &r.Locked); err != nil {
			return nil, err
		}
		rooms = append(rooms, r)
	}
	return rooms, rows.Err()
}

func (s *Store) ChatHistory(ctx context.Context, roomID, userID string, limit int) ([]ChatMessage, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `SELECT c.id,c.room_id,c.sender_id,u.username,c.body,c.created_at FROM chat_messages c JOIN users u ON u.id=c.sender_id WHERE c.room_id=? AND EXISTS (SELECT 1 FROM room_members WHERE room_id=? AND user_id=?) ORDER BY c.created_at DESC LIMIT ?`, roomID, roomID, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var messages []ChatMessage
	for rows.Next() {
		var m ChatMessage
		if err := rows.Scan(&m.ID, &m.RoomID, &m.SenderID, &m.SenderName, &m.Body, &m.CreatedAt); err != nil {
			return nil, err
		}
		messages = append(messages, m)
	}
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
	return messages, rows.Err()
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	store := &Store{db: db}
	if err := store.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
	CREATE TABLE IF NOT EXISTS users (
		id TEXT PRIMARY KEY, email TEXT NOT NULL UNIQUE COLLATE NOCASE,
		username TEXT NOT NULL UNIQUE COLLATE NOCASE, google_subject TEXT UNIQUE,
		password_hash TEXT NOT NULL, created_at DATETIME NOT NULL
	);
	CREATE TABLE IF NOT EXISTS videos (
		id TEXT PRIMARY KEY, owner_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		original_name TEXT NOT NULL, storage_key TEXT NOT NULL UNIQUE, content_type TEXT NOT NULL,
		size_bytes INTEGER NOT NULL CHECK(size_bytes >= 0), created_at DATETIME NOT NULL
	);
	CREATE TABLE IF NOT EXISTS rooms (
		id TEXT PRIMARY KEY, host_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		video_id TEXT NOT NULL REFERENCES videos(id) ON DELETE RESTRICT, title TEXT NOT NULL,
		invite_token TEXT NOT NULL UNIQUE, playing BOOLEAN NOT NULL DEFAULT FALSE,
		invite_expires_at DATETIME NOT NULL,
		locked BOOLEAN NOT NULL DEFAULT FALSE,
		position REAL NOT NULL DEFAULT 0 CHECK(position >= 0), version INTEGER NOT NULL DEFAULT 1,
		updated_at DATETIME NOT NULL, created_at DATETIME NOT NULL
	);
	CREATE TABLE IF NOT EXISTS room_members (
		room_id TEXT NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
		user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		role TEXT NOT NULL CHECK(role IN ('host', 'member')), joined_at DATETIME NOT NULL,
		PRIMARY KEY(room_id, user_id)
	);
	CREATE TABLE IF NOT EXISTS chat_messages (
		id TEXT PRIMARY KEY, room_id TEXT NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
		sender_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE, body TEXT NOT NULL,
		created_at DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS chat_messages_room_created ON chat_messages(room_id, created_at);
	CREATE TABLE IF NOT EXISTS pending_registrations (
		id TEXT PRIMARY KEY, email TEXT NOT NULL UNIQUE COLLATE NOCASE,
		username TEXT NOT NULL, password_hash TEXT NOT NULL, code_hash TEXT NOT NULL,
		expires_at DATETIME NOT NULL, created_at DATETIME NOT NULL
	);
	CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY, user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		token_hash TEXT NOT NULL UNIQUE, expires_at DATETIME NOT NULL, created_at DATETIME NOT NULL
	);
	`)
	if err != nil {
		return err
	}
	_, _ = s.db.ExecContext(ctx, `ALTER TABLE rooms ADD COLUMN invite_expires_at DATETIME`)
	_, _ = s.db.ExecContext(ctx, `ALTER TABLE rooms ADD COLUMN locked BOOLEAN NOT NULL DEFAULT FALSE`)
	_, _ = s.db.ExecContext(ctx, `UPDATE rooms SET invite_expires_at=datetime(created_at, '+1 day') WHERE invite_expires_at IS NULL`)

	// Existing development databases predate usernames and Google identities.
	_, _ = s.db.ExecContext(ctx, `ALTER TABLE users ADD COLUMN username TEXT`)
	_, _ = s.db.ExecContext(ctx, `ALTER TABLE users ADD COLUMN google_subject TEXT`)
	_, _ = s.db.ExecContext(ctx, `UPDATE users SET username='user_' || substr(id, 1, 8) WHERE username IS NULL OR username=''`)
	_, _ = s.db.ExecContext(ctx, `CREATE UNIQUE INDEX IF NOT EXISTS users_username_unique ON users(username COLLATE NOCASE)`)
	_, _ = s.db.ExecContext(ctx, `CREATE UNIQUE INDEX IF NOT EXISTS users_google_subject_unique ON users(google_subject) WHERE google_subject IS NOT NULL`)
	return nil
}

func (s *Store) CreatePendingRegistration(ctx context.Context, p PendingRegistration) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO pending_registrations(id,email,username,password_hash,code_hash,expires_at,created_at) VALUES(?,?,?,?,?,?,?) ON CONFLICT(email) DO UPDATE SET id=excluded.id,username=excluded.username,password_hash=excluded.password_hash,code_hash=excluded.code_hash,expires_at=excluded.expires_at,created_at=excluded.created_at`, p.ID, p.Email, p.Username, p.PasswordHash, p.CodeHash, p.ExpiresAt, p.CreatedAt)
	return err
}

func (s *Store) PendingRegistration(ctx context.Context, email string) (PendingRegistration, error) {
	var p PendingRegistration
	err := s.db.QueryRowContext(ctx, `SELECT id,email,username,password_hash,code_hash,expires_at,created_at FROM pending_registrations WHERE email=?`, email).Scan(&p.ID, &p.Email, &p.Username, &p.PasswordHash, &p.CodeHash, &p.ExpiresAt, &p.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return PendingRegistration{}, ErrNotFound
	}
	return p, err
}

func (s *Store) DeletePendingRegistration(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM pending_registrations WHERE id=?`, id)
	return err
}

// Session management for refresh tokens
func (s *Store) CreateSession(ctx context.Context, id, userID, tokenHash string, expiresAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO sessions(id,user_id,token_hash,expires_at,created_at) VALUES(?,?,?,?,?)`, id, userID, tokenHash, expiresAt, time.Now().UTC())
	return err
}

func (s *Store) SessionByTokenHash(ctx context.Context, tokenHash string) (string, string, time.Time, error) {
	var id, userID string
	var expires time.Time
	err := s.db.QueryRowContext(ctx, `SELECT id,user_id,expires_at FROM sessions WHERE token_hash=?`, tokenHash).Scan(&id, &userID, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", time.Time{}, ErrNotFound
	}
	return id, userID, expires, err
}

func (s *Store) DeleteSessionByHash(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash=?`, tokenHash)
	return err
}

func (s *Store) DeleteSessionsForUser(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id=?`, userID)
	return err
}

func (s *Store) CreateUser(ctx context.Context, email, username, passwordHash string) (User, error) {
	u := User{ID: uuid.NewString(), Email: email, Username: username, PasswordHash: passwordHash, CreatedAt: time.Now().UTC()}
	_, err := s.db.ExecContext(ctx, `INSERT INTO users(id,email,username,password_hash,created_at) VALUES(?,?,?,?,?)`, u.ID, u.Email, u.Username, u.PasswordHash, u.CreatedAt)
	return u, err
}

func (s *Store) UserByEmail(ctx context.Context, email string) (User, error) {
	var u User
	err := s.db.QueryRowContext(ctx, `SELECT id,email,username,google_subject,password_hash,created_at FROM users WHERE email=?`, email).Scan(&u.ID, &u.Email, &u.Username, &u.GoogleSubject, &u.PasswordHash, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return u, err
}

func (s *Store) CreateVideo(ctx context.Context, v Video) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO videos(id,owner_id,original_name,storage_key,content_type,size_bytes,created_at) VALUES(?,?,?,?,?,?,?)`, v.ID, v.OwnerID, v.OriginalName, v.StorageKey, v.ContentType, v.SizeBytes, v.CreatedAt)
	return err
}

func (s *Store) UserByID(ctx context.Context, id string) (User, error) {
	var u User
	err := s.db.QueryRowContext(ctx, `SELECT id,email,username,google_subject,password_hash,created_at FROM users WHERE id=?`, id).Scan(&u.ID, &u.Email, &u.Username, &u.GoogleSubject, &u.PasswordHash, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return u, err
}

func (s *Store) VideoOwnedBy(ctx context.Context, videoID, userID string) (Video, error) {
	var v Video
	err := s.db.QueryRowContext(ctx, `SELECT id,owner_id,original_name,storage_key,content_type,size_bytes,created_at FROM videos WHERE id=? AND owner_id=?`, videoID, userID).Scan(&v.ID, &v.OwnerID, &v.OriginalName, &v.StorageKey, &v.ContentType, &v.SizeBytes, &v.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Video{}, ErrNotFound
	}
	return v, err
}

func (s *Store) VideoForRoomMember(ctx context.Context, roomID, userID string) (Video, error) {
	var v Video
	err := s.db.QueryRowContext(ctx, `SELECT v.id,v.owner_id,v.original_name,v.storage_key,v.content_type,v.size_bytes,v.created_at FROM videos v JOIN rooms r ON r.video_id=v.id JOIN room_members m ON m.room_id=r.id WHERE r.id=? AND m.user_id=?`, roomID, userID).Scan(&v.ID, &v.OwnerID, &v.OriginalName, &v.StorageKey, &v.ContentType, &v.SizeBytes, &v.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Video{}, ErrNotFound
	}
	return v, err
}

func (s *Store) CreateRoom(ctx context.Context, room Room) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var owner string
	if err := tx.QueryRowContext(ctx, `SELECT owner_id FROM videos WHERE id=?`, room.VideoID).Scan(&owner); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if owner != room.HostID {
		return errors.New("only the video owner can create a room")
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO rooms(id,host_id,video_id,title,invite_token,invite_expires_at,locked,playing,position,version,updated_at,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, room.ID, room.HostID, room.VideoID, room.Title, room.InviteToken, room.InviteExpiresAt, room.Locked, room.Playing, room.Position, room.Version, room.UpdatedAt, room.CreatedAt)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO room_members(room_id,user_id,role,joined_at) VALUES(?,?,?,?)`, room.ID, room.HostID, "host", room.CreatedAt)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) JoinRoom(ctx context.Context, inviteToken, userID string) (Room, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Room{}, err
	}
	defer tx.Rollback()
	r, err := scanRoom(tx.QueryRowContext(ctx, `SELECT id,host_id,video_id,title,invite_token,playing,position,version,updated_at,created_at,invite_expires_at,locked FROM rooms WHERE invite_token=? AND (invite_expires_at IS NULL OR invite_expires_at > ?) AND (locked=FALSE OR EXISTS (SELECT 1 FROM room_members WHERE room_id=rooms.id AND user_id=?))`, inviteToken, time.Now().UTC(), userID))
	if err != nil {
		return Room{}, err
	}
	_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO room_members(room_id,user_id,role,joined_at) VALUES(?,?,?,?)`, r.ID, userID, "member", time.Now().UTC())
	if err != nil {
		return Room{}, err
	}
	return r, tx.Commit()
}

func (s *Store) RoomForMember(ctx context.Context, roomID, userID string) (Room, error) {
	r, err := scanRoom(s.db.QueryRowContext(ctx, `SELECT r.id,r.host_id,r.video_id,r.title,r.invite_token,r.playing,r.position,r.version,r.updated_at,r.created_at,r.invite_expires_at,r.locked FROM rooms r JOIN room_members m ON m.room_id=r.id WHERE r.id=? AND m.user_id=?`, roomID, userID))
	return r, err
}

func (s *Store) UpdatePlayback(ctx context.Context, roomID, userID string, playing bool, position float64) (Room, error) {
	if position < 0 || position > 60*60*24 {
		return Room{}, errors.New("invalid playback position")
	}
	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `UPDATE rooms SET playing=?,position=?,version=version+1,updated_at=? WHERE id=? AND EXISTS (SELECT 1 FROM room_members WHERE room_id=rooms.id AND user_id=?)`, playing, position, now, roomID, userID)
	if err != nil {
		return Room{}, err
	}
	changed, _ := result.RowsAffected()
	if changed == 0 {
		return Room{}, ErrNotFound
	}
	return s.RoomForMember(ctx, roomID, userID)
}

func (s *Store) AddChatMessage(ctx context.Context, roomID, senderID, body string) (ChatMessage, error) {
	if len(body) == 0 || len(body) > 1000 {
		return ChatMessage{}, errors.New("chat messages must contain 1 to 1000 characters")
	}
	var senderName string
	err := s.db.QueryRowContext(ctx, `SELECT username FROM users WHERE id=? AND EXISTS (SELECT 1 FROM room_members WHERE room_id=? AND user_id=?)`, senderID, roomID, senderID).Scan(&senderName)
	if errors.Is(err, sql.ErrNoRows) {
		return ChatMessage{}, ErrNotFound
	}
	if err != nil {
		return ChatMessage{}, err
	}
	m := ChatMessage{ID: uuid.NewString(), RoomID: roomID, SenderID: senderID, SenderName: senderName, Body: body, CreatedAt: time.Now().UTC()}
	_, err = s.db.ExecContext(ctx, `INSERT INTO chat_messages(id,room_id,sender_id,body,created_at) VALUES(?,?,?,?,?)`, m.ID, m.RoomID, m.SenderID, m.Body, m.CreatedAt)
	return m, err
}

func (s *Store) KickMember(ctx context.Context, roomID, hostID, memberID string) error {
	if hostID == memberID {
		return errors.New("the host cannot remove themselves")
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM room_members WHERE room_id=? AND user_id=? AND EXISTS (SELECT 1 FROM rooms WHERE id=? AND host_id=?)`, roomID, memberID, roomID, hostID)
	if err != nil {
		return err
	}
	changed, _ := result.RowsAffected()
	if changed == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SetRoomLocked(ctx context.Context, roomID, hostID string, locked bool) (Room, error) {
	result, err := s.db.ExecContext(ctx, `UPDATE rooms SET locked=?,version=version+1,updated_at=? WHERE id=? AND host_id=?`, locked, time.Now().UTC(), roomID, hostID)
	if err != nil {
		return Room{}, err
	}
	changed, _ := result.RowsAffected()
	if changed == 0 {
		return Room{}, ErrNotFound
	}
	return s.RoomForMember(ctx, roomID, hostID)
}

func (s *Store) TransferHost(ctx context.Context, roomID, hostID, memberID string) (Room, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Room{}, err
	}
	defer tx.Rollback()
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM room_members WHERE room_id=? AND user_id=? AND EXISTS (SELECT 1 FROM rooms WHERE id=? AND host_id=?)`, roomID, memberID, roomID, hostID).Scan(&exists); err != nil {
		return Room{}, ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, `UPDATE room_members SET role='member' WHERE room_id=? AND user_id=?`, roomID, hostID); err != nil {
		return Room{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE room_members SET role='host' WHERE room_id=? AND user_id=?`, roomID, memberID); err != nil {
		return Room{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE rooms SET host_id=?,version=version+1,updated_at=? WHERE id=? AND host_id=?`, memberID, time.Now().UTC(), roomID, hostID); err != nil {
		return Room{}, err
	}
	if err := tx.Commit(); err != nil {
		return Room{}, err
	}
	return s.RoomForMember(ctx, roomID, memberID)
}

func scanRoom(row *sql.Row) (Room, error) {
	var r Room
	err := row.Scan(&r.ID, &r.HostID, &r.VideoID, &r.Title, &r.InviteToken, &r.Playing, &r.Position, &r.Version, &r.UpdatedAt, &r.CreatedAt, &r.InviteExpiresAt, &r.Locked)
	if errors.Is(err, sql.ErrNoRows) {
		return Room{}, ErrNotFound
	}
	if err != nil {
		return Room{}, fmt.Errorf("scan room: %w", err)
	}
	return r, nil
}
