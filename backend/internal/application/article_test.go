package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"blog-system/backend/internal/domain"
	"blog-system/backend/internal/mysqlrepo"
)

type fakeArticles struct{ article mysqlrepo.Article }

func (f *fakeArticles) GetArticle(_ context.Context, id, _ int64) (mysqlrepo.Article, error) {
	if id != f.article.ID {
		return mysqlrepo.Article{}, mysqlrepo.ErrNotFound
	}
	return f.article, nil
}
func (f *fakeArticles) SaveArticle(_ context.Context, a mysqlrepo.Article, _ []int64, _ int64) (mysqlrepo.Article, error) {
	f.article = a
	return a, nil
}
func (f *fakeArticles) UpdateArticleState(_ context.Context, id, version int64, d domain.Article) error {
	if f.article.ID != id || f.article.Version != version {
		return mysqlrepo.ErrConflict
	}
	f.article.Status = string(d.Status)
	f.article.PreviousStatus = string(d.PreviousStatus)
	f.article.Version++
	return nil
}

func TestTransitionChecksOwnershipAndRestoresState(t *testing.T) {
	repo := &fakeArticles{article: mysqlrepo.Article{ID: 1, AuthorID: 2, Title: "文章", Body: "正文", Status: "draft", Version: 1}}
	svc := Service{Articles: repo, Now: func() time.Time { return time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC) }}
	if _, err := svc.Transition(context.Background(), mysqlrepo.User{ID: 3, Role: "author"}, 1, "publish"); !errors.Is(err, mysqlrepo.ErrForbidden) {
		t.Fatalf("other author: %v", err)
	}
	if _, err := svc.Transition(context.Background(), mysqlrepo.User{ID: 2, Role: "author"}, 1, "publish"); err != nil {
		t.Fatal(err)
	}
	if repo.article.Status != "published" {
		t.Fatal("not published")
	}
	if _, err := svc.Transition(context.Background(), mysqlrepo.User{ID: 2, Role: "author"}, 1, "delete"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Transition(context.Background(), mysqlrepo.User{ID: 2, Role: "author"}, 1, "restore"); err != nil {
		t.Fatal(err)
	}
	if repo.article.Status != "published" {
		t.Fatalf("restored as %s", repo.article.Status)
	}
}
