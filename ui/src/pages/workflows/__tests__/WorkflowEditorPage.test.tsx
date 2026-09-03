import { describe, it, expect, vi, beforeEach } from "vitest";
import { createElement, type ReactNode } from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const useWorkflow = vi.fn();
const useWorkflowVersions = vi.fn();
const useWorkflowVersion = vi.fn();
const useCreateWorkflow = vi.fn();
const useUpdateWorkflow = vi.fn();
const createMutateAsync = vi.fn();
const updateMutateAsync = vi.fn();

vi.mock("../../../api/queries", () => ({
  queryKeys: { workflow: (id: string) => ["workflows", id] },
  useWorkflow: () => useWorkflow(),
  useWorkflowVersions: () => useWorkflowVersions(),
  useWorkflowVersion: () => useWorkflowVersion(),
  useCreateWorkflow: () => useCreateWorkflow(),
  useUpdateWorkflow: () => useUpdateWorkflow(),
}));

// The canvas and palette drag ReactFlow in; the editor only needs the
// imperative instance the canvas hands back through onInit.
vi.mock("../WorkflowCanvas", () => ({
  WorkflowCanvas: ({ onInit }: { onInit: (instance: unknown) => void }) => {
    onInit({
      getNodes: () => [],
      getEdges: () => [],
      getViewport: () => ({ x: 0, y: 0, zoom: 1 }),
    });
    return null;
  },
}));
vi.mock("../NodePalette", () => ({ NodePalette: () => null }));
vi.mock("../TagsInput", () => ({
  TagsInput: ({
    value,
    onChange,
  }: {
    value: string[];
    onChange: (next: string[]) => void;
  }) =>
    createElement(
      "button",
      { type: "button", onClick: () => onChange([...value, "t"]) },
      "add-tag",
    ),
}));
vi.mock("../graphAdapter", () => ({
  fromRFGraph: () => ({ schemaVersion: 1, nodes: [], edges: [], viewport: {} }),
  toRFGraph: () => ({ nodes: [], edges: [] }),
}));

import { WorkflowEditorPage } from "../WorkflowEditorPage";

function renderAt(path: string) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const wrapper = ({ children }: { children: ReactNode }) =>
    createElement(QueryClientProvider, { client: queryClient }, children);
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route path="/workflows/add" element={<WorkflowEditorPage />} />
        <Route path="/workflows/:id/edit" element={<WorkflowEditorPage />} />
      </Routes>
    </MemoryRouter>,
    { wrapper },
  );
}

function fillRequiredFields() {
  fireEvent.change(screen.getByPlaceholderText("Workflow name"), {
    target: { value: "My flow" },
  });
  fireEvent.change(screen.getByPlaceholderText("Description"), {
    target: { value: "does things" },
  });
}

describe("WorkflowEditorPage save branch", () => {
  beforeEach(() => {
    useWorkflow.mockReset();
    useWorkflowVersions.mockReset().mockReturnValue({ data: [] });
    useWorkflowVersion.mockReset().mockReturnValue({ data: undefined });
    useCreateWorkflow
      .mockReset()
      .mockReturnValue({ mutateAsync: createMutateAsync });
    useUpdateWorkflow
      .mockReset()
      .mockReturnValue({ mutateAsync: updateMutateAsync });
    createMutateAsync
      .mockReset()
      .mockResolvedValue({ id: "new-id", version: 1 });
    updateMutateAsync.mockReset().mockResolvedValue({ id: "wf-1", version: 2 });
  });

  it("calls create (no id) when the route has no workflow id", async () => {
    useWorkflow.mockReturnValue({ data: undefined });

    renderAt("/workflows/add");
    fillRequiredFields();
    // Tags are required for canSave; the editor seeds none, so add one.
    fireEvent.click(screen.getByRole("button", { name: "add-tag" }));

    fireEvent.click(screen.getByRole("button", { name: "Save Workflow" }));

    await waitFor(() => expect(createMutateAsync).toHaveBeenCalledTimes(1));
    expect(updateMutateAsync).not.toHaveBeenCalled();
    expect(createMutateAsync.mock.calls[0][0]).not.toHaveProperty("id");
  });

  it("calls update with the id when the route carries a workflow id", async () => {
    useWorkflow.mockReturnValue({
      data: {
        id: "wf-1",
        version: 1,
        name: "Existing",
        description: "d",
        tags: ["t"],
        graph: { schemaVersion: 1, nodes: [], edges: [], viewport: {} },
      },
    });

    renderAt("/workflows/wf-1/edit");

    fireEvent.click(
      await screen.findByRole("button", { name: "Save Workflow" }),
    );

    await waitFor(() => expect(updateMutateAsync).toHaveBeenCalledTimes(1));
    expect(createMutateAsync).not.toHaveBeenCalled();
    expect(updateMutateAsync.mock.calls[0][0]).toMatchObject({ id: "wf-1" });
  });
});
