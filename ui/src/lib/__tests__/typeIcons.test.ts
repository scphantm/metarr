import { describe, it, expect } from "vitest";
import {
  iconClassForType,
  iconClassForControlPort,
  ITERATE_ICON_CLASS,
  TYPE_UNSAFE_ICON_CLASS,
  RECURSIVE_ICON_CLASS,
} from "../typeIcons";

describe("iconClassForType", () => {
  it("maps path.dir to icon-directory", () => {
    expect(iconClassForType("path.dir")).toBe("icon-directory");
  });

  it("maps path.file to icon-file", () => {
    expect(iconClassForType("path.file")).toBe("icon-file");
  });

  it("maps generic path to icon-tree", () => {
    expect(iconClassForType("path")).toBe("icon-tree");
  });

  it("maps media to icon-media", () => {
    expect(iconClassForType("media")).toBe("icon-media");
  });

  it("returns undefined for unknown types", () => {
    expect(iconClassForType("string")).toBeUndefined();
    expect(iconClassForType("number")).toBeUndefined();
  });

  it("handles list<path.dir>", () => {
    expect(iconClassForType("list<path.dir>")).toBe("icon-list-directory");
  });

  it("handles list<path.file>", () => {
    expect(iconClassForType("list<path.file>")).toBe("icon-list-file");
  });

  it("handles list<path>", () => {
    expect(iconClassForType("list<path>")).toBe("icon-list");
  });

  it("returns undefined for list<unknown>", () => {
    expect(iconClassForType("list<string>")).toBeUndefined();
  });

  it("respects prefix specificity", () => {
    expect(iconClassForType("path.dir.subdir")).toBe("icon-directory");
    expect(iconClassForType("path.file.txt")).toBe("icon-file");
  });
});

describe("iconClassForControlPort", () => {
  it("maps standard control ports", () => {
    expect(iconClassForControlPort("in")).toBe("icon-control-in");
    expect(iconClassForControlPort("next")).toBe("icon-control-next");
    expect(iconClassForControlPort("error")).toBe("icon-control-error");
  });

  it("maps branch ports", () => {
    expect(iconClassForControlPort("yes")).toBe("icon-control-yes");
    expect(iconClassForControlPort("no")).toBe("icon-control-no");
  });

  it("returns undefined for unlisted ports", () => {
    expect(iconClassForControlPort("branch1")).toBeUndefined();
    expect(iconClassForControlPort("body")).toBeUndefined();
  });
});

describe("icon class constants", () => {
  it("defines expected icon classes", () => {
    expect(ITERATE_ICON_CLASS).toBe("icon-iterate");
    expect(TYPE_UNSAFE_ICON_CLASS).toBe("icon-type-unsafe");
    expect(RECURSIVE_ICON_CLASS).toBe("icon-recursive");
  });
});
