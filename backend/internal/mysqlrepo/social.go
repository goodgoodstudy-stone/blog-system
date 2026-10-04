package mysqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

func (s Store) ListTags(ctx context.Context) ([]Tag, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id,name FROM tags ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Tag{}
	for rows.Next() {
		var t Tag
		if err = rows.Scan(&t.ID, &t.Name); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
func (s Store) CreateTag(ctx context.Context, name string) (Tag, error) {
	name = strings.TrimSpace(name)
	r, err := s.DB.ExecContext(ctx, `INSERT INTO tags(name) VALUES(?)`, name)
	if err != nil {
		return Tag{}, err
	}
	id, _ := r.LastInsertId()
	return Tag{ID: id, Name: name}, nil
}
func (s Store) UpdateTag(ctx context.Context, id int64, name string) error {
	r, err := s.DB.ExecContext(ctx, `UPDATE tags SET name=? WHERE id=?`, strings.TrimSpace(name), id)
	if err != nil {
		return err
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
func (s Store) DeleteTag(ctx context.Context, id int64) error {
	r, err := s.DB.ExecContext(ctx, `DELETE FROM tags WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
func (s Store) CountTagArticles(ctx context.Context, id int64) (int, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM article_tags WHERE tag_id=?`, id).Scan(&n)
	return n, err
}

func (s Store) AddFavorite(ctx context.Context, userID, articleID int64) error {
	result, err := s.DB.ExecContext(ctx, `INSERT IGNORE INTO favorites(user_id,article_id) SELECT ?,id FROM articles WHERE id=? AND status='published'`, userID, articleID)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n > 0 {
		return nil
	}
	var status string
	if err = s.DB.QueryRowContext(ctx, `SELECT status FROM articles WHERE id=?`, articleID).Scan(&status); err != nil {
		return ErrNotFound
	}
	if status != "published" {
		return ErrNotFound
	}
	return nil
}
func (s Store) DeleteFavorite(ctx context.Context, userID, articleID int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM favorites WHERE user_id=? AND article_id=?`, userID, articleID)
	return err
}
func (s Store) ListFavorites(ctx context.Context, userID int64, page, size int) (Page[Article], error) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if size > 50 {
		size = 50
	}
	var total int64
	err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM favorites f JOIN articles a ON a.id=f.article_id WHERE f.user_id=? AND a.status='published'`, userID).Scan(&total)
	if err != nil {
		return Page[Article]{}, err
	}
	rows, err := s.DB.QueryContext(ctx, articleSelect+` JOIN favorites f ON f.article_id=a.id WHERE f.user_id=? AND a.status='published' ORDER BY f.created_at DESC LIMIT ? OFFSET ?`, userID, size, (page-1)*size)
	if err != nil {
		return Page[Article]{}, err
	}
	defer rows.Close()
	out := Page[Article]{Items: []Article{}, Total: total, Page: page, PageSize: size}
	for rows.Next() {
		a, e := scanArticle(rows)
		if e != nil {
			return out, e
		}
		a.Body = ""
		a.Favorited = true
		out.Items = append(out.Items, a)
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	for i := range out.Items {
		if err = s.fillArticle(ctx, &out.Items[i], userID); err != nil {
			return out, err
		}
	}
	return out, nil
}

func (s Store) CreateComment(ctx context.Context, articleID, userID int64, content string) (Comment, error) {
	r, err := s.DB.ExecContext(ctx, `INSERT INTO comments(article_id,user_id,content) SELECT id,?,? FROM articles WHERE id=? AND status='published'`, userID, content, articleID)
	if err != nil {
		return Comment{}, err
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		return Comment{}, ErrNotFound
	}
	id, _ := r.LastInsertId()
	return s.GetComment(ctx, id)
}
func (s Store) GetComment(ctx context.Context, id int64) (Comment, error) {
	var c Comment
	err := s.DB.QueryRowContext(ctx, `SELECT c.id,c.article_id,c.user_id,u.nickname,c.content,c.created_at FROM comments c JOIN users u ON u.id=c.user_id WHERE c.id=?`, id).Scan(&c.ID, &c.ArticleID, &c.UserID, &c.Nickname, &c.Content, &c.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return c, ErrNotFound
	}
	return c, err
}
func (s Store) DeleteComment(ctx context.Context, id int64) error {
	r, err := s.DB.ExecContext(ctx, `DELETE FROM comments WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
func (s Store) ListComments(ctx context.Context, articleID int64, page, size int) (Page[Comment], error) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if size > 50 {
		size = 50
	}
	var total int64
	err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM comments WHERE article_id=?`, articleID).Scan(&total)
	if err != nil {
		return Page[Comment]{}, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT c.id,c.article_id,c.user_id,u.nickname,c.content,c.created_at FROM comments c JOIN users u ON u.id=c.user_id WHERE c.article_id=? ORDER BY c.id DESC LIMIT ? OFFSET ?`, articleID, size, (page-1)*size)
	if err != nil {
		return Page[Comment]{}, err
	}
	defer rows.Close()
	out := Page[Comment]{Items: []Comment{}, Total: total, Page: page, PageSize: size}
	for rows.Next() {
		var c Comment
		if err = rows.Scan(&c.ID, &c.ArticleID, &c.UserID, &c.Nickname, &c.Content, &c.CreatedAt); err != nil {
			return out, err
		}
		out.Items = append(out.Items, c)
	}
	return out, rows.Err()
}
func (s Store) ListManageComments(ctx context.Context, userID int64, admin bool, page, size int) (Page[Comment], error) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if size > 50 {
		size = 50
	}
	where := ""
	args := []any{}
	if !admin {
		where = ` WHERE a.author_id=?`
		args = append(args, userID)
	}
	var total int64
	err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM comments c JOIN articles a ON a.id=c.article_id`+where, args...).Scan(&total)
	if err != nil {
		return Page[Comment]{}, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT c.id,c.article_id,c.user_id,u.nickname,c.content,c.created_at FROM comments c JOIN users u ON u.id=c.user_id JOIN articles a ON a.id=c.article_id`+where+` ORDER BY c.id DESC LIMIT ? OFFSET ?`, append(args, size, (page-1)*size)...)
	if err != nil {
		return Page[Comment]{}, err
	}
	defer rows.Close()
	out := Page[Comment]{Items: []Comment{}, Total: total, Page: page, PageSize: size}
	for rows.Next() {
		var c Comment
		if err = rows.Scan(&c.ID, &c.ArticleID, &c.UserID, &c.Nickname, &c.Content, &c.CreatedAt); err != nil {
			return out, err
		}
		out.Items = append(out.Items, c)
	}
	return out, rows.Err()
}

func (s Store) CreateImage(ctx context.Context, uploaderID int64, name, mime string, size int64) (Image, error) {
	r, err := s.DB.ExecContext(ctx, `INSERT INTO images(uploader_id,filename,mime,size_bytes) VALUES(?,?,?,?)`, uploaderID, name, mime, size)
	if err != nil {
		return Image{}, err
	}
	id, _ := r.LastInsertId()
	return Image{ID: id, UploaderID: uploaderID, Filename: name, Mime: mime, Size: size}, nil
}
func (s Store) GetImage(ctx context.Context, id int64) (Image, error) {
	var v Image
	err := s.DB.QueryRowContext(ctx, `SELECT id,uploader_id,article_id,filename,mime,size_bytes FROM images WHERE id=?`, id).Scan(&v.ID, &v.UploaderID, &v.ArticleID, &v.Filename, &v.Mime, &v.Size)
	if errors.Is(err, sql.ErrNoRows) {
		return v, ErrNotFound
	}
	return v, err
}
