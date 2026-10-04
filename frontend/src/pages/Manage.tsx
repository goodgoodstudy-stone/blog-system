import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, NavLink, useNavigate, useParams } from "react-router-dom";
import {
  ArrowLeft,
  FilePlus2,
  ImagePlus,
  RotateCcw,
  Save,
  Trash2,
  Upload,
} from "lucide-react";
import {
  api,
  articleUrl,
  dateTimeText,
  json,
  statusText,
  type Article,
  type Comment,
  type Page,
  type Tag,
  type User,
} from "../api";
import { useCurrentUser } from "../auth";
import { MarkdownView } from "./Public";

function WorkspaceNav() {
  const { data: user } = useCurrentUser();
  return (
    <nav className="workspace-nav" aria-label="工作台">
      <NavLink to="/manage" end>
        文章
      </NavLink>
      <NavLink to="/manage/trash">回收站</NavLink>
      <NavLink to="/manage/comments">评论</NavLink>
      {user?.role === "admin" ? (
        <>
          <NavLink to="/admin/users">用户权限</NavLink>
          <NavLink to="/admin/tags">标签</NavLink>
        </>
      ) : null}
    </nav>
  );
}
function Workspace({
  title,
  subtitle,
  children,
  action,
}: {
  title: string;
  subtitle: string;
  children: React.ReactNode;
  action?: React.ReactNode;
}) {
  return (
    <div className="workspace">
      <div className="workspace-head">
        <div>
          <span className="eyebrow">WRITING STUDIO</span>
          <h1>{title}</h1>
          <p>{subtitle}</p>
        </div>
        {action}
      </div>
      <WorkspaceNav />
      {children}
    </div>
  );
}
function State({ children }: { children: React.ReactNode }) {
  return <div className="state-box">{children}</div>;
}

export function ManageArticles() {
  const [page, setPage] = useState(1);
  const [status, setStatus] = useState("");
  const { data, isPending, error } = useQuery({
    queryKey: ["manage-articles", page, status],
    queryFn: () =>
      api<Page<Article>>(
        `/manage/articles?${new URLSearchParams({ page: String(page), status })}`,
      ),
  });
  return (
    <Workspace
      title="文章管理"
      subtitle="整理草稿，发布想说的话。"
      action={
        <Link className="button" to="/manage/new">
          <FilePlus2 size={17} />
          写新文章
        </Link>
      }
    >
      <div className="filter-row">
        {[
          ["", "全部"],
          ["draft", "草稿"],
          ["published", "已发布"],
          ["unpublished", "已下线"],
        ].map(([value, label]) => (
          <button
            key={value}
            className={status === value ? "filter active" : "filter"}
            onClick={() => {
              setStatus(value);
              setPage(1);
            }}
          >
            {label}
          </button>
        ))}
      </div>
      {isPending ? (
        <State>正在加载文章…</State>
      ) : error ? (
        <State>文章暂时无法加载。</State>
      ) : data?.items.length ? (
        <div className="list-panel">
          {data.items.map((a) => (
            <div className="manage-row" key={a.id}>
              <div>
                <span className={`status ${a.status}`}>
                  {statusText[a.status]}
                </span>
                <h3>
                  <Link to={`/manage/articles/${a.id}/edit`}>{a.title}</Link>
                </h3>
                <p>{a.summary || "暂无摘要"}</p>
                <small>
                  作者：{a.authorName} · 更新于 {dateTimeText(a.updatedAt)}
                </small>
              </div>
              <Link
                className="button soft button-small"
                to={`/manage/articles/${a.id}/edit`}
              >
                编辑
              </Link>
            </div>
          ))}
        </div>
      ) : (
        <State>
          <h2>这里还没有文章</h2>
          <p>新建一篇草稿，开始写作吧。</p>
          <Link className="button" to="/manage/new">
            写新文章
          </Link>
        </State>
      )}
      {data && data.total > data.pageSize ? (
        <div className="pager">
          <button disabled={page <= 1} onClick={() => setPage(page - 1)}>
            上一页
          </button>
          <span>第 {page} 页</span>
          <button
            disabled={page * data.pageSize >= data.total}
            onClick={() => setPage(page + 1)}
          >
            下一页
          </button>
        </div>
      ) : null}
    </Workspace>
  );
}

export function ArticleEditor() {
  const param = useParams().id;
  const id = param ? Number(param) : 0;
  const article = useQuery({
    queryKey: ["manage-article", id],
    queryFn: () => api<Article>(`/manage/articles/${id}`),
    enabled: id > 0,
  });
  if (id && article.isPending) return <State>正在打开编辑器…</State>;
  if (id && article.error) return <State>文章不存在或没有编辑权限。</State>;
  return <EditorForm key={id} id={id} initial={article.data} />;
}

