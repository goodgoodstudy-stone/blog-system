import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useParams, useSearchParams } from "react-router-dom";
import ReactMarkdown, { defaultUrlTransform } from "react-markdown";
import {
  ArrowLeft,
  ArrowRight,
  Heart,
  MessageCircle,
  Search,
  Tag as TagIcon,
} from "lucide-react";
import {
  blogRequest,
  articleUrl,
  dateText,
  json,
  type Article,
  type Comment,
  type Page,
  type Tag,
} from "../blog-client";
import { useCurrentUser } from "../auth";

export function MarkdownView({ body }: { body: string }) {
  return (
    <div className="prose">
      <ReactMarkdown
        urlTransform={(url, key) =>
          key === "src"
            ? url.startsWith("/api/v1/images/")
              ? url
              : ""
            : defaultUrlTransform(url)
        }
      >
        {body}
      </ReactMarkdown>
    </div>
  );
}
export function ArticleCard({ article }: { article: Article }) {
  return (
    <article className="article-card">
      <div className="card-meta">
        <span>{article.authorName}</span>
        <span className="dot">·</span>
        <time>{dateText(article.publishedAt)}</time>
      </div>
      <h2>
        <Link to={articleUrl(article.id)}>{article.title}</Link>
      </h2>
      <p>{article.summary || "点开文章，继续阅读。"}</p>
      <div className="card-bottom">
        <div className="tag-row">
          {article.tags.map((t) => (
            <Link
              className="tag"
              key={t.id}
              to={`/?tag=${encodeURIComponent(t.name)}`}
            >
              #{t.name}
            </Link>
          ))}
        </div>
        <Link className="read-link" to={articleUrl(article.id)}>
          阅读全文 <ArrowRight size={15} />
        </Link>
      </div>
    </article>
  );
}
function Pager({
  page,
  total,
  pageSize,
  onChange,
}: {
  page: number;
  total: number;
  pageSize: number;
  onChange: (page: number) => void;
}) {
  if (total <= pageSize) return null;
  return (
    <div className="pager">
      <button disabled={page <= 1} onClick={() => onChange(page - 1)}>
        上一页
      </button>
      <span>
        第 {page} 页 / 共 {Math.ceil(total / pageSize)} 页
      </span>
      <button
        disabled={page * pageSize >= total}
        onClick={() => onChange(page + 1)}
      >
        下一页
      </button>
    </div>
  );
}

export function Home() {
  const [params, setParams] = useSearchParams();
  const q = params.get("q") || "";
  const tag = params.get("tag") || "";
  const page = Number(params.get("page") || "1") || 1;
  const [text, setText] = useState(q);
  const { data, isPending, error } = useQuery({
    queryKey: ["articles", q, tag, page],
    queryFn: () =>
      blogRequest<Page<Article>>(
        `/articles?${new URLSearchParams({ q, tag, page: String(page) })}`,
      ),
  });
  const { data: tags = [] } = useQuery({
    queryKey: ["tags"],
    queryFn: () => blogRequest<Tag[]>("/tags"),
  });
  function change(next: Record<string, string>) {
    const p = new URLSearchParams(params);
    Object.entries(next).forEach(([k, v]) => (v ? p.set(k, v) : p.delete(k)));
    if (!("page" in next)) p.delete("page");
    setParams(p);
  }
  return (
    <>
      <section className="hero">
        <div className="hero-copy">
          <span className="eyebrow">写给生活，也写给认真思考的人</span>
          <h1>在这里，慢慢读一篇好文章。</h1>
          <p>
            记录工程实践，也收集日常灵感。跟着兴趣出发，找到下一段值得停留的文字。
          </p>
          <form
            className="search-form"
            onSubmit={(e) => {
              e.preventDefault();
              change({ q: text.trim(), page: "" });
            }}
          >
            <Search size={19} />
            <input
              aria-label="搜索文章"
              placeholder="搜索文章、话题或一个念头…"
              value={text}
              onChange={(e) => setText(e.target.value)}
            />
            <button type="submit">搜索</button>
          </form>
        </div>
        <div className="hero-art" aria-hidden="true">
          <div className="art-circle" />
          <div className="art-book" />
          <div className="art-leaf art-leaf-one" />
          <div className="art-leaf art-leaf-two" />
          <span>
            WORDS
            <br />
            AND WARMTH
          </span>
        </div>
      </section>
      <div className="section-head">
        <div>
          <span className="eyebrow">EXPLORE THE STORIES</span>
          <h2>{q ? `搜索「${q}」` : tag ? `标签「${tag}」` : "最新文章"}</h2>
        </div>
        <span className="result-count">{data?.total ?? 0} 篇文章</span>
      </div>
      <div className="content-grid">
        <div>
          {isPending ? (
            <div className="state-box">正在整理文章…</div>
          ) : error ? (
            <div className="state-box">文章暂时无法加载。</div>
          ) : data!.items.length ? (
            <>
              {data!.items.map((a) => (
                <ArticleCard key={a.id} article={a} />
              ))}
              <Pager
                page={page}
                total={data!.total}
                pageSize={data!.pageSize}
                onChange={(p) => change({ page: String(p) })}
              />
            </>
          ) : (
            <div className="state-box">
              <h3>还没有找到文章</h3>
              <p>换一个关键词，或者看看其他标签。</p>
              <button
                className="text-button"
                onClick={() => {
                  setText("");
                  setParams({});
                }}
              >
                查看全部文章
              </button>
            </div>
          )}
        </div>
        <aside className="side-panel">
          <h3>
            <TagIcon size={18} />
            探索标签
          </h3>
          <p>沿着感兴趣的主题继续读下去。</p>
          <div className="tag-cloud">
            <Link className={!tag ? "tag active" : "tag"} to="/">
              全部文章
            </Link>
            {tags.map((t) => (
              <Link
                className={tag === t.name ? "tag active" : "tag"}
                key={t.id}
                to={`/?tag=${encodeURIComponent(t.name)}`}
              >
                {t.name}
              </Link>
            ))}
          </div>
          <div className="aside-note">
            “文字会留下来，成为我们与世界相遇的另一种方式。”
          </div>
        </aside>
      </div>
    </>
  );
}

