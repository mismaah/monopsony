import type { Envelope, CommandType, CommandPayloads } from "@monopsony/protocol";
import { getAccessToken, refreshSession } from "./http";

type Handler = (payload: unknown, env: Envelope) => void;

/**
 * GameSocket keeps one WebSocket per tab, reconnects with backoff, and turns
 * Command frames into promises resolved by the matching Ack/Error.
 */
export class GameSocket {
  private ws: WebSocket | null = null;
  private handlers = new Map<string, Set<Handler>>();
  private pending = new Map<string, { resolve: () => void; reject: (e: Error) => void }>();
  private refSeq = 0;
  private joined = new Map<string, number>(); // gameId -> lastSeq seen
  private backoff = 500;
  private closed = false;
  private opening = false;
  status: "connecting" | "open" | "closed" = "closed";
  onStatus?: (s: GameSocket["status"]) => void;

  connect() {
    this.closed = false;
    void this.open();
  }

  close() {
    this.closed = true;
    const ws = this.ws;
    this.ws = null;
    ws?.close();
    this.pending.forEach((p) => p.reject(new SocketError("disconnected", "connection closed")));
    this.pending.clear();
    this.setStatus("closed");
  }

  /**
   * open establishes the single connection. It is idempotent: a socket that
   * is already connecting/open is left alone (StrictMode runs the bootstrap
   * effect twice, and every reconnect timer lands here too). Each socket's
   * handlers ignore themselves once it is no longer `this.ws`, so a stale
   * socket can never re-subscribe or spawn another reconnect. Without these
   * guards several sockets end up subscribed to the same game and every
   * event is delivered (and animated) once per socket.
   */
  private async open() {
    if (this.closed || this.opening) return;
    if (this.ws && (this.ws.readyState === WebSocket.CONNECTING || this.ws.readyState === WebSocket.OPEN)) return;
    this.opening = true;
    try {
      let token = getAccessToken();
      if (!token && (await refreshSession())) token = getAccessToken();
      if (!token || this.closed) return;
      // Re-check: close()/connect() may have raced with the token refresh above.
      if (this.ws && (this.ws.readyState === WebSocket.CONNECTING || this.ws.readyState === WebSocket.OPEN)) return;
      this.setStatus("connecting");
      const proto = location.protocol === "https:" ? "wss" : "ws";
      const ws = new WebSocket(`${proto}://${location.host}/ws?token=${encodeURIComponent(token)}`);
      this.ws = ws;
      this.attach(ws);
    } finally {
      this.opening = false;
    }
  }

  private attach(ws: WebSocket) {
    const current = () => this.ws === ws;
    ws.onopen = () => {
      if (!current()) {
        ws.close();
        return;
      }
      this.backoff = 500;
      this.setStatus("open");
      // Re-subscribe after a reconnect, asking only for the events we missed.
      for (const [gameId, lastSeq] of this.joined) {
        this.raw({ t: "JoinGame", p: { gameId, lastSeq } });
      }
    };
    ws.onmessage = (m) => {
      if (!current()) return;
      const env = JSON.parse(m.data) as Envelope;
      if (env.ref && (env.t === "Ack" || env.t === "Error")) {
        const p = this.pending.get(env.ref);
        if (p) {
          this.pending.delete(env.ref);
          if (env.t === "Ack") p.resolve();
          else {
            const e = env.p as { code: string; message: string };
            p.reject(new SocketError(e.code, e.message));
          }
          if (env.t === "Ack") return;
        }
      }
      if (env.t === "Error" && !env.ref) {
        // Unsolicited errors are room-level notices. "rejoin" means the node
        // hosting the game went away: subscribe again and the server resolves
        // (or adopts) the new host.
        const e = env.p as { code: string; message: string };
        if (e.code === "rejoin" && this.joined.has(e.message)) {
          setTimeout(() => this.raw({ t: "JoinGame", p: { gameId: e.message, lastSeq: this.joined.get(e.message) ?? 0 } }), 250);
          return;
        }
      }
      if (env.t === "Event") {
        const ev = env.p as { gameId: string; seq: number };
        this.joined.set(ev.gameId, Math.max(ev.seq, this.joined.get(ev.gameId) ?? 0));
      }
      if (env.t === "Snapshot" || env.t === "Update") {
        const s = env.p as { gameId: string; seq?: number; state?: { seq: number } };
        const seq = s.seq ?? s.state?.seq ?? 0;
        this.joined.set(s.gameId, Math.max(seq, this.joined.get(s.gameId) ?? 0));
      }
      this.handlers.get(env.t)?.forEach((h) => h(env.p, env));
      this.handlers.get("*")?.forEach((h) => h(env.p, env));
    };
    ws.onclose = () => {
      if (!current()) return; // superseded or closed on purpose
      this.ws = null;
      this.setStatus("closed");
      this.pending.forEach((p) => p.reject(new SocketError("disconnected", "connection lost")));
      this.pending.clear();
      if (!this.closed) {
        setTimeout(() => void this.open(), this.backoff);
        this.backoff = Math.min(this.backoff * 2, 8000);
      }
    };
  }

  private setStatus(s: GameSocket["status"]) {
    this.status = s;
    this.onStatus?.(s);
  }

  on(type: string, h: Handler): () => void {
    if (!this.handlers.has(type)) this.handlers.set(type, new Set());
    this.handlers.get(type)!.add(h);
    return () => this.handlers.get(type)?.delete(h);
  }

  private raw(env: Envelope) {
    if (this.ws?.readyState === WebSocket.OPEN) this.ws.send(JSON.stringify(env));
  }

  join(gameId: string) {
    if (!this.joined.has(gameId)) this.joined.set(gameId, 0);
    this.raw({ t: "JoinGame", p: { gameId, lastSeq: this.joined.get(gameId) ?? 0 } });
  }

  leave(gameId: string) {
    this.joined.delete(gameId);
    this.raw({ t: "LeaveGame", p: { gameId } });
  }

  chat(gameId: string, text: string) {
    this.raw({ t: "Chat", p: { gameId, text } });
  }

  command<K extends CommandType>(gameId: string, type: K, payload?: CommandPayloads[K]): Promise<void> {
    const ref = `c${++this.refSeq}`;
    return new Promise((resolve, reject) => {
      if (this.ws?.readyState !== WebSocket.OPEN) {
        reject(new SocketError("disconnected", "not connected"));
        return;
      }
      this.pending.set(ref, { resolve, reject });
      this.raw({ t: "Command", ref, p: { gameId, type, payload: payload ?? {} } });
      setTimeout(() => {
        if (this.pending.delete(ref)) reject(new SocketError("timeout", "no reply from server"));
      }, 10000);
    });
  }
}

export class SocketError extends Error {
  constructor(public code: string, message: string) {
    super(message);
  }
}

export const socket = new GameSocket();
