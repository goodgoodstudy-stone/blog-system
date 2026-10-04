import {
  QueryClient,
  QueryClientProvider,
  useQueryClient,
} from "@tanstack/react-query";
import {
  BrowserRouter,
  Link,
  NavLink,
  Route,
  Routes,
  useNavigate,
} from "react-router-dom";
import { BookOpen, Heart, LogIn, LogOut, PenLine, Shield } from "lucide-react";
import { api } from "./api";
import { useCurrentUser } from "./auth";
import { Home, ArticleDetail, Favorites } from "./pages/Public";
import { Login, Register } from "./pages/Auth";
import {
  ArticleEditor,
  ManageArticles,
  ManageComments,
  ManageTags,
  ManageUsers,
  Trash,
} from "./pages/Manage";

const queryClient = new QueryClient({
  defaultOptions: { queries: { retry: false, staleTime: 30_000 } },
});
function Header() {
  const { data: user } = useCurrentUser();
  const qc = useQueryClient();
  const navigate = useNavigate();
  async function logout() {
    await api("/auth/logout", { method: "POST" });
    qc.clear();
    navigate("/");
  }
  return (
    <header className="site-header">
      <div className="header-inner">
        <Link to="/" className="brand">
          <span className="brand-mark">拾</span>
          <span>
            <strong>拾光博客</strong>
            <small>写下值得留住的片刻</small>
          </span>
        </Link>
        <nav aria-label="主导航" className="main-nav">
          <NavLink to="/" end>
            <BookOpen size={17} />
            文章
          </NavLink>
          {user ? (
            <NavLink to="/favorites">
              <Heart size={17} />
              我的收藏
            </NavLink>
          ) : null}
          {user && user.role !== "reader" ? (
            <NavLink to="/manage">
              <PenLine size={17} />
              创作台
            </NavLink>
          ) : null}
          {user?.role === "admin" ? (
            <NavLink to="/admin/users">
              <Shield size={17} />
              管理
            </NavLink>
          ) : null}
        </nav>
        <div className="account-nav">
          {user ? (
            <>
              <span className="greeting">你好，{user.nickname}</span>
              <button className="text-button" onClick={logout}>
                <LogOut size={16} />
                退出
              </button>
            </>
          ) : (
            <Link to="/login" className="button button-small">
              <LogIn size={16} />
              登录 / 注册
            </Link>
          )}
        </div>
      </div>
    </header>
  );
}

export function Gate({
  children,
  role,
}: {
  children: React.ReactNode;
  role?: "author" | "admin";
}) {
  const { data: user, isPending } = useCurrentUser();
  if (isPending) return <div className="state-box">正在确认登录状态…</div>;
  if (!user)
    return (
      <div className="state-box">
        <h2>请先登录</h2>
        <p>登录后即可继续。</p>
        <Link className="button" to="/login">
          前往登录
        </Link>
      </div>
    );
  if (role === "admin" && user.role !== "admin")
    return <div className="state-box">没有管理员权限。</div>;
  if (role === "author" && user.role === "reader")
    return <div className="state-box">没有博主权限。</div>;
  return <>{children}</>;
}

function Shell() {
  return (
    <>
      <Header />
      <main className="main-content">
        <Routes>
          <Route path="/" element={<Home />} />
          <Route path="/articles/:id" element={<ArticleDetail />} />
          <Route path="/login" element={<Login />} />
          <Route path="/register" element={<Register />} />
          <Route
            path="/favorites"
            element={
              <Gate>
                <Favorites />
              </Gate>
            }
          />
          <Route
            path="/manage"
            element={
              <Gate role="author">
                <ManageArticles />
              </Gate>
            }
          />
          <Route
            path="/manage/new"
            element={
              <Gate role="author">
                <ArticleEditor />
              </Gate>
            }
          />
          <Route
            path="/manage/articles/:id/edit"
            element={
              <Gate role="author">
                <ArticleEditor />
              </Gate>
            }
          />
          <Route
            path="/manage/trash"
            element={
              <Gate role="author">
                <Trash />
              </Gate>
            }
          />
          <Route
            path="/manage/comments"
            element={
              <Gate role="author">
                <ManageComments />
              </Gate>
            }
          />
          <Route
            path="/admin/users"
            element={
              <Gate role="admin">
                <ManageUsers />
              </Gate>
            }
          />
          <Route
            path="/admin/tags"
            element={
              <Gate role="admin">
                <ManageTags />
              </Gate>
            }
          />
          <Route
            path="*"
            element={
              <div className="state-box">
                <h2>没有找到这个页面</h2>
                <Link to="/">返回首页</Link>
              </div>
            }
          />
        </Routes>
      </main>
      <footer className="site-footer">
        <span>拾光博客 · 让每一篇文字都有归处</span>
        <span>一个可以亲手体验的开源博客</span>
      </footer>
    </>
  );
}

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <Shell />
      </BrowserRouter>
    </QueryClientProvider>
  );
}
