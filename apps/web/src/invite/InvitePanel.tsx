import { useEffect, useState } from "react";
import type { Lobby } from "@monopsony/protocol";
import { Button, Card } from "@/lib/ui";
import { QrCode } from "./QrCode";
import { inviteUrl } from "./code";

/** Copies text, falling back to a hidden selection where the API is blocked. */
async function copy(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    // Insecure origins and older mobile browsers have no clipboard API.
  }
  const el = document.createElement("textarea");
  el.value = text;
  el.setAttribute("readonly", "");
  el.style.position = "fixed";
  el.style.opacity = "0";
  document.body.appendChild(el);
  el.select();
  let ok = false;
  try {
    ok = document.execCommand("copy");
  } catch {
    ok = false;
  }
  document.body.removeChild(el);
  return ok;
}

/**
 * The lobby's share card: a link to hand out, a QR code for the people in the
 * room, and the raw code for anyone typing it in by hand.
 */
export function InvitePanel({ lobby }: { lobby: Lobby }) {
  const url = inviteUrl(lobby);
  const [copied, setCopied] = useState(false);
  const [showQr, setShowQr] = useState(false);
  const canShare = typeof navigator !== "undefined" && typeof navigator.share === "function";

  useEffect(() => {
    if (!copied) return;
    const t = setTimeout(() => setCopied(false), 2000);
    return () => clearTimeout(t);
  }, [copied]);

  const share = async () => {
    try {
      await navigator.share({ title: lobby.name || "Monopsony", text: "Join my table on Monopsony", url });
    } catch {
      // The sheet was dismissed, or the target refused it. Nothing to report.
    }
  };

  return (
    <Card title="Invite friends">
      <div className="flex flex-col sm:flex-row sm:items-center gap-4">
        <div className="flex-1 min-w-0 space-y-2">
          <div className="flex gap-2">
            <input
              readOnly
              value={url}
              onFocus={(e) => e.currentTarget.select()}
              aria-label="Invite link"
              className="flex-1 min-w-0 px-3 py-2 rounded-md bg-slate-800 border border-slate-700 text-sm text-slate-300 font-mono truncate outline-none focus:border-emerald-400"
            />
            <Button onClick={async () => setCopied(await copy(url))}>{copied ? "Copied!" : "Copy"}</Button>
          </div>
          <div className="flex flex-wrap gap-2">
            {canShare && (
              <Button variant="secondary" onClick={share}>
                Share…
              </Button>
            )}
            <Button variant="ghost" className="sm:hidden" onClick={() => setShowQr((v) => !v)} aria-expanded={showQr}>
              {showQr ? "Hide QR code" : "Show QR code"}
            </Button>
          </div>
          {lobby.inviteCode ? (
            <p className="text-sm text-slate-400">
              Or enter the code{" "}
              <code className="bg-slate-800 px-2 py-0.5 rounded font-mono text-emerald-300 select-all">{lobby.inviteCode}</code>{" "}
              in the lobby.
            </p>
          ) : (
            <p className="text-sm text-slate-400">Anyone with the link can take a seat at this table.</p>
          )}
        </div>
        <div className={`${showQr ? "flex" : "hidden"} sm:flex flex-col items-center gap-1 shrink-0`}>
          <QrCode value={url} label={`QR code linking to ${url}`} className="w-36 h-36 p-1" />
          <span className="text-xs text-slate-500">Scan to join</span>
        </div>
      </div>
    </Card>
  );
}
