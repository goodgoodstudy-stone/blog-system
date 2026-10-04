package mysqlrepo

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

var ErrNotFound = errors.New("not found")
var ErrConflict = errors.New("conflict")
var ErrForbidden = errors.New("forbidden")

type Store struct{ DB *sql.DB }
type User struct {
	ID           int64  `json:"id"`
	Email        string `json:"email"`
	Nickname     string `json:"nickname"`
	Role         string `json:"role"`
	PasswordHash string `json:"-"`
}
type Tag struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}
type Article struct {
	ID             int64      `json:"id"`
	AuthorID       int64      `json:"authorId"`
	AuthorName     string     `json:"authorName"`
	Title          string     `json:"title"`
	Summary        string     `json:"summary"`
	Body           string     `json:"body"`
	Status         string     `json:"status"`
	PreviousStatus string     `json:"-"`
	PublishedAt    *time.Time `json:"publishedAt"`
	DeletedAt      *time.Time `json:"deletedAt,omitempty"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
	Version        int64      `json:"version"`
	Tags           []Tag      `json:"tags"`
	Favorited      bool       `json:"favorited"`
}
type Comment struct {
	ID        int64     `json:"id"`
	ArticleID int64     `json:"articleId"`
	UserID    int64     `json:"userId"`
	Nickname  string    `json:"nickname"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"createdAt"`
}
type Image struct {
	ID         int64
	UploaderID int64
	ArticleID  sql.NullInt64
	Filename   string
	Mime       string
	Size       int64
}
type Page[T any] struct {
	Items    []T   `json:"items"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"pageSize"`
}

func NormalizeEmail(v string) string { return strings.ToLower(strings.TrimSpace(v)) }
func TokenHash(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func (s Store) CreateUser(ctx context.Context, email, nickname, hash string) (User, error) {
	email = NormalizeEmail(email)
	r, err := s.DB.ExecContext(ctx, `INSERT INTO users(email,nickname,password_hash) VALUES(?,?,?)`, email, nickname, hash)
	if err != nil {
		return User{}, err
	}
	id, _ := r.LastInsertId()
	return User{ID: id, Email: email, Nickname: nickname, Role: "reader"}, nil
}
func (s Store) UserByEmail(ctx context.Context, email string) (User, error) {
	var u User
	err := s.DB.QueryRowContext(ctx, `SELECT id,email,nickname,role,password_hash FROM users WHERE email=?`, NormalizeEmail(email)).Scan(&u.ID, &u.Email, &u.Nickname, &u.Role, &u.PasswordHash)
	if errors.Is(err, sql.ErrNoRows) {
		return u, ErrNotFound
	}
	return u, err
}
func (s Store) UserByID(ctx context.Context, id int64) (User, error) {
	var u User
	err := s.DB.QueryRowContext(ctx, `SELECT id,email,nickname,role,password_hash FROM users WHERE id=?`, id).Scan(&u.ID, &u.Email, &u.Nickname, &u.Role, &u.PasswordHash)
	if errors.Is(err, sql.ErrNoRows) {
		return u, ErrNotFound
	}
	return u, err
}
func (s Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id,email,nickname,role FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		var u User
		if err = rows.Scan(&u.ID, &u.Email, &u.Nickname, &u.Role); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}
func (s Store) SetRole(ctx context.Context, id int64, role string) error {
	if role != "reader" && role != "author" {
		return ErrForbidden
	}
	r, err := s.DB.ExecContext(ctx, `UPDATE users SET role=? WHERE id=? AND role <> 'admin'`, role, id)
	if err != nil {
		return err
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		var current string
		err = s.DB.QueryRowContext(ctx, `SELECT role FROM users WHERE id=?`, id).Scan(&current)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if current == "admin" {
			return ErrForbidden
		}
		if current == role {
			return nil
		}
		return ErrConflict
	}
	return nil
}
func (s Store) CreateSession(ctx context.Context, userID int64, token string, expires time.Time) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO sessions(token_hash,user_id,expires_at) VALUES(?,?,?)`, TokenHash(token), userID, expires)
	return err
}
func (s Store) UserBySession(ctx context.Context, token string) (User, error) {
	var u User
	err := s.DB.QueryRowContext(ctx, `SELECT u.id,u.email,u.nickname,u.role FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=? AND s.expires_at > UTC_TIMESTAMP(6)`, TokenHash(token)).Scan(&u.ID, &u.Email, &u.Nickname, &u.Role)
	if errors.Is(err, sql.ErrNoRows) {
		return u, ErrNotFound
	}
	return u, err
}
func (s Store) DeleteSession(ctx context.Context, token string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash=?`, TokenHash(token))
	return err
}
