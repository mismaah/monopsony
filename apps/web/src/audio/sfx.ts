import { useSettings } from "@/store/settings";

/**
 * Procedural sound effects on Web Audio. Every cue is synthesised from
 * oscillators and filtered noise, so there are no audio assets to load and
 * nothing to license. The engine is lazy: the AudioContext is created on the
 * first play and resumed on the first user gesture (browsers block autoplay).
 */

type Wave = OscillatorType;

interface ToneOpts {
  freq: number;
  /** Exponential glide target, if any. */
  to?: number;
  type?: Wave;
  at?: number;
  dur: number;
  gain?: number;
  attack?: number;
}

interface NoiseOpts {
  at?: number;
  dur: number;
  gain?: number;
  filter?: { type: BiquadFilterType; freq: number; q?: number };
}

class Synth {
  constructor(
    private ctx: AudioContext,
    private out: GainNode,
    private t0: number,
  ) {}

  tone({ freq, to, type = "sine", at = 0, dur, gain = 0.3, attack = 0.004 }: ToneOpts) {
    const t = this.t0 + at;
    const osc = this.ctx.createOscillator();
    const g = this.ctx.createGain();
    osc.type = type;
    osc.frequency.setValueAtTime(freq, t);
    if (to) osc.frequency.exponentialRampToValueAtTime(to, t + dur);
    g.gain.setValueAtTime(0, t);
    g.gain.linearRampToValueAtTime(gain, t + attack);
    g.gain.exponentialRampToValueAtTime(0.0001, t + dur);
    osc.connect(g).connect(this.out);
    osc.start(t);
    osc.stop(t + dur + 0.02);
  }

  noise({ at = 0, dur, gain = 0.3, filter }: NoiseOpts) {
    const t = this.t0 + at;
    const src = this.ctx.createBufferSource();
    src.buffer = noiseBuffer(this.ctx);
    const g = this.ctx.createGain();
    g.gain.setValueAtTime(gain, t);
    g.gain.exponentialRampToValueAtTime(0.0001, t + dur);
    let head: AudioNode = src;
    if (filter) {
      const f = this.ctx.createBiquadFilter();
      f.type = filter.type;
      f.frequency.value = filter.freq;
      f.Q.value = filter.q ?? 1;
      head = src.connect(f);
    }
    head.connect(g).connect(this.out);
    src.start(t);
    src.stop(t + dur + 0.02);
  }

  /** Short pitched hit with an octave partial — bells, coins, pings. */
  ping(freq: number, at: number, dur: number, gain = 0.25, type: Wave = "sine") {
    this.tone({ freq, at, dur, gain, type });
    this.tone({ freq: freq * 2.01, at, dur: dur * 0.6, gain: gain * 0.35, type });
  }
}

const noiseCache = new WeakMap<AudioContext, AudioBuffer>();
function noiseBuffer(ctx: AudioContext) {
  let b = noiseCache.get(ctx);
  if (!b) {
    b = ctx.createBuffer(1, ctx.sampleRate, ctx.sampleRate);
    const d = b.getChannelData(0);
    for (let i = 0; i < d.length; i++) d[i] = Math.random() * 2 - 1;
    noiseCache.set(ctx, b);
  }
  return b;
}

const C4 = 261.63;
const note = (semis: number) => C4 * Math.pow(2, semis / 12);

