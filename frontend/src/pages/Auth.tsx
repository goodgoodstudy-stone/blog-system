import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "react-router-dom";
import { ArrowRight, LockKeyhole } from "lucide-react";
import { api, json, type User } from "../api";

function AuthPage({ register }: { register: boolean }) {
  const [email, setEmail] = useState("");
  const [nickname, setNickname] = useState("");
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const qc = useQueryClient();
  const navigate = useNavigate();
  const mutation = useMutation({
    mutationFn: () =>
      api<User>(register ? "/auth/register" : "/auth/login", {
        method: "POST",
        body: json(
          register
            ? { email, nickname, password, confirm }
            : { email, password },
        ),
      }),
    onSuccess: (u) => {
      qc.setQueryData(["me"], u);
      navigate("/");
    },
  });
  return (
    <div className="auth-wrap">
      <div className="auth-story">
        <span className="eyebrow">A PLACE FOR YOUR WORDS</span>
        <h1>
          把想法写下来，
          <br />
          把喜欢的留下来。
        </h1>
        <p>与文字相遇，与有趣的人交流。每一篇文章，都有属于它的温度。</p>
        <div className="auth-ornament" aria-hidden="true">
          ✺
        </div>
      </div>
      <form
        className="auth-card"
        onSubmit={(e) => {
          e.preventDefault();
          mutation.mutate();
        }}
      >
        <div className="auth-icon">
          <LockKeyhole size={22} />
        </div>
        <h2>{register ? "创建账号" : "欢迎回来"}</h2>
        <p>
          {register
            ? "注册后就能收藏与评论喜欢的文章。"
            : "登录后，继续你的阅读和写作。"}
        </p>
        <label>
          邮箱
          <input
            type="email"
            required
            autoComplete="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            placeholder="name@example.com"
          />
        </label>
        {register ? (
          <label>
            昵称
            <input
              required
              maxLength={100}
              value={nickname}
              onChange={(e) => setNickname(e.target.value)}
              placeholder="怎么称呼你？"
            />
          </label>
        ) : null}
        <label>
          密码
          <input
            type="password"
            required
            minLength={register ? 8 : 1}
            autoComplete={register ? "new-password" : "current-password"}
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            placeholder={register ? "至少 8 位" : "输入密码"}
          />
        </label>
        {register ? (
          <label>
            确认密码
            <input
              type="password"
              required
              minLength={8}
              autoComplete="new-password"
              value={confirm}
              onChange={(e) => setConfirm(e.target.value)}
              placeholder="再输入一次密码"
            />
          </label>
        ) : null}
        {mutation.error ? (
          <p className="form-error" role="alert">
            {mutation.error.message}
          </p>
        ) : null}
        <button
          className="button wide"
          type="submit"
          disabled={mutation.isPending}
        >
          {mutation.isPending ? "请稍候…" : register ? "注册并进入" : "登录"}{" "}
          <ArrowRight size={16} />
        </button>
        <p className="auth-switch">
          {register ? "已有账号？" : "还没有账号？"}{" "}
          <Link to={register ? "/login" : "/register"}>
            {register ? "去登录" : "立即注册"}
          </Link>
        </p>
      </form>
    </div>
  );
}

export function Login() {
  return <AuthPage register={false} />;
}
export function Register() {
  return <AuthPage register />;
}
