import type { ButtonHTMLAttributes, InputHTMLAttributes, ReactNode } from "react";

type Variant = "primary" | "secondary" | "danger" | "ghost";

const variants: Record<Variant, string> = {
  primary: "bg-emerald-500 hover:bg-emerald-400 text-slate-950 font-semibold",
  secondary: "bg-slate-700 hover:bg-slate-600 text-slate-100",
  danger: "bg-rose-600 hover:bg-rose-500 text-white",
  ghost: "bg-transparent hover:bg-slate-800 text-slate-200",
};

export function Button({
  variant = "primary",
  className = "",
  ...rest
}: ButtonHTMLAttributes<HTMLButtonElement> & { variant?: Variant }) {
  return (
    <button
      className={`px-3 py-2 sm:py-1.5 rounded-md text-sm transition disabled:opacity-40 disabled:cursor-not-allowed ${variants[variant]} ${className}`}
      {...rest}
    />
  );
}

export function Input({ className = "", ...rest }: InputHTMLAttributes<HTMLInputElement>) {
  return (
    <input
      className={`px-3 py-2 rounded-md bg-slate-800 border border-slate-700 focus:border-emerald-400 outline-none text-base sm:text-sm w-full ${className}`}
      {...rest}
    />
  );
}

export function Card({ title, children, className = "" }: { title?: ReactNode; children: ReactNode; className?: string }) {
  return (
    <section className={`bg-slate-900/80 border border-slate-800 rounded-xl p-3 sm:p-4 ${className}`}>
      {title && <h2 className="text-sm uppercase tracking-wide text-slate-400 mb-3">{title}</h2>}
      {children}
    </section>
  );
}

export function Money({ value }: { value: number }) {
  return <span className="tabular-nums">${value.toLocaleString()}</span>;
}

/** Seat colours: index in the seating order → a stable colour. */
export const seatColors = ["#f43f5e", "#3b82f6", "#22c55e", "#eab308", "#a855f7", "#f97316", "#06b6d4", "#ec4899"];
