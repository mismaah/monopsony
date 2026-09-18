import type { ButtonHTMLAttributes, ReactNode } from "react";

export function Btn({ tone = "primary", className = "", ...rest }: ButtonHTMLAttributes<HTMLButtonElement> & { tone?: "primary" | "secondary" | "danger" }) {
  const tones = {
    primary: "bg-emerald-500 text-zinc-950 hover:bg-emerald-400",
    secondary: "bg-zinc-800 text-zinc-100 hover:bg-zinc-700",
    danger: "bg-rose-600 text-white hover:bg-rose-500",
  };
  return <button className={`px-3 py-1 rounded text-sm font-medium disabled:opacity-40 ${tones[tone]} ${className}`} {...rest} />;
}

export function Panel({ title, actions, children }: { title: string; actions?: ReactNode; children: ReactNode }) {
  return (
    <section className="mb-6">
      <div className="flex items-center gap-3 mb-3">
        <h1 className="text-xl font-semibold">{title}</h1>
        <div className="flex-1" />
        {actions}
      </div>
      <div className="bg-zinc-900 border border-zinc-800 rounded-xl p-4">{children}</div>
    </section>
  );
}

export function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="flex flex-col gap-1 text-sm">
      <span className="text-zinc-400 text-xs uppercase tracking-wide">{label}</span>
      {children}
    </label>
  );
}

export function Notice({ kind, text }: { kind: "ok" | "err"; text: string }) {
  return <div className={`text-sm px-3 py-2 rounded ${kind === "ok" ? "bg-emerald-500/15 text-emerald-300" : "bg-rose-500/15 text-rose-300"}`}>{text}</div>;
}
