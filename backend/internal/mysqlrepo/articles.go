package mysqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"blog-system/backend/internal/domain"
)

const articleSelect = `SELECT a.id,a.author_id,u.nickname,a.title,a.summary,a.body_md,a.status,a.previous_status,a.published_at,a.deleted_at,a.created_at,a.updated_at,a.version FROM articles a JOIN users u ON u.id=a.author_id`

var imageRef = regexp.MustCompile(`/api/v1/images/([0-9]+)`)

func scanArticle(row interface{ Scan(...any) error }) (Article, error) {
	var a Article
	var prev sql.NullString
	var pub, del sql.NullTime
	err := row.Scan(&a.ID, &a.AuthorID, &a.AuthorName, &a.Title, &a.Summary, &a.Body, &a.Status, &prev, &pub, &del, &a.CreatedAt, &a.UpdatedAt, &a.Version)
	if err != nil {
		return a, err
	}
	if prev.Valid {
		a.PreviousStatus = prev.String
	}
	if pub.Valid {
		a.PublishedAt = &pub.Time
	}
	if del.Valid {
		a.DeletedAt = &del.Time
	}
	a.Tags = []Tag{}
	return a, nil
}
func (s Store) GetArticle(ctx context.Context, id, viewerID int64) (Article, error) {
	a, err := scanArticle(s.DB.QueryRowContext(ctx, articleSelect+` WHERE a.id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrNotFound
	}
	if err != nil {
		return a, err
	}
	if err = s.fillArticle(ctx, &a, viewerID); err != nil {
		return a, err
	}
	return a, nil
}
func (s Store) fillArticle(ctx context.Context, a *Article, viewerID int64) error {
	rows, err := s.DB.QueryContext(ctx, `SELECT t.id,t.name FROM article_tags at JOIN tags t ON t.id=at.tag_id WHERE at.article_id=? ORDER BY t.name`, a.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var t Tag
		if err = rows.Scan(&t.ID, &t.Name); err != nil {
			return err
		}
		a.Tags = append(a.Tags, t)
	}
	if err = rows.Err(); err != nil {
		return err
	}
	if viewerID > 0 {
		var n int
		err = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM favorites WHERE user_id=? AND article_id=?`, viewerID, a.ID).Scan(&n)
		if err != nil {
			return err
		}
		a.Favorited = n > 0
	}
	return nil
}

type ArticleFilter struct {
	UserID   int64
	Admin    bool
	Public   bool
	Trash    bool
	Status   string
	Query    string
	Tag      string
	Page     int
	PageSize int
}

