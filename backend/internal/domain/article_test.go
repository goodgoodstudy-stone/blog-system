package domain

import (
	"testing"
	"time"
)

func TestArticleLifecycleAndRestore(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	a := Article{AuthorID: 2, Status: Draft, Title: "Title", Body: "Body"}
	if err := a.Publish(now); err != nil {
		t.Fatal(err)
	}
	if a.Status != Published || !a.PublishedAt.Equal(now) {
		t.Fatalf("publish: %+v", a)
	}
	if err := a.Unpublish(); err != nil {
		t.Fatal(err)
	}
	if a.Status != Unpublished {
		t.Fatalf("unpublish: %s", a.Status)
	}
	if err := a.Delete(now); err != nil {
		t.Fatal(err)
	}
	if a.Status != Deleted || a.PreviousStatus != Unpublished {
		t.Fatalf("delete: %+v", a)
	}
	if err := a.Restore(); err != nil {
		t.Fatal(err)
	}
	if a.Status != Unpublished || a.PreviousStatus != "" {
		t.Fatalf("restore: %+v", a)
	}
}

func TestInvalidTransitionsAndVisibility(t *testing.T) {
	a := Article{Status: Draft, Title: "", Body: ""}
	if err := a.Publish(time.Now()); err == nil {
		t.Fatal("empty article published")
	}
	if err := a.Unpublish(); err == nil {
		t.Fatal("draft unpublished")
	}
	if a.VisibleToReader() {
		t.Fatal("draft visible")
	}
	a.Title, a.Body = "title", "body"
	if err := a.Publish(time.Now()); err != nil {
		t.Fatal(err)
	}
	if !a.VisibleToReader() {
		t.Fatal("published article hidden")
	}
	if err := a.Delete(time.Now()); err != nil {
		t.Fatal(err)
	}
	if a.VisibleToReader() {
		t.Fatal("deleted article visible")
	}
}

func TestArticleAndCommentPermissions(t *testing.T) {
	a := Article{AuthorID: 2}
	if !CanManageArticle(User{ID: 2, Role: Author}, a) {
		t.Fatal("author denied")
	}
	if CanManageArticle(User{ID: 3, Role: Author}, a) {
		t.Fatal("other author allowed")
	}
	if CanManageArticle(User{ID: 2, Role: Reader}, a) {
		t.Fatal("reader allowed")
	}
	if !CanManageArticle(User{ID: 3, Role: Admin}, a) {
		t.Fatal("admin denied")
	}
	if !CanDeleteComment(User{ID: 4, Role: Reader}, 4, a.AuthorID) {
		t.Fatal("commenter denied")
	}
	if !CanDeleteComment(User{ID: 2, Role: Author}, 4, a.AuthorID) {
		t.Fatal("article author denied")
	}
	if CanDeleteComment(User{ID: 3, Role: Author}, 4, a.AuthorID) {
		t.Fatal("other author allowed")
	}
}
