package bloghttp

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"blog-system/backend/internal/domain"
	"blog-system/backend/internal/mysqlrepo"
)

func viewerID(r *http.Request) int64 {
	if u := current(r); u != nil {
		return u.ID
	}
	return 0
}
func (s *Server) publicArticles(w http.ResponseWriter, r *http.Request) {
	p, n := pagination(r)
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if utf8.RuneCountInString(q) > 100 {
		fail(w, r, 400, "validation", "关键词过长")
		return
	}
	v, err := s.Store.ListArticles(r.Context(), mysqlrepo.ArticleFilter{Public: true, UserID: viewerID(r), Query: q, Tag: r.URL.Query().Get("tag"), Page: p, PageSize: n})
	if err != nil {
		failErr(w, r, err)
		return
	}
	respond(w, r, 200, v)
}
func (s *Server) publicArticle(w http.ResponseWriter, r *http.Request) {
	id, err := paramID(r)
	if err != nil {
		fail(w, r, 404, "not_found", "文章不存在或暂不可访问")
		return
	}
	a, err := s.Store.GetArticle(r.Context(), id, viewerID(r))
	if err != nil || a.Status != "published" {
		fail(w, r, 404, "not_found", "文章不存在或暂不可访问")
		return
	}
	respond(w, r, 200, a)
}
func (s *Server) publicComments(w http.ResponseWriter, r *http.Request) {
	id, err := paramID(r)
	if err != nil {
		fail(w, r, 404, "not_found", "文章不存在或暂不可访问")
		return
	}
	a, err := s.Store.GetArticle(r.Context(), id, 0)
	if err != nil || a.Status != "published" {
		fail(w, r, 404, "not_found", "文章不存在或暂不可访问")
		return
	}
	p, n := pagination(r)
	v, err := s.Store.ListComments(r.Context(), id, p, n)
	if err != nil {
		failErr(w, r, err)
		return
	}
	respond(w, r, 200, v)
}
func (s *Server) tags(w http.ResponseWriter, r *http.Request) {
	v, err := s.Store.ListTags(r.Context())
	if err != nil {
		failErr(w, r, err)
		return
	}
	respond(w, r, 200, v)
}
func (s *Server) favorites(w http.ResponseWriter, r *http.Request) {
	u := require(w, r)
	if u == nil {
		return
	}
	p, n := pagination(r)
	v, err := s.Store.ListFavorites(r.Context(), u.ID, p, n)
	if err != nil {
		failErr(w, r, err)
		return
	}
	respond(w, r, 200, v)
}
func (s *Server) addFavorite(w http.ResponseWriter, r *http.Request) {
	u := require(w, r)
	if u == nil {
		return
	}
	id, err := paramID(r)
	if err != nil {
		fail(w, r, 404, "not_found", "文章不存在")
		return
	}
	if err = s.App.Favorite(r.Context(), *u, id); err != nil {
		failErr(w, r, err)
		return
	}
	respond(w, r, 200, map[string]bool{"favorited": true})
}
func (s *Server) deleteFavorite(w http.ResponseWriter, r *http.Request) {
	u := require(w, r)
	if u == nil {
		return
	}
	id, err := paramID(r)
	if err != nil {
		fail(w, r, 404, "not_found", "文章不存在")
		return
	}
	if err = s.Store.DeleteFavorite(r.Context(), u.ID, id); err != nil {
		failErr(w, r, err)
		return
	}
	respond(w, r, 200, map[string]bool{"favorited": false})
}
func (s *Server) createComment(w http.ResponseWriter, r *http.Request) {
	u := require(w, r)
	if u == nil {
		return
	}
	id, err := paramID(r)
	if err != nil {
		fail(w, r, 404, "not_found", "文章不存在")
		return
	}
	var v struct {
		Content string `json:"content"`
	}
	if !decodeRequest(w, r, &v) {
		return
	}
	c, err := s.App.Comment(r.Context(), *u, id, v.Content)
	if err != nil {
		if errors.Is(err, domain.ErrIncomplete) {
			fail(w, r, 400, "validation", "评论须为 1–1000 个字符")
			return
		}
		failErr(w, r, err)
		return
	}
	respond(w, r, 201, c)
}
func (s *Server) deleteComment(w http.ResponseWriter, r *http.Request) {
	u := require(w, r)
	if u == nil {
		return
	}
	id, err := paramID(r)
	if err != nil {
		fail(w, r, 404, "not_found", "评论不存在")
		return
	}
	if err = s.App.DeleteComment(r.Context(), *u, id); err != nil {
		failErr(w, r, err)
		return
	}
	respond(w, r, 200, map[string]bool{"ok": true})
}