export function ArticleDetail() {
  const id = Number(useParams().id);
  const qc = useQueryClient();
  const { data: user } = useCurrentUser();
  const [comment, setComment] = useState("");
  const [page, setPage] = useState(1);
  const [message, setMessage] = useState("");
  const article = useQuery({
    queryKey: ["article", id],
    queryFn: () => blogRequest<Article>(`/articles/${id}`),
    enabled: Number.isFinite(id),
  });
  const comments = useQuery({
    queryKey: ["comments", id, page],
    queryFn: () =>
      blogRequest<Page<Comment>>(`/articles/${id}/comments?page=${page}`),
    enabled: article.isSuccess,
  });
  const favorite = useMutation({
    mutationFn: () =>
      blogRequest(`/favorites/${id}`, {
        method: article.data?.favorited ? "DELETE" : "PUT",
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["article", id] });
      void qc.invalidateQueries({ queryKey: ["favorites"] });
    },
    onError: (e) => setMessage(e.message),
  });
  const post = useMutation({
    mutationFn: () =>
      blogRequest<Comment>(`/articles/${id}/comments`, {
        method: "POST",
        body: json({ content: comment }),
      }),
    onSuccess: () => {
      setComment("");
      setPage(1);
      void qc.invalidateQueries({ queryKey: ["comments", id] });
    },
    onError: (e) => setMessage(e.message),
  });
  const remove = useMutation({
    mutationFn: (commentId: number) =>
      blogRequest(`/comments/${commentId}`, { method: "DELETE" }),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["comments", id] }),
    onError: (e) => setMessage(e.message),
  });
  if (article.isPending) return <div className="state-box">正在打开文章…</div>;
  if (article.error || !article.data)
    return (
      <div className="state-box">
        <h2>文章不存在或暂不可访问</h2>
        <Link to="/">返回首页</Link>
      </div>
    );
  const a = article.data;
  return (
    <div className="reading-layout">
      <Link className="back-link" to="/">
        <ArrowLeft size={16} /> 返回文章列表
      </Link>
      <article className="reading-article">
        <div className="article-heading">
          <div className="tag-row">
            {a.tags.map((t) => (
              <Link
                className="tag"
                to={`/?tag=${encodeURIComponent(t.name)}`}
                key={t.id}
              >
                #{t.name}
              </Link>
            ))}
          </div>
          <h1>{a.title}</h1>
          <p className="article-subtitle">{a.summary}</p>
          <div className="article-byline">
            <span className="avatar">{a.authorName.slice(0, 1)}</span>
            <span>
              <strong>{a.authorName}</strong>
              <small>{dateText(a.publishedAt)}</small>
            </span>
          </div>
        </div>
        <MarkdownView body={a.body} />
        <div className="article-actions">
          {user ? (
            <button
              className={a.favorited ? "button soft favorited" : "button soft"}
              onClick={() => favorite.mutate()}
              disabled={favorite.isPending}
            >
              <Heart size={17} fill={a.favorited ? "currentColor" : "none"} />
              {a.favorited ? "已收藏" : "收藏文章"}
            </button>
          ) : (
            <Link className="button soft" to="/login">
              <Heart size={17} />
              登录后收藏
            </Link>
          )}
        </div>
      </article>
      <section className="comments-section">
        <div className="section-head">
          <h2>
            <MessageCircle size={21} />
            读者评论
          </h2>
          <span>{comments.data?.total ?? 0} 条</span>
        </div>
        {user ? (
          <form
            className="comment-form"
            onSubmit={(e) => {
              e.preventDefault();
              post.mutate();
            }}
          >
            <label htmlFor="comment">留下你的想法</label>
            <textarea
              id="comment"
              maxLength={1000}
              value={comment}
              onChange={(e) => setComment(e.target.value)}
              placeholder="说说读完后的感受…"
            />
            <div>
              <small>{comment.length} / 1000</small>
              <button
                className="button"
                type="submit"
                disabled={post.isPending || !comment.trim()}
              >
                发表评论
              </button>
            </div>
          </form>
        ) : (
          <div className="comment-prompt">
            想参与讨论？<Link to="/login">登录后评论</Link>
          </div>
        )}
        {message ? (
          <p className="form-error" role="alert">
            {message}
          </p>
        ) : null}
        {comments.data?.items.length ? (
          <div className="comment-list">
            {comments.data.items.map((c) => (
              <div className="comment" key={c.id}>
                <span className="avatar small">{c.nickname.slice(0, 1)}</span>
                <div>
                  <div className="comment-head">
                    <strong>{c.nickname}</strong>
                    <time>{dateText(c.createdAt)}</time>
                  </div>
                  <p>{c.content}</p>
                  {user &&
                  (user.id === c.userId ||
                    user.role === "admin" ||
                    (user.id === a.authorId && user.role === "author")) ? (
                    <button
                      className="text-button danger"
                      onClick={() => {
                        if (window.confirm("确定删除这条评论？"))
                          remove.mutate(c.id);
                      }}
                    >
                      删除
                    </button>
                  ) : null}
                </div>
              </div>
            ))}
          </div>
        ) : (
          <p className="empty-hint">还没有评论，来留下第一句吧。</p>
        )}
        <Pager
          page={page}
          total={comments.data?.total ?? 0}
          pageSize={comments.data?.pageSize ?? 20}
          onChange={setPage}
        />
      </section>
    </div>
  );
}

