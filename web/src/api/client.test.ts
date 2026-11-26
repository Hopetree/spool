import { describe, expect, it } from "vitest";
import { ApiError, parseApiError } from "./client";

describe("parseApiError", () => {
  it("parses structured error body", async () => {
    const res = new Response(
      JSON.stringify({ code: "invalid_credentials", message: "用户名或密码错误" }),
      { status: 401, headers: { "Content-Type": "application/json" } },
    );
    const err = await parseApiError(res);
    expect(err).toBeInstanceOf(ApiError);
    expect(err.status).toBe(401);
    expect(err.code).toBe("invalid_credentials");
    expect(err.message).toBe("用户名或密码错误");
  });

  it("falls back for non-json body", async () => {
    const res = new Response("oops", { status: 502 });
    const err = await parseApiError(res);
    expect(err.code).toBe("http_502");
    expect(err.message).toContain("502");
  });
});
