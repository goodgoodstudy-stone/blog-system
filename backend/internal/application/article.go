package application

import (
	"context"
	"strings"
	"time"

	"blog-system/backend/internal/domain"
	"blog-system/backend/internal/mysqlrepo"
	"go.opentelemetry.io/otel"
)

type ArticleRepository interface {
	GetArticle(context.Context, int64, int64) (mysqlrepo.Article, error)
	SaveArticle(context.Context, mysqlrepo.Article, []int64, int64) (mysqlrepo.Article, error)
	UpdateArticleState(context.Context, int64, int64, domain.Article) error
}
type Service struct {
	Articles ArticleRepository
	Social   SocialRepository
	Now      func() time.Time
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
func (s Service) LoadManage(ctx context.Context, u mysqlrepo.User, id int64) (mysqlrepo.Article, error) {
	if u.Role != "author" && u.Role != "admin" {
		return mysqlrepo.Article{}, mysqlrepo.ErrForbidden
	}
	a, err := s.Articles.GetArticle(ctx, id, u.ID)
	if err != nil {
		return a, err
	}
	if !domain.CanManageArticle(domain.User{ID: u.ID, Role: domain.Role(u.Role)}, domain.Article{AuthorID: a.AuthorID}) {
		return a, mysqlrepo.ErrForbidden
	}
	return a, nil
}
func (s Service) Save(ctx context.Context, u mysqlrepo.User, a mysqlrepo.Article, tagIDs []int64) (mysqlrepo.Article, error) {
	if u.Role != "author" && u.Role != "admin" {
		return mysqlrepo.Article{}, mysqlrepo.ErrForbidden
	}
	if strings.TrimSpace(a.Title) == "" || strings.TrimSpace(a.Body) == "" {
		return mysqlrepo.Article{}, domain.ErrIncomplete
	}
	if a.ID > 0 {
		old, err := s.LoadManage(ctx, u, a.ID)
		if err != nil {
			return old, err
		}
		if old.Status == "deleted" {
			return old, mysqlrepo.ErrConflict
		}
		a.AuthorID = old.AuthorID
	}
	return s.Articles.SaveArticle(ctx, a, tagIDs, u.ID)
}
func (s Service) Transition(ctx context.Context, u mysqlrepo.User, id int64, action string) (mysqlrepo.Article, error) {
	spanName := "app.Transition"
	switch action {
	case "publish":
		spanName = "app.Publish"
	case "unpublish":
		spanName = "app.Unpublish"
	case "delete":
		spanName = "app.Delete"
	case "restore":
		spanName = "app.Restore"
	}
	ctx, span := otel.Tracer("blog/application").Start(ctx, spanName)
	defer span.End()
	a, err := s.LoadManage(ctx, u, id)
	if err != nil {
		return a, err
	}
	d := domain.Article{ID: a.ID, AuthorID: a.AuthorID, Title: a.Title, Body: a.Body, Status: domain.Status(a.Status), PreviousStatus: domain.Status(a.PreviousStatus)}
	if a.PublishedAt != nil {
		d.PublishedAt = *a.PublishedAt
	}
	if a.DeletedAt != nil {
		d.DeletedAt = *a.DeletedAt
	}
	now := s.now()
	switch action {
	case "publish":
		err = d.Publish(now)
	case "unpublish":
		err = d.Unpublish()
	case "delete":
		err = d.Delete(now)
	case "restore":
		err = d.Restore()
	default:
		err = domain.ErrTransition
	}
	if err != nil {
		return a, err
	}
	if err = s.Articles.UpdateArticleState(ctx, id, a.Version, d); err != nil {
		return a, err
	}
	return s.Articles.GetArticle(ctx, id, u.ID)
}
