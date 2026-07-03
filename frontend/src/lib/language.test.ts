// @vitest-environment node
import { describe, expect, it } from "vitest";
import { languageFor } from "./language";

describe("languageFor", () => {
  it("maps obvious code extensions", () => {
    expect(languageFor("foo.ts")).toBe("typescript");
    expect(languageFor("foo.tsx")).toBe("typescript");
    expect(languageFor("foo.js")).toBe("javascript");
    expect(languageFor("foo.go")).toBe("go");
    expect(languageFor("foo.py")).toBe("python");
    expect(languageFor("foo.rs")).toBe("rust");
    expect(languageFor("foo.sh")).toBe("shell");
    expect(languageFor("foo.bash")).toBe("shell");
    expect(languageFor("foo.yaml")).toBe("yaml");
    expect(languageFor("foo.yml")).toBe("yaml");
    expect(languageFor("foo.json")).toBe("json");
    expect(languageFor("foo.toml")).toBe("ini");
    expect(languageFor("foo.md")).toBe("markdown");
    expect(languageFor("foo.sql")).toBe("sql");
    expect(languageFor("foo.css")).toBe("css");
    expect(languageFor("foo.html")).toBe("html");
    expect(languageFor("foo.xml")).toBe("xml");
  });

  it("preserves the full directory path for matching", () => {
    expect(languageFor("src/kernel/agent/agent.go")).toBe("go");
    expect(languageFor("frontend/src/views/Chat.tsx")).toBe("typescript");
    expect(languageFor("a/b/c.yaml")).toBe("yaml");
  });

  it("recognises filename shortcuts (Dockerfile, Makefile, dotfiles)", () => {
    expect(languageFor("Dockerfile")).toBe("dockerfile");
    expect(languageFor("docker/Dockerfile")).toBe("dockerfile");
    expect(languageFor("Makefile")).toBe("makefile");
    expect(languageFor(".bashrc")).toBe("shell");
    expect(languageFor("home/.zshrc")).toBe("shell");
  });

  it("falls back to plaintext for unknown extensions and empty input", () => {
    expect(languageFor("")).toBe("plaintext");
    expect(languageFor("photo.jpg")).toBe("plaintext");
    expect(languageFor("archive.zip")).toBe("plaintext");
    expect(languageFor("untitled")).toBe("plaintext");
  });

  it("is case-insensitive on the extension", () => {
    expect(languageFor("FOO.GO")).toBe("go");
    expect(languageFor("Foo.YAML")).toBe("yaml");
  });
});
