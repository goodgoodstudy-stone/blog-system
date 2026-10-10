export type User = {
  id: number;
  email: string;
  nickname: string;
  role: "reader" | "author" | "admin";
};
export type Tag = { id: number; name: string };
export type Article = {
  id: number;
  authorId: number;
  authorName: string;
  title: string;
  summary: string;
  body: string;
  status: "draft" | "published" | "unpublished" | "deleted";
  publishedAt: string | null;
  deletedAt?: string | null;
  createdAt: string;
  updatedAt: string;
  version: number;
  tags: Tag[];
  favorited: boolean;
};
export type Comment = {
  id: number;
  articleId: number;
  userId: number;
  nickname: string;
  content: string;
  createdAt: string;
};
export type Page<T> = {
  items: T[];
  total: number;
  page: number;
  pageSize: number;
};
export type BlogRequestError = { code: string; message: string };

export async function blogRequest<T>(
  path: string,
  init: RequestInit = {},
): Promise<T> {
  const isForm = init.body instanceof FormData;
  const response = await fetch(`/api/v1${path}`, {
    ...init,
    credentials: "include",
    headers: {
      ...(!isForm && init.body ? { "Content-Type": "application/json" } : {}),
      ...init.headers,
    },
  });
  if (!response.ok) {
    const data = (await response.json().catch(() => ({
      code: "network",
      message: "请求失败，请稍后重试",
    }))) as BlogRequestError;
    throw new Error(data.message || "请求失败");
  }
  return response.json() as Promise<T>;
}

export const json = (value: unknown) => JSON.stringify(value);
export const articleUrl = (id: number) => `/articles/${id}`;
export const dateText = (value?: string | null) =>
  value
    ? new Date(value).toLocaleDateString("zh-CN", {
        year: "numeric",
        month: "long",
        day: "numeric",
      })
    : "尚未发布";
export const dateTimeText = (value?: string | null) =>
  value
    ? new Date(value).toLocaleString("zh-CN", {
        year: "numeric",
        month: "numeric",
        day: "numeric",
        hour: "2-digit",
        minute: "2-digit",
      })
    : "无";
export const statusText: Record<Article["status"], string> = {
  draft: "草稿",
  published: "已发布",
  unpublished: "已下线",
  deleted: "回收站",
};
