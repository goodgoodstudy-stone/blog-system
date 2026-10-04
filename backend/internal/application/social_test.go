package application

import (
	"context"
	"errors"
	"testing"

	"blog-system/backend/internal/mysqlrepo"
)

type fakeSocial struct {
	article        mysqlrepo.Article
	comment        mysqlrepo.Comment
	favoriteAdded  bool
	commentDeleted bool
}

func (f *fakeSocial) GetArticle(_ context.Context, id, _ int64) (mysqlrepo.Article, error) {
	if id != f.article.ID {
		return mysqlrepo.Article{}, mysqlrepo.ErrNotFound
	}
	return f.article, nil
}
func (f *fakeSocial) AddFavorite(_ context.Context, _, _ int64) error {
	f.favoriteAdded = true
	return nil
}
func (f *fakeSocial) CreateComment(_ context.Context, _, _ int64, content string) (mysqlrepo.Comment, error) {
	f.comment.Content = content
	return f.comment, nil
}
func (f *fakeSocial) GetComment(_ context.Context, id int64) (mysqlrepo.Comment, error) {
	if id != f.comment.ID {
		return mysqlrepo.Comment{}, mysqlrepo.ErrNotFound
	}
	return f.comment, nil
}
func (f *fakeSocial) DeleteComment(_ context.Context, _ int64) error {
	f.commentDeleted = true
	return nil
}

func TestSocialRules(t *testing.T) {
	f := &fakeSocial{article: mysqlrepo.Article{ID: 1, AuthorID: 2, Status: "draft"}, comment: mysqlrepo.Comment{ID: 9, ArticleID: 1, UserID: 4}}
	svc := Service{Social: f}
	reader := mysqlrepo.User{ID: 4, Role: "reader"}
	if err := svc.Favorite(context.Background(), reader, 1); !errors.Is(err, mysqlrepo.ErrNotFound) {
		t.Fatalf("draft favorite: %v", err)
	}
	if f.favoriteAdded {
		t.Fatal("draft favorited")
	}
	f.article.Status = "published"
	if err := svc.Favorite(context.Background(), reader, 1); err != nil {
		t.Fatal(err)
	}
	if !f.favoriteAdded {
		t.Fatal("favorite missing")
	}
	if err := svc.DeleteComment(context.Background(), mysqlrepo.User{ID: 3, Role: "author"}, 9); !errors.Is(err, mysqlrepo.ErrForbidden) {
		t.Fatalf("other author delete: %v", err)
	}
	if err := svc.DeleteComment(context.Background(), mysqlrepo.User{ID: 2, Role: "author"}, 9); err != nil {
		t.Fatal(err)
	}
	if !f.commentDeleted {
		t.Fatal("comment not deleted")
	}
}