/** Every cue the game can play. Offsets and durations are seconds. */
export const cues = {
  /** Dice shaken in a cup, then dropped. */
  dice(s) {
    for (let i = 0; i < 6; i++) {
      s.noise({ at: i * 0.075 + Math.random() * 0.02, dur: 0.04, gain: 0.18, filter: { type: "bandpass", freq: 2400 + Math.random() * 1200, q: 3 } });
    }
    s.noise({ at: 0.5, dur: 0.08, gain: 0.35, filter: { type: "lowpass", freq: 1800 } });
    s.tone({ freq: 900, to: 300, at: 0.5, dur: 0.07, gain: 0.2, type: "triangle" });
    s.noise({ at: 0.58, dur: 0.06, gain: 0.25, filter: { type: "lowpass", freq: 1500 } });
  },
  doubles(s) {
    s.ping(note(12), 0, 0.18, 0.18);
    s.ping(note(19), 0.09, 0.25, 0.18);
  },
  /** One token hop. */
  step(s) {
    s.tone({ freq: 700 + Math.random() * 80, to: 420, dur: 0.05, gain: 0.16, type: "triangle" });
    s.noise({ dur: 0.03, gain: 0.08, filter: { type: "highpass", freq: 3000 } });
  },
  /** Card-driven jump across the board. */
  teleport(s) {
    s.tone({ freq: 300, to: 1400, dur: 0.35, gain: 0.15 });
    s.noise({ dur: 0.35, gain: 0.06, filter: { type: "bandpass", freq: 3000, q: 0.7 } });
  },
  passGo(s) {
    s.ping(note(7), 0, 0.2, 0.2);
    s.ping(note(12), 0.12, 0.35, 0.22);
  },
  cashIn(s) {
    s.ping(2200, 0, 0.12, 0.14);
    s.ping(2960, 0.07, 0.22, 0.14);
  },
  cashOut(s) {
    s.tone({ freq: 180, to: 70, dur: 0.18, gain: 0.3 });
    s.noise({ dur: 0.05, gain: 0.08, filter: { type: "lowpass", freq: 600 } });
  },
  /** Paper flip. */
  card(s) {
    s.noise({ dur: 0.09, gain: 0.25, filter: { type: "bandpass", freq: 1800, q: 1.2 } });
    s.noise({ at: 0.11, dur: 0.05, gain: 0.12, filter: { type: "bandpass", freq: 2600, q: 1.5 } });
  },
  /** Deed stamped: thump + confirm ping. */
  buy(s) {
    s.noise({ dur: 0.07, gain: 0.3, filter: { type: "lowpass", freq: 700 } });
    s.tone({ freq: 140, to: 60, dur: 0.1, gain: 0.25 });
    s.ping(note(12), 0.1, 0.3, 0.18);
  },
  /** Auction gavel tap. */
  bid(s) {
    s.noise({ dur: 0.04, gain: 0.2, filter: { type: "lowpass", freq: 1200 } });
    s.tone({ freq: 320, to: 120, dur: 0.06, gain: 0.2, type: "triangle" });
  },
  build(s) {
    s.tone({ freq: 380, to: 820, dur: 0.07, gain: 0.22, type: "square" });
  },
  sell(s) {
    s.tone({ freq: 820, to: 380, dur: 0.08, gain: 0.18, type: "square" });
  },
  /** Cell door: inharmonic clang with a long tail. */
  jail(s) {
    s.tone({ freq: 220, dur: 0.7, gain: 0.18, type: "square" });
    s.tone({ freq: 583, dur: 0.6, gain: 0.12, type: "square" });
    s.tone({ freq: 1230, dur: 0.45, gain: 0.08, type: "square" });
    s.noise({ dur: 0.05, gain: 0.3, filter: { type: "highpass", freq: 2000 } });
  },
  /** Generic positive resolution — trade accepted, out of jail. */
  chime(s) {
    s.ping(note(4), 0, 0.2, 0.16);
    s.ping(note(9), 0.1, 0.3, 0.16);
  },
  /** Generic negative resolution — trade rejected, purchase declined. */
  deny(s) {
    s.tone({ freq: 260, dur: 0.12, gain: 0.15, type: "square" });
    s.tone({ freq: 200, at: 0.13, dur: 0.18, gain: 0.15, type: "square" });
  },
  trade(s) {
    s.ping(note(0), 0, 0.15, 0.14);
    s.ping(note(5), 0.08, 0.15, 0.14);
    s.ping(note(9), 0.16, 0.3, 0.14);
  },
  /** Debt: two-tone alarm. */
  alarm(s) {
    for (let i = 0; i < 2; i++) {
      s.tone({ freq: 660, at: i * 0.3, dur: 0.14, gain: 0.14, type: "sawtooth" });
      s.tone({ freq: 520, at: i * 0.3 + 0.15, dur: 0.14, gain: 0.14, type: "sawtooth" });
    }
  },
  bankrupt(s) {
    [7, 3, 0, -5].forEach((n, i) => s.tone({ freq: note(n), at: i * 0.16, dur: 0.35, gain: 0.16, type: "triangle" }));
  },
  yourTurn(s) {
    s.ping(note(16), 0, 0.25, 0.2);
    s.ping(note(12), 0.16, 0.4, 0.2);
  },
  start(s) {
    [0, 4, 7, 12].forEach((n, i) => s.ping(note(n), i * 0.1, 0.35, 0.18));
  },
  win(s) {
    [0, 4, 7, 12, 7, 12].forEach((n, i) => s.ping(note(n), i * 0.13, 0.4, 0.2, "triangle"));
    s.ping(note(16), 0.8, 1.2, 0.22, "triangle");
  },
  /** Someone else won. */
  end(s) {
    [7, 3, 0].forEach((n, i) => s.tone({ freq: note(n), at: i * 0.2, dur: 0.7, gain: 0.14, type: "triangle" }));
  },
  error(s) {
    s.tone({ freq: 150, dur: 0.12, gain: 0.15, type: "sawtooth" });
  },
} satisfies Record<string, (s: Synth) => void>;

export type CueName = keyof typeof cues;

export interface PlayOpts {
  /** Multiplier on the cue's own level (0..1). */
  gain?: number;
  /** Seconds from now. */
  delay?: number;
}

let ctx: AudioContext | null = null;
let master: GainNode | null = null;
let installed = false;

function ensure(): AudioContext | null {
  if (ctx) return ctx;
  if (typeof window === "undefined" || typeof AudioContext === "undefined") return null;
  ctx = new AudioContext();
  master = ctx.createGain();
  master.gain.value = useSettings.getState().volume;
  master.connect(ctx.destination);
  useSettings.subscribe((s) => {
    if (master && ctx) master.gain.setTargetAtTime(s.volume, ctx.currentTime, 0.02);
  });
  return ctx;
}

/**
 * Register the user-gesture listeners that let the context start. Safe to
 * call repeatedly; only the first call does anything.
 */
export function installSfx() {
  if (installed || typeof window === "undefined") return;
  installed = true;
  const resume = () => {
    const c = ensure();
    if (c && c.state === "suspended") void c.resume();
  };
  window.addEventListener("pointerdown", resume, { passive: true });
  window.addEventListener("keydown", resume, { passive: true });
}

/** Play a cue now (or after `delay`). No-op when muted or unsupported. */
export function play(name: CueName, { gain = 1, delay = 0 }: PlayOpts = {}) {
  if (!useSettings.getState().sfx) return;
  const c = ensure();
  if (!c || !master || c.state !== "running") return;
  const bus = c.createGain();
  bus.gain.value = gain;
  bus.connect(master);
  try {
    cues[name](new Synth(c, bus, c.currentTime + delay));
  } catch (e) {
    console.warn("sfx failed", name, e);
  }
}

export const sfx = { play, install: installSfx };
