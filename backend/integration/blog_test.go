package integration

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"os"
	"strconv"
	"testing"
	"time"
)

type client struct {
	t    *testing.T
	base string
	http *http.Client
}

func newClient(t *testing.T, base string) *client {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	return &client{t: t, base: base, http: &http.Client{Jar: jar, Timeout: 10 * time.Second}}
}
func (c *client) request(method, path string, body any, want int) map[string]any {
	c.t.Helper()
	var data io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		data = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.base+"/api/v1"+path, data)
	if err != nil {
		c.t.Fatal(err)
	}
	if method != "GET" {
		req.Header.Set("Origin", c.base)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != want {
		c.t.Fatalf("%s %s: got %d want %d: %s", method, path, res.StatusCode, want, raw)
	}
	var result map[string]any
	_ = json.Unmarshal(raw, &result)
	return result
}
func number(v any) int64 { return int64(v.(float64)) }

func TestProductJourney(t *testing.T) {
	base := os.Getenv("BLOG_BASE_URL")
	if base == "" {
		t.Skip("set BLOG_BASE_URL to run against Compose")
	}
	reader := newClient(t, base)
	admin := newClient(t, base)
	other := newClient(t, base)
	email := "journey-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.test"
	reader.request("POST", "/auth/register", map[string]any{"email": "bad@@example.test", "nickname": "无效邮箱", "password": "Example123!", "confirm": "Example123!"}, 400)
	u := reader.request("POST", "/auth/register", map[string]any{"email": email, "nickname": "测试读者", "password": "Example123!", "confirm": "Example123!"}, 201)
	uid := number(u["id"])
	reader.request("POST", "/manage/articles", map[string]any{"title": "不应创建", "body": "内容"}, 403)
	admin.request("POST", "/auth/login", map[string]any{"email": "admin@blog.local", "password": "Demo12345!"}, 200)
	admin.request("PATCH", "/admin/users/"+strconv.FormatInt(uid, 10)+"/role", map[string]any{"role": "author"}, 200)
	a := reader.request("POST", "/manage/articles", map[string]any{"title": "测试文章", "summary": "集成验收", "body": "# 正文\n\n工程化让博客更可靠。", "tagIds": []int{1}}, 201)
	id := number(a["id"])
	path := "/articles/" + strconv.FormatInt(id, 10)
	reader.request("GET", path, nil, 404)
	filtered := reader.request("GET", "/manage/articles?status=published", nil, 200)
	if number(filtered["total"]) != 0 {
		t.Fatalf("draft leaked into published filter: %#v", filtered)
	}
	other.request("POST", "/auth/login", map[string]any{"email": "author2@blog.local", "password": "Demo12345!"}, 200)
	other.request("PATCH", "/manage/articles/"+strconv.FormatInt(id, 10), map[string]any{"title": "越权", "body": "内容", "version": 1}, 403)
	reader.request("POST", "/manage/articles/"+strconv.FormatInt(id, 10)+"/publish", nil, 200)
	reader.request("GET", path, nil, 200)
	reader.request("POST", "/auth/logout", nil, 200)
	reader.request("POST", "/auth/login", map[string]any{"email": email, "password": "Example123!"}, 200)
	reader.request("PUT", "/favorites/"+strconv.FormatInt(id, 10), nil, 200)
	reader.request("PUT", "/favorites/"+strconv.FormatInt(id, 10), nil, 200)
	favs := reader.request("GET", "/favorites", nil, 200)
	if number(favs["total"]) != 1 {
		t.Fatal("duplicate favorite created")
	}
	comment := reader.request("POST", path+"/comments", map[string]any{"content": "写得很好"}, 201)
	commentPath := "/comments/" + strconv.FormatInt(number(comment["id"]), 10)
	other.request("DELETE", commentPath, nil, 403)
	admin.request("DELETE", commentPath, nil, 200)
	comments := reader.request("GET", path+"/comments", nil, 200)
	if number(comments["total"]) != 0 {
		t.Fatal("deleted comment is still public")
	}
	otherComment := other.request("POST", path+"/comments", map[string]any{"content": "另一位博主的评论"}, 201)
	reader.request("DELETE", "/comments/"+strconv.FormatInt(number(otherComment["id"]), 10), nil, 200)
	reader.request("POST", "/manage/articles/"+strconv.FormatInt(id, 10)+"/unpublish", nil, 200)
	reader.request("GET", path, nil, 404)
	favs = reader.request("GET", "/favorites", nil, 200)
	if number(favs["total"]) != 0 {
		t.Fatal("unpublished article visible in favorites")
	}
	reader.request("POST", "/manage/articles/"+strconv.FormatInt(id, 10)+"/publish", nil, 200)
	favs = reader.request("GET", "/favorites", nil, 200)
	if number(favs["total"]) != 1 {
		t.Fatal("favorites not restored after republish")
	}
	reader.request("DELETE", "/manage/articles/"+strconv.FormatInt(id, 10), nil, 200)
	reader.request("GET", path, nil, 404)
	reader.request("POST", "/manage/articles/"+strconv.FormatInt(id, 10)+"/restore", nil, 200)
	reader.request("GET", path, nil, 200)
	admin.request("PATCH", "/admin/users/"+strconv.FormatInt(uid, 10)+"/role", map[string]any{"role": "reader"}, 200)
	reader.request("POST", "/manage/articles/"+strconv.FormatInt(id, 10)+"/unpublish", nil, 403)
}

func TestSearchAndPrivateImage(t *testing.T) {
	base := os.Getenv("BLOG_BASE_URL")
	if base == "" {
		t.Skip("set BLOG_BASE_URL to run against Compose")
	}
	guest := newClient(t, base)
	for _, term := range []string{"工程", "工"} {
		result := guest.request("GET", "/articles?q="+term, nil, 200)
		if number(result["total"]) < 1 {
			t.Fatalf("Chinese search returned no result for %q", term)
		}
	}
	author := newClient(t, base)
	author.request("POST", "/auth/login", map[string]any{"email": "author1@blog.local", "password": "Demo12345!"}, 200)
	pngBytes, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/lqkAAAAASUVORK5CYII=")
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, _ := writer.CreateFormFile("file", "tiny.png")
	_, _ = part.Write(pngBytes)
	_ = writer.Close()
	req, _ := http.NewRequest("POST", base+"/api/v1/manage/images", &body)
	req.Header.Set("Origin", base)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	res, err := author.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.StatusCode != 201 {
		t.Fatalf("upload: %d %s", res.StatusCode, raw)
	}
	var uploaded map[string]any
	_ = json.Unmarshal(raw, &uploaded)
	url := uploaded["url"].(string)
	imageGet := func(want int) {
		t.Helper()
		r, e := guest.http.Get(base + url)
		if e != nil {
			t.Fatal(e)
		}
		_ = r.Body.Close()
		if r.StatusCode != want {
			t.Fatalf("image status = %d, want %d", r.StatusCode, want)
		}
	}
	imageGet(404)
	a := author.request("POST", "/manage/articles", map[string]any{"title": "图片访问测试", "body": "![测试图片](" + url + ")"}, 201)
	id := number(a["id"])
	path := "/manage/articles/" + strconv.FormatInt(id, 10)
	imageGet(404)
	author.request("POST", path+"/publish", nil, 200)
	imageGet(200)
	author.request("POST", path+"/unpublish", nil, 200)
	imageGet(404)
	author.request("DELETE", path, nil, 200)
	author.request("POST", path+"/restore", nil, 200)
	imageGet(404)
}
