// Brand asset generator — the single source of truth for the Monopsony logo,
// favicon, mascot ("Mono", an octopus: one buyer, eight arms) and the board
// centre emblem. Run `npm run brand -w @monopsony/web` after editing; it
// rewrites the SVGs under apps/web/public and apps/admin/public and rasterises
// the PNG icons with the Playwright Chromium the e2e suite already installs.
//
// Everything is original art. Deliberately avoided: top hats, moustaches, a red
// banner wordmark, red/green house-and-hotel iconography.

import { mkdirSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const webPublic = join(here, "..", "public");
const adminPublic = join(here, "..", "..", "admin", "public");

export const palette = {
  emerald: "#10b981",
  emeraldLight: "#34d399",
  emeraldDark: "#059669",
  gold: "#f5c451",
  goldDark: "#c9962e",
  navy: "#0e3a4a",
  navyDeep: "#082430",
  ink: "#0f172a",
  sand: "#f1e9d2",
  white: "#ffffff",
};

const FONT = "'Segoe UI', Inter, Roboto, 'Helvetica Neue', Arial, sans-serif";

// ---- Logo mark: octagon coin + M -------------------------------------------------
// Regular octagon, circumradius 30 about (32,32), flat sides top/bottom/left/right.
const OCTAGON = "59.72,20.52 43.48,4.28 20.52,4.28 4.28,20.52 4.28,43.48 20.52,59.72 43.48,59.72 59.72,43.48";
// Heavy M whose centre vertex dips like an arrow: everything flows to the one buyer.
const M_PATH = "M16 46V20h7l9 13 9-13h7v26h-7V32l-9 12.5L23 32v14z";

/** The mark in a 64x64 box. `variant` picks the colourway. */
export function mark({ variant = "brand" } = {}) {
  const v = {
    brand: { fill: palette.emerald, rim: palette.gold, letter: palette.white, shade: palette.emeraldDark },
    admin: { fill: "#27272a", rim: palette.gold, letter: palette.emeraldLight, shade: "#18181b" },
    mono: { fill: "currentColor", rim: "none", letter: "#fff", shade: "currentColor" },
  }[variant];
  return `<g>
  <polygon points="${OCTAGON}" fill="${v.fill}" stroke="${v.rim}" stroke-width="3" stroke-linejoin="round"/>
  <polygon points="${OCTAGON}" fill="none" stroke="${v.shade}" stroke-width="1.5" stroke-linejoin="round" transform="translate(32 32) scale(0.82) translate(-32 -32)" opacity="0.6"/>
  <path d="${M_PATH}" fill="${v.letter}"/>
</g>`;
}

function svg(w, h, body, extra = "") {
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${w} ${h}" width="${w}" height="${h}"${extra}>\n${body}\n</svg>\n`;
}

// ---- Mascot: Mono the octopus -------------------------------------------------------
// 200x200 box. Front arms are drawn over the head, back arms behind it. The far
// right arm presents the coin; a captain's cap ties Mono to the Harbourside board.
export function mascot({ coin = true } = {}) {
  const p = palette;
  const backArm = (d) => `<path d="${d}" fill="none" stroke="${p.emeraldDark}" stroke-width="12" stroke-linecap="round"/>`;
  const frontArm = (d) => `<path d="${d}" fill="none" stroke="${p.emerald}" stroke-width="13" stroke-linecap="round"/>`;
  const sucker = (x, y) => `<circle cx="${x}" cy="${y}" r="2.4" fill="#a7f3d0" opacity="0.9"/>`;
  return `<g>
  ${backArm("M70 118C55 140 45 160 58 178")}
  ${backArm("M92 126C90 150 88 165 96 185")}
  ${backArm("M108 126C110 150 112 165 104 185")}
  ${backArm("M130 118C145 140 155 160 142 178")}
  <!-- head -->
  <path d="M45 100C45 45 155 45 155 100C155 120 140 132 100 132C60 132 45 120 45 100Z" fill="${p.emerald}"/>
  <ellipse cx="78" cy="66" rx="16" ry="9" transform="rotate(-25 78 66)" fill="${p.emeraldLight}" opacity="0.55"/>
  <!-- front arms -->
  ${frontArm("M62 120C45 130 22 145 30 170C34 184 52 184 50 172")}
  ${frontArm("M80 128C72 145 60 160 68 182C72 192 86 190 84 180")}
  ${frontArm("M120 128C128 145 140 160 132 182C128 192 114 190 116 180")}
  ${frontArm(coin ? "M140 116C162 122 186 110 180 84" : "M138 120C155 130 178 145 170 170C166 184 148 184 150 172")}
  ${sucker(40, 150)}${sucker(34, 162)}${sucker(70, 150)}${sucker(65, 165)}${sucker(130, 150)}${sucker(135, 165)}
  ${coin ? sucker(160, 118) + sucker(176, 106) : sucker(160, 150) + sucker(166, 162)}
  <!-- face -->
  <circle cx="80" cy="86" r="13" fill="#fff"/>
  <circle cx="120" cy="86" r="13" fill="#fff"/>
  <circle cx="84" cy="88" r="6" fill="${p.ink}"/>
  <circle cx="124" cy="88" r="6" fill="${p.ink}"/>
  <circle cx="86" cy="85" r="2" fill="#fff"/>
  <circle cx="126" cy="85" r="2" fill="#fff"/>
  <path d="M90 108C96 116 104 116 110 108" fill="none" stroke="${p.ink}" stroke-width="3" stroke-linecap="round"/>
  <circle cx="64" cy="104" r="6" fill="${p.gold}" opacity="0.35"/>
  <circle cx="136" cy="104" r="6" fill="${p.gold}" opacity="0.35"/>
  <!-- captain's cap -->
  <path d="M84 56C92 34 136 30 150 50L154 62C132 54 104 56 86 66Z" fill="${p.navy}"/>
  <path d="M84 66C104 56 134 54 156 62C150 70 122 66 88 72Z" fill="${p.ink}"/>
  <path d="M90 62C108 55 132 54 152 60" fill="none" stroke="${p.gold}" stroke-width="3" stroke-linecap="round"/>
  <circle cx="118" cy="47" r="4" fill="${p.gold}"/>
  ${coin ? `<g transform="translate(160 56) scale(0.75)">${mark()}</g>` : ""}
</g>`;
}

// ---- Board centre emblem --------------------------------------------------------------
// 1024x1024, transparent outside the disc so it sits on any board skin's centre panel.
function boardCentre() {
  const p = palette;
  const waves = [560, 620, 680, 740, 800, 860]
    .map((y, i) => `<path d="M0 ${y}Q64 ${y - 22} 128 ${y}T256 ${y}T384 ${y}T512 ${y}T640 ${y}T768 ${y}T896 ${y}T1024 ${y}" fill="none" stroke="${p.emeraldLight}" stroke-width="4" opacity="${0.12 + i * 0.03}"/>`)
    .join("\n    ");
  const compass = [0, 45, 90, 135, 180, 225, 270, 315]
    .map((a) => `<path d="M512 90L528 512L496 512Z" fill="${p.gold}" transform="rotate(${a} 512 512)" opacity="${a % 90 === 0 ? 0.28 : 0.16}"/>`)
    .join("\n    ");
  return svg(
    1024,
    1024,
    `<defs>
    <clipPath id="disc"><circle cx="512" cy="512" r="452"/></clipPath>
    <path id="ring" d="M104 512A408 408 0 0 1 920 512"/>
  </defs>
  <circle cx="512" cy="512" r="470" fill="${p.navy}"/>
  <circle cx="512" cy="512" r="470" fill="none" stroke="${p.gold}" stroke-width="10"/>
  <circle cx="512" cy="512" r="452" fill="none" stroke="${p.gold}" stroke-width="3" opacity="0.6"/>
  <g clip-path="url(#disc)">
    ${compass}
    ${waves}
    <g transform="translate(512 1000) scale(2.6) translate(-100 -160)">${mascot()}</g>
  </g>
  <text font-family="${FONT}" font-weight="700" font-size="34" letter-spacing="8" fill="${p.gold}" opacity="0.9" text-anchor="middle">
    <textPath href="#ring" startOffset="50%">ONE BUYER · EIGHT ARMS</textPath>
  </text>
  <g transform="translate(512 300) scale(3.6) translate(-32 -32)">${mark()}</g>
  <text x="512" y="520" text-anchor="middle" font-family="${FONT}" font-weight="900" font-size="112" letter-spacing="-2" fill="${p.sand}">MONOPSONY</text>
  <text x="512" y="576" text-anchor="middle" font-family="${FONT}" font-weight="700" font-size="34" letter-spacing="14" fill="${p.emeraldLight}">HARBOURSIDE</text>`,
  );
}

// ---- Lockups ----------------------------------------------------------------------------
function logoLockup({ dark = true } = {}) {
  const text = dark ? palette.sand : palette.ink;
  return svg(
    420,
    96,
    `<g transform="translate(16 16)">${mark()}</g>
  <text x="96" y="66" font-family="${FONT}" font-weight="900" font-size="48" letter-spacing="-1.5" fill="${text}">MONOPSONY</text>`,
  );
}

function ogImage() {
  const p = palette;
  return svg(
    1200,
    630,
    `<rect width="1200" height="630" fill="${p.ink}"/>
  <circle cx="1040" cy="520" r="520" fill="${p.navy}" opacity="0.7"/>
  <g transform="translate(60 160) scale(2.8)">${mark()}</g>
  <text x="256" y="266" font-family="${FONT}" font-weight="900" font-size="92" letter-spacing="-3" fill="${p.sand}">MONOPSONY</text>
  <text x="260" y="330" font-family="${FONT}" font-weight="600" font-size="36" fill="${p.emeraldLight}">One buyer. Eight arms.</text>
  <text x="260" y="384" font-family="${FONT}" font-size="30" fill="#94a3b8">Buy, build, bankrupt your friends — in 3D.</text>
  <g transform="translate(800 210) scale(1.85)">${mascot()}</g>`,
  );
}

// ---- Emit --------------------------------------------------------------------------------
const files = {
  [join(webPublic, "favicon.svg")]: svg(64, 64, mark()),
  [join(webPublic, "brand", "logo-mark.svg")]: svg(64, 64, mark()),
  [join(webPublic, "brand", "logo.svg")]: logoLockup(),
  [join(webPublic, "brand", "logo-light.svg")]: logoLockup({ dark: false }),
  [join(webPublic, "brand", "mono.svg")]: svg(200, 200, mascot()),
  [join(webPublic, "brand", "mono-plain.svg")]: svg(200, 200, mascot({ coin: false })),
  [join(webPublic, "brand", "board-centre.svg")]: boardCentre(),
  [join(webPublic, "brand", "og.svg")]: ogImage(),
  [join(adminPublic, "favicon.svg")]: svg(64, 64, mark({ variant: "admin" })),
};
for (const [file, body] of Object.entries(files)) {
  mkdirSync(dirname(file), { recursive: true });
  writeFileSync(file, body);
  console.log("wrote", file);
}

// PNG rasters: touch icon, PWA icons, social card. Chromium renders the SVG
// text with the same font stack the app uses.
const rasters = [
  { svg: files[join(webPublic, "favicon.svg")], out: join(webPublic, "apple-touch-icon.png"), w: 180, h: 180, pad: 0.1, bg: palette.ink },
  { svg: files[join(webPublic, "favicon.svg")], out: join(webPublic, "icon-192.png"), w: 192, h: 192, pad: 0.1, bg: palette.ink },
  { svg: files[join(webPublic, "favicon.svg")], out: join(webPublic, "icon-512.png"), w: 512, h: 512, pad: 0.1, bg: palette.ink },
  { svg: files[join(webPublic, "brand", "og.svg")], out: join(webPublic, "brand", "og.png"), w: 1200, h: 630 },
  { svg: files[join(webPublic, "brand", "mono.svg")], out: join(webPublic, "brand", "mono.png"), w: 400, h: 400 },
  { svg: files[join(webPublic, "brand", "board-centre.svg")], out: join(webPublic, "brand", "board-centre.png"), w: 1024, h: 1024 },
];

if (!process.argv.includes("--svg-only")) {
  const { chromium } = await import("playwright");
  const browser = await chromium.launch();
  const page = await browser.newPage({ viewport: { width: 1200, height: 1200 }, deviceScaleFactor: 1 });
  for (const r of rasters) {
    const pad = Math.round((r.pad ?? 0) * r.w);
    const bg = r.bg ?? "transparent";
    await page.setViewportSize({ width: r.w, height: r.h });
    await page.setContent(
      `<html><body style="margin:0;background:${bg};width:${r.w}px;height:${r.h}px;display:grid;place-items:center">` +
        `<div style="width:${r.w - pad * 2}px;height:${r.h - pad * 2}px">${r.svg.replace(/width="\d+" height="\d+"/, 'width="100%" height="100%"')}</div></body></html>`,
    );
    await page.screenshot({ path: r.out, omitBackground: bg === "transparent", clip: { x: 0, y: 0, width: r.w, height: r.h } });
    console.log("rasterised", r.out);
  }
  await browser.close();
}
