import { describe, it, expect } from "vitest";
import { isListType, elementType, isSubtypeOf } from "../connectionRules";

describe("isListType", () => {
  it("detects list types", () => {
    expect(isListType("list<string>")).toBe(true);
    expect(isListType("list<path.file>")).toBe(true);
  });

  it("rejects non-list types", () => {
    expect(isListType("string")).toBe(false);
    expect(isListType("path.file")).toBe(false);
    expect(isListType("list")).toBe(false);
  });

  it("handles edge cases", () => {
    expect(isListType("")).toBe(false);
    expect(isListType("list<>")).toBe(true);
  });
});

describe("elementType", () => {
  it("extracts element from list type", () => {
    expect(elementType("list<string>")).toBe("string");
    expect(elementType("list<path.file>")).toBe("path.file");
  });

  it("returns null for non-list types", () => {
    expect(elementType("string")).toBeNull();
    expect(elementType("path.file")).toBeNull();
  });

  it("handles nested list syntax", () => {
    expect(elementType("list<list<string>>")).toBe("list<string>");
  });
});

describe("isSubtypeOf", () => {
  it("same type is subtype of itself", () => {
    expect(isSubtypeOf("string", "string")).toBe(true);
    expect(isSubtypeOf("path.file", "path.file")).toBe(true);
  });

  it("any is supertype of all types", () => {
    expect(isSubtypeOf("string", "any")).toBe(true);
    expect(isSubtypeOf("path.file", "any")).toBe(true);
  });

  it("detects dotted prefix hierarchy", () => {
    expect(isSubtypeOf("path.file", "path")).toBe(true);
    expect(isSubtypeOf("path.dir", "path")).toBe(true);
    expect(isSubtypeOf("path.file.txt", "path.file")).toBe(true);
  });

  it("respects prefix boundary", () => {
    // Dotted prefix check prevents path.file from being subtype of path.f
    expect(isSubtypeOf("path.file", "path.f")).toBe(false);
    expect(isSubtypeOf("pathed", "path")).toBe(false);
    // But path.f IS a subtype of path
    expect(isSubtypeOf("path.f", "path")).toBe(true);
  });

  it("handles list type subtypes", () => {
    expect(isSubtypeOf("list<path.file>", "list<path>")).toBe(true);
    expect(isSubtypeOf("list<path>", "list<any>")).toBe(true);
  });

  it("list type rules apply independently", () => {
    expect(isSubtypeOf("list<path.file>", "path")).toBe(false);
    expect(isSubtypeOf("path.file", "list<path>")).toBe(false);
  });
});