func (s Store) ListArticles(ctx context.Context, f ArticleFilter) (Page[Article], error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 {
		f.PageSize = 20
	}
	if f.PageSize > 50 {
		f.PageSize = 50
	}
	where := []string{}
	args := []any{}
	if f.Public {
		where = append(where, `a.status='published'`)
	} else if f.Trash {
		where = append(where, `a.status='deleted'`)
	} else {
		where = append(where, `a.status<>'deleted'`)
	}
	if !f.Public && !f.Trash && f.Status != "" {
		where = append(where, `a.status=?`)
		args = append(args, f.Status)
	}
	if !f.Public && !f.Admin {
		where = append(where, `a.author_id=?`)
		args = append(args, f.UserID)
	}
	if f.Public && f.Tag != "" {
		where = append(where, `EXISTS (SELECT 1 FROM article_tags at JOIN tags t ON t.id=at.tag_id WHERE at.article_id=a.id AND t.name=?)`)
		args = append(args, f.Tag)
	}
	q := strings.TrimSpace(f.Query)
	if f.Public && q != "" {
		if len([]rune(q)) == 1 {
			where = append(where, `(a.title LIKE ? OR a.summary LIKE ? OR a.body_plain LIKE ?)`)
			v := "%" + escapeLike(q) + "%"
			args = append(args, v, v, v)
		} else {
			where = append(where, `MATCH(a.title,a.summary,a.body_plain) AGAINST (? IN NATURAL LANGUAGE MODE)`)
			args = append(args, q)
		}
	}
	cond := strings.Join(where, " AND ")
	var total int64
	err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM articles a WHERE `+cond, args...).Scan(&total)
	if err != nil {
		return Page[Article]{}, err
	}
	order := `a.updated_at DESC,a.id DESC`
	if f.Public {
		order = `a.published_at DESC,a.id DESC`
	}
	query := articleSelect + ` WHERE ` + cond + ` ORDER BY ` + order + ` LIMIT ? OFFSET ?`
	rows, err := s.DB.QueryContext(ctx, query, append(args, f.PageSize, (f.Page-1)*f.PageSize)...)
	if err != nil {
		return Page[Article]{}, err
	}
	defer rows.Close()
	out := Page[Article]{Items: []Article{}, Total: total, Page: f.Page, PageSize: f.PageSize}
	for rows.Next() {
		a, e := scanArticle(rows)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, a)
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	for i := range out.Items {
		if err = s.fillArticle(ctx, &out.Items[i], f.UserID); err != nil {
			return out, err
		}
		if !f.Public {
			continue
		}
		out.Items[i].Body = ""
	}
	return out, nil
}
func escapeLike(v string) string {
	v = strings.ReplaceAll(v, `\`, `\\`)
	v = strings.ReplaceAll(v, `%`, `\%`)
	return strings.ReplaceAll(v, `_`, `\_`)
}

func (s Store) SaveArticle(ctx context.Context, a Article, tagIDs []int64, editorID int64) (Article, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Article{}, err
	}
	defer tx.Rollback()
	plain := plainText(a.Body)
	if a.ID == 0 {
		r, e := tx.ExecContext(ctx, `INSERT INTO articles(author_id,title,summary,body_md,body_plain,status) VALUES(?,?,?,?,?,'draft')`, editorID, a.Title, a.Summary, a.Body, plain)
		if e != nil {
			return Article{}, e
		}
		a.ID, _ = r.LastInsertId()
	} else {
		r, e := tx.ExecContext(ctx, `UPDATE articles SET title=?,summary=?,body_md=?,body_plain=?,version=version+1,updated_at=UTC_TIMESTAMP(6) WHERE id=? AND version=? AND status<>'deleted'`, a.Title, a.Summary, a.Body, plain, a.ID, a.Version)
		if e != nil {
			return Article{}, e
		}
		n, _ := r.RowsAffected()
		if n == 0 {
			return Article{}, ErrConflict
		}
		if _, e = tx.ExecContext(ctx, `DELETE FROM article_tags WHERE article_id=?`, a.ID); e != nil {
			return Article{}, e
		}
	}
	seen := map[int64]bool{}
	for _, id := range tagIDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		if _, err = tx.ExecContext(ctx, `INSERT INTO article_tags(article_id,tag_id) VALUES(?,?)`, a.ID, id); err != nil {
			return Article{}, err
		}
	}
	if err = bindImages(ctx, tx, a.ID, editorID, a.Body); err != nil {
		return Article{}, err
	}
	if err = tx.Commit(); err != nil {
		return Article{}, err
	}
	return s.GetArticle(ctx, a.ID, editorID)
}
func plainText(md string) string {
	md = imageRef.ReplaceAllString(md, "")
	re := regexp.MustCompile(`[#*_~` + "`" + `>\[\]()]`)
	return re.ReplaceAllString(md, " ")
}
func bindImages(ctx context.Context, tx *sql.Tx, articleID, editorID int64, body string) error {
	refs := imageRef.FindAllStringSubmatch(body, -1)
	ids := map[int64]bool{}
	for _, m := range refs {
		id, _ := strconv.ParseInt(m[1], 10, 64)
		ids[id] = true
	}
	for id := range ids {
		var owner int64
		var attached sql.NullInt64
		err := tx.QueryRowContext(ctx, `SELECT uploader_id,article_id FROM images WHERE id=? FOR UPDATE`, id).Scan(&owner, &attached)
		if err != nil {
			return ErrForbidden
		}
		if attached.Valid && attached.Int64 != articleID {
			return ErrForbidden
		}
		if !attached.Valid && owner != editorID {
			return ErrForbidden
		}
		if _, err = tx.ExecContext(ctx, `UPDATE images SET article_id=? WHERE id=?`, articleID, id); err != nil {
			return err
		}
	}
	// Removed images become private staging resources, then the cleanup job may remove them.
	if len(ids) == 0 {
		_, err := tx.ExecContext(ctx, `UPDATE images SET article_id=NULL WHERE article_id=?`, articleID)
		return err
	}
	placeholders := make([]string, 0, len(ids))
	args := []any{articleID}
	for id := range ids {
		placeholders = append(placeholders, "?")
		args = append(args, id)
	}
	_, err := tx.ExecContext(ctx, fmt.Sprintf(`UPDATE images SET article_id=NULL WHERE article_id=? AND id NOT IN (%s)`, strings.Join(placeholders, ",")), args...)
	return err
}

func (s Store) UpdateArticleState(ctx context.Context, id, expectedVersion int64, d domain.Article) error {
	var pub, del any
	if !d.PublishedAt.IsZero() {
		pub = d.PublishedAt
	}
	if !d.DeletedAt.IsZero() {
		del = d.DeletedAt
	}
	var previous any
	if d.PreviousStatus != "" {
		previous = d.PreviousStatus
	}
	r, err := s.DB.ExecContext(ctx, `UPDATE articles SET status=?,previous_status=?,published_at=?,deleted_at=?,version=version+1,updated_at=UTC_TIMESTAMP(6) WHERE id=? AND version=?`, d.Status, previous, pub, del, id, expectedVersion)
	if err != nil {
		return err
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		return ErrConflict
	}
	return nil
}
