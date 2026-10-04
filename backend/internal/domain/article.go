package domain

import (
	"errors"
	"strings"
	"time"
)

type Status string

const (
	Draft       Status = "draft"
	Published   Status = "published"
	Unpublished Status = "unpublished"
	Deleted     Status = "deleted"
)

type Role string

const (
	Reader Role = "reader"
	Author Role = "author"
	Admin  Role = "admin"
)

type User struct {
	ID   int64
	Role Role
}

type Article struct {
	ID             int64
	AuthorID       int64
	Title          string
	Body           string
	Status         Status
	PreviousStatus Status
	PublishedAt    time.Time
	DeletedAt      time.Time
}

var ErrTransition = errors.New("invalid article state transition")
var ErrIncomplete = errors.New("title and body are required")

func (a *Article) Publish(now time.Time) error {
	if a.Status != Draft && a.Status != Unpublished {
		return ErrTransition
	}
	if strings.TrimSpace(a.Title) == "" || strings.TrimSpace(a.Body) == "" {
		return ErrIncomplete
	}
	a.Status = Published
	a.PublishedAt = now
	return nil
}

func (a *Article) Unpublish() error {
	if a.Status != Published {
		return ErrTransition
	}
	a.Status = Unpublished
	return nil
}

func (a *Article) Delete(now time.Time) error {
	if a.Status == Deleted || (a.Status != Draft && a.Status != Published && a.Status != Unpublished) {
		return ErrTransition
	}
	a.PreviousStatus = a.Status
	a.Status = Deleted
	a.DeletedAt = now
	return nil
}

func (a *Article) Restore() error {
	if a.Status != Deleted || a.PreviousStatus == "" {
		return ErrTransition
	}
	a.Status = a.PreviousStatus
	a.PreviousStatus = ""
	a.DeletedAt = time.Time{}
	return nil
}

func (a Article) VisibleToReader() bool { return a.Status == Published }

func CanManageArticle(u User, a Article) bool {
	return u.Role == Admin || (u.Role == Author && u.ID == a.AuthorID)
}

func CanDeleteComment(u User, commenterID, articleAuthorID int64) bool {
	return u.Role == Admin || u.ID == commenterID || (u.Role == Author && u.ID == articleAuthorID)
}