func (s *Server) manageable(w http.ResponseWriter, r *http.Request) (mysqlrepo.Article, bool) {
	u := requireAuthor(w, r)
	if u == nil {
		return mysqlrepo.Article{}, false
	}
	id, err := paramID(r)
	if err != nil {
		fail(w, r, 404, "not_found", "文章不存在")
		return mysqlrepo.Article{}, false
	}
	a, err := s.App.LoadManage(r.Context(), *u, id)
	if err != nil {
		failErr(w, r, err)
		return mysqlrepo.Article{}, false
	}
	return a, true
}
func (s *Server) manageArticles(w http.ResponseWriter, r *http.Request) {
	u := requireAuthor(w, r)
	if u == nil {
		return
	}
	status := r.URL.Query().Get("status")
	if status != "" && status != "draft" && status != "published" && status != "unpublished" {
		fail(w, r, 400, "validation", "文章状态不正确")
		return
	}
	p, n := pagination(r)
	v, err := s.Store.ListArticles(r.Context(), mysqlrepo.ArticleFilter{UserID: u.ID, Admin: u.Role == "admin", Status: status, Page: p, PageSize: n})
	if err != nil {
		failErr(w, r, err)
		return
	}
	respond(w, r, 200, v)
}
func (s *Server) trash(w http.ResponseWriter, r *http.Request) {
	u := requireAuthor(w, r)
	if u == nil {
		return
	}
	p, n := pagination(r)
	v, err := s.Store.ListArticles(r.Context(), mysqlrepo.ArticleFilter{UserID: u.ID, Admin: u.Role == "admin", Trash: true, Page: p, PageSize: n})
	if err != nil {
		failErr(w, r, err)
		return
	}
	respond(w, r, 200, v)
}
func (s *Server) manageArticle(w http.ResponseWriter, r *http.Request) {
	a, ok := s.manageable(w, r)
	if ok {
		respond(w, r, 200, a)
	}
}
func (s *Server) saveArticle(w http.ResponseWriter, r *http.Request) {
	u := requireAuthor(w, r)
	if u == nil {
		return
	}
	var v struct {
		Title   string  `json:"title"`
		Summary string  `json:"summary"`
		Body    string  `json:"body"`
		TagIDs  []int64 `json:"tagIds"`
		Version int64   `json:"version"`
	}
	if !decodeRequest(w, r, &v) {
		return
	}
	v.Title = strings.TrimSpace(v.Title)
	v.Summary = strings.TrimSpace(v.Summary)
	if v.Title == "" || strings.TrimSpace(v.Body) == "" || utf8.RuneCountInString(v.Title) > 255 {
		fail(w, r, 400, "validation", "标题和正文必填，标题不超过 255 字")
		return
	}
	if len(v.TagIDs) > 20 {
		fail(w, r, 400, "validation", "标签数量过多")
		return
	}
	a := mysqlrepo.Article{Title: v.Title, Summary: v.Summary, Body: v.Body, Version: v.Version}
	if r.Method == "PATCH" {
		id, err := paramID(r)
		if err != nil {
			fail(w, r, 404, "not_found", "文章不存在")
			return
		}
		a.ID = id
	}
	out, err := s.App.Save(r.Context(), *u, a, v.TagIDs)
	if err != nil {
		failErr(w, r, err)
		return
	}
	status := 200
	if r.Method == "POST" {
		status = 201
	}
	respond(w, r, status, out)
}
func (s *Server) transition(w http.ResponseWriter, r *http.Request, action string) {
	u := requireAuthor(w, r)
	if u == nil {
		return
	}
	id, err := paramID(r)
	if err != nil {
		fail(w, r, 404, "not_found", "文章不存在")
		return
	}
	v, err := s.App.Transition(r.Context(), *u, id, action)
	if err != nil {
		if errors.Is(err, domain.ErrIncomplete) {
			fail(w, r, 400, "validation", "标题和正文必填")
			return
		}
		if errors.Is(err, domain.ErrTransition) {
			fail(w, r, 409, "conflict", "当前状态不能执行此操作")
			return
		}
		failErr(w, r, err)
		return
	}
	respond(w, r, 200, v)
}
func (s *Server) publish(w http.ResponseWriter, r *http.Request)       { s.transition(w, r, "publish") }
func (s *Server) unpublish(w http.ResponseWriter, r *http.Request)     { s.transition(w, r, "unpublish") }
func (s *Server) deleteArticle(w http.ResponseWriter, r *http.Request) { s.transition(w, r, "delete") }
func (s *Server) restore(w http.ResponseWriter, r *http.Request)       { s.transition(w, r, "restore") }
func (s *Server) manageComments(w http.ResponseWriter, r *http.Request) {
	u := requireAuthor(w, r)
	if u == nil {
		return
	}
	p, n := pagination(r)
	v, err := s.Store.ListManageComments(r.Context(), u.ID, u.Role == "admin", p, n)
	if err != nil {
		failErr(w, r, err)
		return
	}
	respond(w, r, 200, v)
}

func parseID(s string) int64 { v, _ := strconv.ParseInt(s, 10, 64); return v }