export function Favorites() {
  const [page, setPage] = useState(1);
  const qc = useQueryClient();
  const { data, isPending } = useQuery({
    queryKey: ["favorites", page],
    queryFn: () => blogRequest<Page<Article>>(`/favorites?page=${page}`),
  });
  const remove = useMutation({
    mutationFn: (id: number) =>
      blogRequest(`/favorites/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["favorites"] });
      void qc.invalidateQueries({ queryKey: ["article"] });
    },
  });
  return (
    <div className="narrow-page">
      <div className="page-intro">
        <span className="eyebrow">YOUR READING SHELF</span>
        <h1>我的收藏</h1>
        <p>把喜欢的文章留在这里，想读时再慢慢打开。</p>
      </div>
      {isPending ? (
        <div className="state-box">正在整理收藏…</div>
      ) : data?.items.length ? (
        <>
          {data.items.map((a) => (
            <div key={a.id} className="favorite-item">
              <ArticleCard article={a} />
              <button
                className="text-button danger"
                onClick={() => remove.mutate(a.id)}
              >
                取消收藏
              </button>
            </div>
          ))}
          <Pager
            page={page}
            total={data.total}
            pageSize={data.pageSize}
            onChange={setPage}
          />
        </>
      ) : (
        <div className="state-box">
          <h2>收藏夹还是空的</h2>
          <p>遇到喜欢的文章，点一下收藏吧。</p>
          <Link className="button" to="/">
            去读文章
          </Link>
        </div>
      )}
    </div>
  );
}
