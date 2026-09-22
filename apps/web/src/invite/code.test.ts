import { describe, expect, it } from "vitest";
import { afterSignInPath, inviteUrl, normalizeInviteCode } from "./code";

describe("normalizeInviteCode", () => {
  it("canonicalises whatever a player pastes", () => {
    expect(normalizeInviteCode("ABCD-2345")).toBe("ABCD-2345");
    expect(normalizeInviteCode("abcd2345")).toBe("ABCD-2345");
    expect(normalizeInviteCode("  abcd 2345 ")).toBe("ABCD-2345");
    expect(normalizeInviteCode("https://monopsony.game/join/abcd-2345")).toBe("ABCD-2345");
  });

  it("keeps a partial code usable while it is being typed", () => {
    expect(normalizeInviteCode("ab")).toBe("AB");
    expect(normalizeInviteCode("abcd")).toBe("ABCD");
    expect(normalizeInviteCode("abcde")).toBe("ABCD-E");
  });

  it("drops anything that is not a base32 character", () => {
    expect(normalizeInviteCode("a!b@c#d$2345")).toBe("ABCD-2345");
    expect(normalizeInviteCode("ABCD-2345-EXTRA")).toBe("ABCD-2345"); // codes are eight characters
    expect(normalizeInviteCode("https://monopsony.game/lobby")).toBe(""); // a link, but not an invite
    expect(normalizeInviteCode("https://monopsony.game/join/abcd-2345?from=chat")).toBe("ABCD-2345");
  });
});

describe("inviteUrl", () => {
  it("shares a private table by its code", () => {
    expect(inviteUrl({ gameId: "g1", inviteCode: "ABCD-2345" }, "https://monopsony.game")).toBe(
      "https://monopsony.game/join/ABCD-2345",
    );
  });

  it("shares a public table by its room, which anyone may join", () => {
    expect(inviteUrl({ gameId: "g1" }, "https://monopsony.game")).toBe("https://monopsony.game/game/g1");
  });
});

describe("afterSignInPath", () => {
  it("returns to the page that asked for a sign-in", () => {
    expect(afterSignInPath("/join/ABCD-2345")).toBe("/join/ABCD-2345");
  });

  it("falls back to the lobby", () => {
    expect(afterSignInPath()).toBe("/lobby");
    expect(afterSignInPath("/")).toBe("/lobby");
  });
});
