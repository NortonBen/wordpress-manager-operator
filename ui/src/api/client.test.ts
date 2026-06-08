import { describe, it, expect, beforeEach } from "vitest";
import { getToken, setToken, clearToken } from "./client";

describe("token storage", () => {
  beforeEach(() => clearToken());

  it("round-trips the JWT through localStorage", () => {
    expect(getToken()).toBeNull();
    setToken("jwt-abc");
    expect(getToken()).toBe("jwt-abc");
    clearToken();
    expect(getToken()).toBeNull();
  });
});
