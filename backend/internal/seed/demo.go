package seed

import (
	"context"
	"database/sql"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"time"

	"blog-system/backend/internal/auth"
)

func Demo(ctx context.Context, db *sql.DB, uploadDir, password string) error {
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	admin, err := user(ctx, db, "admin@blog.local", "演示管理员", "admin", hash)
	if err != nil {
		return err
	}
	author1, err := user(ctx, db, "author1@blog.local", "林间来信", "author", hash)
	if err != nil {
		return err
	}
	author2, err := user(ctx, db, "author2@blog.local", "午后手记", "author", hash)
	if err != nil {
		return err
	}
	_ = admin
	tag1, err := tag(ctx, db, "工程实践")
	if err != nil {
		return err
	}
	tag2, err := tag(ctx, db, "生活记录")
	if err != nil {
		return err
	}
	articles := []struct {
		author               int64
		title, summary, body string
		tagID                int64
	}{
		{author1, "把一个小博客认真做好", "从需求到发布，小系统也能体现完整的工程过程。", "# 从一篇文章开始\n\n小系统的价值，在于能看清每个决定如何影响读者。\n\n> 先写清需求，再动手实现。\n\n- 让文章易于查找\n- 让作者放心发布\n- 让误删有机会恢复", tag1},
		{author2, "秋日的阅读角落", "在温暖的页面里，给文字留一点呼吸的空间。", "# 午后的光\n\n一杯茶、一页书，也是一篇文章的起点。\n\n页面不需要喧闹，标题、正文和图片清楚就够了。", tag2},
		{author1, "如何给文章设计标签", "标签帮助读者沿着兴趣继续发现内容。", "# 标签的用法\n\n一篇文章可以有多个标签，但每个标签都应该有清晰含义。\n\n代码示例：\n\n```go\nfmt.Println(\"hello, blog\")\n```", tag1},
	}
	var first int64
	for i, v := range articles {
		id, e := article(ctx, db, v.author, v.title, v.summary, v.body, v.tagID)
		if e != nil {
			return e
		}
		if i == 0 {
			first = id
		}
	}
	return illustration(ctx, db, uploadDir, author1, first)
}
func user(ctx context.Context, db *sql.DB, email, nickname, role, hash string) (int64, error) {
	r, err := db.ExecContext(ctx, `INSERT INTO users(email,nickname,password_hash,role) VALUES(?,?,?,?) ON DUPLICATE KEY UPDATE id=LAST_INSERT_ID(id)`, email, nickname, hash, role)
	if err != nil {
		return 0, err
	}
	return r.LastInsertId()
}
func tag(ctx context.Context, db *sql.DB, name string) (int64, error) {
	r, err := db.ExecContext(ctx, `INSERT INTO tags(name) VALUES(?) ON DUPLICATE KEY UPDATE id=LAST_INSERT_ID(id)`, name)
	if err != nil {
		return 0, err
	}
	return r.LastInsertId()
}
func article(ctx context.Context, db *sql.DB, author int64, title, summary, body string, tagID int64) (int64, error) {
	var id int64
	err := db.QueryRowContext(ctx, `SELECT id FROM articles WHERE author_id=? AND title=? LIMIT 1`, author, title).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return 0, err
	}
	r, err := db.ExecContext(ctx, `INSERT INTO articles(author_id,title,summary,body_md,body_plain,status,published_at) VALUES(?,?,?,?,?,'published',?)`, author, title, summary, body, body, time.Now().UTC())
	if err != nil {
		return 0, err
	}
	id, _ = r.LastInsertId()
	_, err = db.ExecContext(ctx, `INSERT IGNORE INTO article_tags(article_id,tag_id) VALUES(?,?)`, id, tagID)
	return id, err
}
func illustration(ctx context.Context, db *sql.DB, uploadDir string, uploader, articleID int64) error {
	var count int
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM images WHERE article_id=?`, articleID).Scan(&count)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	if err = os.MkdirAll(uploadDir, 0755); err != nil {
		return err
	}
	name := "demo-warm-landscape.png"
	path := filepath.Join(uploadDir, name)
	img := image.NewRGBA(image.Rect(0, 0, 960, 520))
	for y := 0; y < 520; y++ {
		c := color.RGBA{R: uint8(248 - y/25), G: uint8(232 - y/18), B: uint8(211 - y/12), A: 255}
		draw.Draw(img, image.Rect(0, y, 960, y+1), &image.Uniform{C: c}, image.Point{}, draw.Src)
	}
	draw.Draw(img, image.Rect(0, 350, 960, 520), &image.Uniform{C: color.RGBA{181, 120, 91, 255}}, image.Point{}, draw.Src)
	draw.Draw(img, image.Rect(120, 130, 370, 380), &image.Uniform{C: color.RGBA{237, 190, 130, 255}}, image.Point{}, draw.Src)
	draw.Draw(img, image.Rect(440, 220, 850, 420), &image.Uniform{C: color.RGBA{111, 125, 96, 255}}, image.Point{}, draw.Src)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err = png.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	r, err := db.ExecContext(ctx, `INSERT INTO images(uploader_id,article_id,filename,mime,size_bytes) VALUES(?, ?, ?, 'image/png', ?)`, uploader, articleID, name, info.Size())
	if err != nil {
		return err
	}
	id, _ := r.LastInsertId()
	_, err = db.ExecContext(ctx, `UPDATE articles SET body_md=CONCAT(body_md,?),body_plain=CONCAT(body_plain,?) WHERE id=?`, fmt.Sprintf("\n\n![温暖的抽象风景](/api/v1/images/%d)", id), " 温暖的抽象风景", articleID)
	return err
}
