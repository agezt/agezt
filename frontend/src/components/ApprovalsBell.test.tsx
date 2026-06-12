// @vitest-environment jsdom
import { describe, it, expect } from "vitest";
import { approvalLabel } from "@/components/ApprovalsBell";

describe("approvalLabel (M913)", () => {
  it("joins capability and reason, falling back through the available fields", () => {
    expect(approvalLabel({ capability: "shell.exec", reason: "install deps" })).toBe("shell.exec — install deps");
    expect(approvalLabel({ capability: "net.fetch" })).toBe("net.fetch");
    expect(approvalLabel({ tool_name: "browser" })).toBe("browser");
    expect(approvalLabel({})).toBe("capability");
    // A blank/whitespace reason is not appended.
    expect(approvalLabel({ capability: "x", reason: "  " })).toBe("x");
  });
});
