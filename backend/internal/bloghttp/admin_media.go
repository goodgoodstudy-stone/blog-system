package bloghttp

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"blog-system/backend/internal/mysqlrepo"
)

func (s *Server) users(w http.ResponseWriter, r *http.Request) {
	if requireAdmin(w, r) == nil {
		return
	}
	v, err := s.Store.ListUsers(r.Context())
	if err != nil {
		failErr(w, r, err)
		return
	}
	respond(w, r, 200, v)
}
func (s *Server) role(w http.ResponseWriter, r *http.Request) {
	if requireAdmin(w, r) == nil {
		return
	}
	id, err := paramID(r)
	if err != nil {
		fail(w, r, 404, "not_found", "用户不存在")
		return
	}
	var v struct {
		Role string `json:"role"`
	}
	if !decodeRequest(w, r, &v) {
		return
	}
	if v.Role != "reader" && v.Role != "author" {
		fail(w, r, 400, "validation", "角色只能是普通用户或博主")
		return
	}
	if err = s.Store.SetRole(r.Context(), id, v.Role); err != nil {
		failErr(w, r, err)
		return
	}
	u, err := s.Store.UserByID(r.Context(), id)
	if err != nil {
		failErr(w, r, err)
		return
	}
	respond(w, r, 200, u)
}
func (s *Server) createTag(w http.ResponseWriter, r *http.Request) {
	if requireAdmin(w, r) == nil {
		return
	}
	var v struct {
		Name string `json:"name"`
	}
	if !decodeRequest(w, r, &v) {
		return
	}
	if strings.TrimSpace(v.Name) == "" || len([]rune(v.Name)) > 80 {
		fail(w, r, 400, "validation", "请输入 1–80 字的标签名")
		return
	}
	t, err := s.Store.CreateTag(r.Context(), v.Name)
	if err != nil {
		failErr(w, r, err)
		return
	}
	respond(w, r, 201, t)
}
func (s *Server) updateTag(w http.ResponseWriter, r *http.Request) {
	if requireAdmin(w, r) == nil {
		return
	}
	id, err := paramID(r)
	if err != nil {
		fail(w, r, 404, "not_found", "标签不存在")
		return
	}
	var v struct {
		Name string `json:"name"`
	}
	if !decodeRequest(w, r, &v) {
		return
	}
	if strings.TrimSpace(v.Name) == "" || len([]rune(v.Name)) > 80 {
		fail(w, r, 400, "validation", "请输入 1–80 字的标签名")
		return
	}
	if err = s.Store.UpdateTag(r.Context(), id, v.Name); err != nil {
		failErr(w, r, err)
		return
	}
	respond(w, r, 200, map[string]bool{"ok": true})
}
func (s *Server) deleteTag(w http.ResponseWriter, r *http.Request) {
	if requireAdmin(w, r) == nil {
		return
	}
	id, err := paramID(r)
	if err != nil {
		fail(w, r, 404, "not_found", "标签不存在")
		return
	}
	if err = s.Store.DeleteTag(r.Context(), id); err != nil {
		failErr(w, r, err)
		return
	}
	respond(w, r, 200, map[string]bool{"ok": true})
}

func (s *Server) uploadImage(w http.ResponseWriter, r *http.Request) {
	u := requireAuthor(w, r)
	if u == nil {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20+1024)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		fail(w, r, 413, "too_large", "图片不能超过 10 MB")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		fail(w, r, 400, "validation", "请选择图片")
		return
	}
	defer file.Close()
	buf := make([]byte, 512)
	n, err := io.ReadFull(file, buf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		failErr(w, r, err)
		return
	}
	mime := http.DetectContentType(buf[:n])
	ext := map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp"}[mime]
	if ext == "" {
		fail(w, r, 400, "invalid_image", "只支持 JPG、PNG、WebP 图片")
		return
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		failErr(w, r, err)
		return
	}
	if err = os.MkdirAll(s.UploadDir, 0755); err != nil {
		failErr(w, r, err)
		return
	}
	random := make([]byte, 16)
	if _, err = rand.Read(random); err != nil {
		failErr(w, r, err)
		return
	}
	name := hex.EncodeToString(random) + ext
	path := filepath.Join(s.UploadDir, name)
	out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		failErr(w, r, err)
		return
	}
	size, err := io.Copy(out, io.LimitReader(file, 10<<20+1))
	closeErr := out.Close()
	if err != nil || closeErr != nil {
		removeUpload(w, r, path)
		logRequestFailure(w, r, "upload file write failed", errors.Join(err, closeErr), "upload_write")
		fail(w, r, 500, "upload_failed", "图片保存失败")
		return
	}
	if size > 10<<20 {
		removeUpload(w, r, path)
		fail(w, r, 413, "too_large", "图片不能超过 10 MB")
		return
	}
	img, err := s.Store.CreateImage(r.Context(), u.ID, name, mime, size)
	if err != nil {
		removeUpload(w, r, path)
		failErr(w, r, err)
		return
	}
	respond(w, r, 201, map[string]any{"id": img.ID, "url": "/api/v1/images/" + strconv.FormatInt(img.ID, 10)})
}
func (s *Server) image(w http.ResponseWriter, r *http.Request) {
	id, err := paramID(r)
	if err != nil {
		fail(w, r, 404, "not_found", "图片不存在")
		return
	}
	img, err := s.Store.GetImage(r.Context(), id)
	if err != nil {
		failErr(w, r, err)
		return
	}
	u := current(r)
	allowed := false
	if !img.ArticleID.Valid {
		allowed = u != nil && u.ID == img.UploaderID
	} else {
		a, e := s.Store.GetArticle(r.Context(), img.ArticleID.Int64, 0)
		if e == nil {
			allowed = a.Status == "published" || (u != nil && (u.Role == "admin" || u.ID == a.AuthorID))
		} else if !errors.Is(e, mysqlrepo.ErrNotFound) {
			failErr(w, r, e)
			return
		}
	}
	if !allowed {
		fail(w, r, 404, "not_found", "图片不存在或暂不可访问")
		return
	}
	w.Header().Set("Content-Type", img.Mime)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeFile(w, r, filepath.Join(s.UploadDir, img.Filename))
}

func removeUpload(w http.ResponseWriter, r *http.Request, path string) {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		logRequestFailure(w, r, "upload cleanup failed", err, "upload_cleanup")
	}
}
