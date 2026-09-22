import { describe, expect, it } from "vitest";
import { qrMatrix } from "./qr";
import fixtures from "./qr.fixtures.json";

/**
 * The fixtures are reference symbols at error-correction level M in byte mode,
 * built from two independent implementations: python-qrcode places the
 * modules, segno's spec-faithful penalty scoring picks the mask. A matching
 * matrix means the data encoding, the Reed–Solomon block interleave, the
 * module placement and the mask choice all agree, down to the module.
 * Regenerate with `python scripts/qr-fixtures.py`, which explains why it
 * takes one thing from each library.
 */
const render = (matrix: boolean[][]) => matrix.map((row) => row.map((m) => (m ? "#" : ".")).join(""));

describe("qrMatrix", () => {
  for (const fixture of fixtures) {
    const label = fixture.text.length > 32 ? `${fixture.text.slice(0, 32)}… (${fixture.text.length} chars)` : fixture.text;
    it(`matches the reference symbol for ${label}`, () => {
      expect(render(qrMatrix(fixture.text))).toEqual(fixture.rows);
    });
  }

  it("grows the symbol with the payload", () => {
    expect(qrMatrix("a").length).toBe(21); // version 1
    expect(qrMatrix("x".repeat(200)).length).toBe(57); // version 10
  });

  it("refuses a payload past the largest supported version", () => {
    expect(() => qrMatrix("x".repeat(700))).toThrow(/too long/);
  });
});