function EditorForm({ id, initial }: { id: number; initial?: Article }) {
  const navigate = useNavigate();
  const qc = useQueryClient();
  const { data: tags = [] } = useQuery({
    queryKey: ["tags"],
    queryFn: () => api<Tag[]>("/tags"),
  });
  const [title, setTitle] = useState(initial?.title ?? "");
  const [summary, setSummary] = useState(initial?.summary ?? "");
  const [body, setBody] = useState(initial?.body ?? "");
  const [tagIds, setTagIds] = useState<number[]>(
    initial?.tags.map((t) => t.id) ?? [],
  );
  const [version, setVersion] = useState(initial?.version ?? 0);
  const [status, setStatus] = useState<Article["status"]>(
    initial?.status ?? "draft",
  );
  const [preview, setPreview] = useState(false);
  const [alt, setAlt] = useState("");
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);
  async function save(): Promise<Article> {
    const result = await api<Article>(
      id ? `/manage/articles/${id}` : "/manage/articles",
      {
        method: id ? "PATCH" : "POST",
        body: json({ title, summary, body, tagIds, version }),
      },
    );
    setVersion(result.version);
    setStatus(result.status);
    void qc.invalidateQueries({ queryKey: ["manage-articles"] });
    return result;
  }
  async function doSave() {
    setBusy(true);
    setMessage("");
    try {
      const saved = await save();
      if (!id) navigate(`/manage/articles/${saved.id}/edit`, { replace: true });
      setMessage(saved.status === "published" ? "文章已保存。" : "草稿已保存。");
    } catch (e) {
      setMessage((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  async function transition(action: "publish" | "unpublish" | "delete") {
    if (
      action === "delete" &&
      !window.confirm("确定删除这篇文章？可以在回收站恢复。")
    )
      return;
    setBusy(true);
    setMessage("");
    try {
      let target = id;
      if (action === "publish") {
        const saved = await save();
        target = saved.id;
      }
      const result = await api<Article>(
        `/manage/articles/${target}/${action}`,
        { method: action === "delete" ? "DELETE" : "POST" },
      );
      setVersion(result.version);
      setStatus(result.status);
      void qc.invalidateQueries({ queryKey: ["manage-articles"] });
      void qc.invalidateQueries({ queryKey: ["manage-article", target] });
      void qc.invalidateQueries({ queryKey: ["article", target] });
      if (action === "delete") navigate("/manage/trash");
      else if (!id)
        navigate(`/manage/articles/${target}/edit`, { replace: true });
      else setMessage(action === "publish" ? "文章已发布。" : "文章已下线。");
    } catch (e) {
      setMessage((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  async function upload(file?: File) {
    if (!file) return;
    if (!alt.trim()) {
      setMessage("先填写图片的替代文字。");
      return;
    }
    setBusy(true);
    setMessage("");
    try {
      const form = new FormData();
      form.append("file", file);
      const result = await api<{ id: number; url: string }>("/manage/images", {
        method: "POST",
        body: form,
      });
      setBody((value) => value + `\n\n![${alt.trim()}](${result.url})\n`);
      setAlt("");
      setMessage("图片已插入正文，保存文章后生效。");
    } catch (e) {
      setMessage((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <Workspace
      title={id ? "编辑文章" : "写新文章"}
      subtitle="写下文字，先预览，再决定何时公开。"
      action={
        <Link className="button soft" to="/manage">
          <ArrowLeft size={16} />
          返回列表
        </Link>
      }
    >
      <div className="editor-toolbar">
        <span className={`status ${status}`}>{statusText[status]}</span>
        <div>
          <button
            className="button soft button-small"
            onClick={() => setPreview(!preview)}
          >
            {preview ? "继续编辑" : "预览文章"}
          </button>
          <button
            className="button soft button-small"
            onClick={doSave}
            disabled={busy}
          >
            <Save size={16} />
            {status === "published" ? "保存修改" : "保存草稿"}
          </button>
          {status !== "published" ? (
            <button
              className="button button-small"
              onClick={() => transition("publish")}
              disabled={busy || !title.trim() || !body.trim()}
            >
              <Upload size={16} />
              发布
            </button>
          ) : (
            <button
              className="button button-small"
              onClick={() => transition("unpublish")}
              disabled={busy}
            >
              下线
            </button>
          )}
        </div>
      </div>
      {message ? (
        <p
          className={message.includes("已") ? "form-success" : "form-error"}
          role="status"
        >
          {message}
        </p>
      ) : null}
      {preview ? (
        <div className="preview-panel">
          <span className="eyebrow">PREVIEW</span>
          <h1>{title || "未命名文章"}</h1>
          <p>{summary}</p>
          <MarkdownView body={body} />
        </div>
      ) : (
        <div className="editor-layout">
          <div className="editor-main">
            <label>
              标题 <span>*</span>
              <input
                maxLength={255}
                value={title}
                onChange={(e) => setTitle(e.target.value)}
                placeholder="给文章一个清楚的标题"
              />
            </label>
            <label>
              摘要
              <textarea
                className="summary-input"
                value={summary}
                onChange={(e) => setSummary(e.target.value)}
                placeholder="用几句话介绍这篇文章"
              />
            </label>
            <label>
              正文 <span>*</span>
              <textarea
                className="body-input"
                value={body}
                onChange={(e) => setBody(e.target.value)}
                placeholder="从这里开始写作，支持 Markdown…"
              />
            </label>
          </div>
          <aside className="editor-side">
            <h3>文章设置</h3>
            <label>标签</label>
            <div className="tag-select">
              {tags.length ? (
                tags.map((t) => (
                  <label key={t.id}>
                    <input
                      type="checkbox"
                      checked={tagIds.includes(t.id)}
                      onChange={(e) =>
                        setTagIds((ids) =>
                          e.target.checked
                            ? [...ids, t.id]
                            : ids.filter((x) => x !== t.id),
                        )
                      }
                    />
                    {t.name}
                  </label>
                ))
              ) : (
                <p>暂无标签，请联系管理员添加。</p>
              )}
            </div>
            <hr />
            <h3>插入图片</h3>
            <p>支持 JPG、PNG、WebP，单张不超过 10 MB。</p>
            <label>
              替代文字
              <input
                value={alt}
                onChange={(e) => setAlt(e.target.value)}
                placeholder="描述图片内容"
              />
            </label>
            <label className="upload-button">
              <ImagePlus size={17} />
              选择图片
              <input
                type="file"
                accept="image/jpeg,image/png,image/webp"
                onChange={(e) => {
                  void upload(e.target.files?.[0]);
                  e.target.value = "";
                }}
              />
            </label>
          </aside>
        </div>
      )}
      {id ? (
        <div className="editor-foot">
          {status === "published" ? (
            <Link to={articleUrl(id)}>查看公开文章</Link>
          ) : null}
          <button
            className="text-button danger"
            onClick={() => transition("delete")}
            disabled={busy}
          >
            <Trash2 size={16} />
            删除文章
          </button>
        </div>
      ) : null}
    </Workspace>
  );
}

export function Trash() {
  const qc = useQueryClient();
  const { data, isPending } = useQuery({
    queryKey: ["trash"],
    queryFn: () => api<Page<Article>>("/manage/trash"),
  });
  const restore = useMutation({
    mutationFn: (id: number) =>
      api<Article>(`/manage/articles/${id}/restore`, { method: "POST" }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["trash"] });
      void qc.invalidateQueries({ queryKey: ["manage-articles"] });
    },
  });
  return (
    <Workspace title="文章回收站" subtitle="误删的文章可以从这里找回来。">
      <div className="list-panel">
        {isPending ? (
          <State>正在加载…</State>
        ) : data?.items.length ? (
          data.items.map((a) => (
            <div className="manage-row" key={a.id}>
              <div>
                <h3>{a.title}</h3>
                <small>
                  作者：{a.authorName} · 删除于 {dateTimeText(a.deletedAt)}
                </small>
              </div>
              <button
                className="button soft button-small"
                onClick={() => restore.mutate(a.id)}
              >
                <RotateCcw size={16} />
                恢复
              </button>
            </div>
          ))
        ) : (
          <State>回收站里没有文章。</State>
        )}
      </div>
      {restore.error ? (
        <p className="form-error">{restore.error.message}</p>
      ) : null}
    </Workspace>
  );
}

export function ManageComments() {
  const qc = useQueryClient();
  const { data, isPending } = useQuery({
    queryKey: ["manage-comments"],
    queryFn: () => api<Page<Comment>>("/manage/comments"),
  });
  const remove = useMutation({
    mutationFn: (id: number) => api(`/comments/${id}`, { method: "DELETE" }),
    onSuccess: () =>
      void qc.invalidateQueries({ queryKey: ["manage-comments"] }),
  });
  return (
    <Workspace title="评论管理" subtitle="查看读者留下的想法。">
      <div className="list-panel">
        {isPending ? (
          <State>正在加载…</State>
        ) : data?.items.length ? (
          data.items.map((c) => (
            <div className="manage-row" key={c.id}>
              <div>
                <h3>{c.nickname}</h3>
                <p>{c.content}</p>
                <small>
                  文章 #{c.articleId} · {dateTimeText(c.createdAt)}
                </small>
              </div>
              <button
                className="text-button danger"
                onClick={() => {
                  if (window.confirm("确定删除这条评论？删除后无法恢复。"))
                    remove.mutate(c.id);
                }}
              >
                删除
              </button>
            </div>
          ))
        ) : (
          <State>暂无评论。</State>
        )}
      </div>
      {remove.error ? (
        <p className="form-error">{remove.error.message}</p>
      ) : null}
    </Workspace>
  );
}

export function ManageUsers() {
  const qc = useQueryClient();
  const { data = [], isPending } = useQuery({
    queryKey: ["users"],
    queryFn: () => api<User[]>("/admin/users"),
  });
  const mutation = useMutation({
    mutationFn: ({ id, role }: { id: number; role: "reader" | "author" }) =>
      api(`/admin/users/${id}/role`, { method: "PATCH", body: json({ role }) }),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["users"] }),
  });
  return (
    <Workspace title="用户权限" subtitle="由管理员决定谁可以成为博主。">
      <div className="list-panel">
        {isPending ? (
          <State>正在加载…</State>
        ) : (
          data.map((u) => (
            <div className="manage-row" key={u.id}>
              <div>
                <h3>{u.nickname}</h3>
                <p>{u.email}</p>
                <small>
                  {u.role === "admin"
                    ? "管理员"
                    : u.role === "author"
                      ? "博主"
                      : "普通用户"}
                </small>
              </div>
              {u.role !== "admin" ? (
                <button
                  className="button soft button-small"
                  onClick={() =>
                    mutation.mutate({
                      id: u.id,
                      role: u.role === "author" ? "reader" : "author",
                    })
                  }
                >
                  {u.role === "author" ? "撤销博主" : "设为博主"}
                </button>
              ) : null}
            </div>
          ))
        )}
      </div>
      {mutation.error ? (
        <p className="form-error">{mutation.error.message}</p>
      ) : null}
    </Workspace>
  );
}

export function ManageTags() {
  const qc = useQueryClient();
  const { data = [], isPending } = useQuery({
    queryKey: ["tags"],
    queryFn: () => api<Tag[]>("/tags"),
  });
  const [name, setName] = useState("");
  const [error, setError] = useState("");
  const reload = () => void qc.invalidateQueries({ queryKey: ["tags"] });
  async function create(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    try {
      await api("/admin/tags", { method: "POST", body: json({ name }) });
      setName("");
      reload();
    } catch (e) {
      setError((e as Error).message);
    }
  }
  async function rename(t: Tag) {
    const next = window.prompt("新的标签名", t.name);
    if (!next || next === t.name) return;
    try {
      await api(`/admin/tags/${t.id}`, {
        method: "PATCH",
        body: json({ name: next }),
      });
      reload();
    } catch (e) {
      setError((e as Error).message);
    }
  }
  async function remove(t: Tag) {
    if (!window.confirm(`删除「${t.name}」？这个标签会从所有相关文章中移除。`))
      return;
    try {
      await api(`/admin/tags/${t.id}`, { method: "DELETE" });
      reload();
    } catch (e) {
      setError((e as Error).message);
    }
  }
  return (
    <Workspace title="标签管理" subtitle="用简洁的主题帮助读者发现文章。">
      <form className="inline-form" onSubmit={create}>
        <label htmlFor="tagName">新标签</label>
        <input
          id="tagName"
          required
          maxLength={80}
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="输入标签名"
        />
        <button className="button" type="submit">
          添加标签
        </button>
      </form>
      {error ? <p className="form-error">{error}</p> : null}
      <div className="list-panel">
        {isPending ? (
          <State>正在加载…</State>
        ) : data.length ? (
          data.map((t) => (
            <div className="manage-row" key={t.id}>
              <h3>#{t.name}</h3>
              <div>
                <button className="text-button" onClick={() => rename(t)}>
                  改名
                </button>
                <button
                  className="text-button danger"
                  onClick={() => remove(t)}
                >
                  删除
                </button>
              </div>
            </div>
          ))
        ) : (
          <State>还没有标签。</State>
        )}
      </div>
    </Workspace>
  );
}
