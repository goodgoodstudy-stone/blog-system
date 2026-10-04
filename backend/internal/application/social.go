package application

import (
	"context"
	"strings"
	"unicode/utf8"

	"blog-system/backend/internal/domain"
	"blog-system/backend/internal/mysqlrepo"
)

type SocialRepository interface {
	GetArticle(context.Context, int64, int64) (mysqlrepo.Article, error)
	AddFavorite(context.Context, int64, int64) error
	CreateComment(context.Context, int64, int64, string) (mysqlrepo.Comment, error)
	GetComment(context.Context, int64) (mysqlrepo.Comment, error)
	DeleteComment(context.Context, int64) error
}

func (s Service) Favorite(ctx context.Context, u mysqlrepo.User, articleID int64) error {
	a, err := s.Social.GetArticle(ctx, articleID, 0)
	if err != nil {
		return err
	}
	if a.Status != "published" {
		return mysqlrepo.ErrNotFound
	}
	return s.Social.AddFavorite(ctx, u.ID, articleID)
}
func (s Service) Comment(ctx context.Context, u mysqlrepo.User, articleID int64, content string) (mysqlrepo.Comment, error) {
	content = strings.TrimSpace(content)
	if content == "" || utf8.RuneCountInString(content) > 1000 {
		return mysqlrepo.Comment{}, domain.ErrIncomplete
	}
	a, err := s.Social.GetArticle(ctx, articleID, 0)
	if err != nil {
		return mysqlrepo.Comment{}, err
	}
	if a.Status != "published" {
		return mysqlrepo.Comment{}, mysqlrepo.ErrNotFound
	}
	return s.Social.CreateComment(ctx, articleID, u.ID, content)
}
func (s Service) DeleteComment(ctx context.Context, u mysqlrepo.User, id int64) error {
	c, err := s.Social.GetComment(ctx, id)
	if err != nil {
		return err
	}
	a, err := s.Social.GetArticle(ctx, c.ArticleID, 0)
	if err != nil {
		return err
	}
	if !domain.CanDeleteComment(domain.User{ID: u.ID, Role: domain.Role(u.Role)}, c.UserID, a.AuthorID) {
		return mysqlrepo.ErrForbidden
	}
	return s.Social.DeleteComment(ctx, id)
}
