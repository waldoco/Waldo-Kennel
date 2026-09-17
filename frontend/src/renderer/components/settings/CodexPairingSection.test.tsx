import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { CodexPairingSection } from "./CodexPairingSection";
afterEach(() => {
  delete window.kennel;
});
describe("Codex project pairing", () => {
  it("discovers inside a project and requires native pairing confirmation", async () => {
    const pairCodex = vi.fn(async () => ({
      state: "connected" as const,
      connectionId: "c",
      generation: 1,
    }));
    window.kennel = {
      app: {
        discoverCodex: vi.fn(async () => ({
          state: "installed" as const,
          installationId: "i",
          version: "1.2.3",
          source: "path",
        })),
        getCodexPairing: vi.fn(async () => ({ state: "unpaired" as const })),
        pairCodex,
      } as never,
    } as never;
    render(<CodexPairingSection projectId="project-1" />);
    await screen.findByText(/Codex 1.2.3/);
    fireEvent.click(screen.getByRole("button", { name: "Pair Codex" }));
    await waitFor(() =>
      expect(pairCodex).toHaveBeenCalledWith({
        projectId: "project-1",
        requestKey: expect.any(String),
      }),
    );
    await screen.findByText(/Connected/);
  });
  it.each([
    ["not_found", "Codex was not found"],
    ["incompatible", "This Codex version is incompatible"],
  ] as const)("shows %s truthfully", async (state, text) => {
    window.kennel = {
      app: {
        discoverCodex: vi.fn(async () => ({ state, message: text })),
        getCodexPairing: vi.fn(async () => ({ state: "unpaired" })),
      } as never,
    } as never;
    render(<CodexPairingSection projectId="p" />);
    await screen.findByText(text);
    expect(screen.getByRole("button")).toBeDisabled();
  });
  it("never renders secret-shaped main-process fields", async () => {
    window.kennel = {
      app: {
        discoverCodex: vi.fn(async () => ({
          state: "installed",
          installationId: "i",
          version: "1",
          source: "path",
        })),
        getCodexPairing: vi.fn(async () => ({
          state: "error",
          message: "Pairing failed. Your previous connection was not changed.",
          secret: "PRIVATE_SECRET",
          bearer: "PRIVATE_BEARER",
        })),
      } as never,
    } as never;
    const { container } = render(<CodexPairingSection projectId="p" />);
    await screen.findByText(/previous connection/);
    expect(container.textContent).not.toContain("PRIVATE_");
  });
});

it("drops stale project results and refreshes an error without repeating approval", async () => {
  let releaseOld!: (value: { state: "unpaired" }) => void;
  const oldPairing = new Promise<{ state: "unpaired" }>((resolve) => {
    releaseOld = resolve;
  });
  const discoverCodex = vi.fn(async ({ projectId }: { projectId: string }) => ({
    state: "installed" as const,
    installationId: projectId,
    version: projectId === "old" ? "1.0.0" : "2.0.0",
    source: "path",
  }));
  const getCodexPairing = vi.fn(({ projectId }: { projectId: string }) =>
    projectId === "old"
      ? oldPairing
      : Promise.reject(new Error("daemon offline")),
  );
  window.kennel = {
    app: { discoverCodex, getCodexPairing, pairCodex: vi.fn() } as never,
  } as never;
  const { rerender } = render(<CodexPairingSection projectId="old" />);
  rerender(<CodexPairingSection projectId="new" />);
  await screen.findByText("Pairing state could not be loaded.");
  releaseOld({ state: "unpaired" });
  await new Promise((resolve) => setTimeout(resolve, 0));
  expect(screen.queryByText(/Codex 1.0.0/)).not.toBeInTheDocument();
  expect(
    screen.getByRole("button", { name: "Refresh status" }),
  ).toBeInTheDocument();
});

it("names canonical reconnect and incomplete activation repairs", async () => {
  const getCodexPairing = vi
    .fn()
    .mockResolvedValueOnce({
      state: "action_needed",
      repair: "reconnect_adapter",
      message: "Reconnect Codex for this app session.",
    })
    .mockResolvedValueOnce({
      state: "action_needed",
      repair: "retry_pairing",
      message: "Pairing was approved but did not complete.",
    });
  window.kennel = {
    app: {
      discoverCodex: vi.fn(async () => ({
        state: "installed",
        installationId: "i",
        version: "1",
        source: "path",
      })),
      getCodexPairing,
      pairCodex: vi.fn(),
    } as never,
  } as never;
  const { rerender } = render(<CodexPairingSection projectId="p1" />);
  await screen.findByRole("button", { name: "Reconnect" });
  rerender(<CodexPairingSection projectId="p2" />);
  await screen.findByRole("button", { name: "Retry pairing" });
});

it("does not project a late native-confirmed pair result onto another Project", async () => {
  let resolvePair!: (value: {
    state: "connected";
    connectionId: string;
    generation: number;
  }) => void;
  const pairCodex = vi.fn(
    () =>
      new Promise<{
        state: "connected";
        connectionId: string;
        generation: number;
      }>((resolve) => {
        resolvePair = resolve;
      }),
  );
  window.kennel = {
    app: {
      discoverCodex: vi.fn(async ({ projectId }: { projectId: string }) => ({
        state: "installed",
        installationId: projectId,
        version: projectId,
        source: "path",
      })),
      getCodexPairing: vi.fn(async () => ({ state: "unpaired" })),
      pairCodex,
    } as never,
  } as never;
  const { rerender } = render(<CodexPairingSection projectId="project-a" />);
  await screen.findByText(/Codex project-a/);
  fireEvent.click(screen.getByRole("button", { name: "Pair Codex" }));
  await waitFor(() => expect(pairCodex).toHaveBeenCalled());
  rerender(<CodexPairingSection projectId="project-b" />);
  await screen.findByText(/Codex project-b/);
  resolvePair({ state: "connected", connectionId: "a", generation: 7 });
  await new Promise((resolve) => setTimeout(resolve, 0));
  expect(
    screen.queryByText(/Connected · generation 7/),
  ).not.toBeInTheDocument();
  expect(
    screen.getByRole("button", { name: "Pair Codex" }),
  ).not.toHaveAttribute("aria-busy", "true");
});

it("does not project a late manual Refresh result onto another Project", async () => {
  let resolveRefresh!: (value: { state: "unpaired" }) => void;
  const getCodexPairing = vi
    .fn()
    .mockRejectedValueOnce(new Error("offline"))
    .mockImplementationOnce(
      () =>
        new Promise<{ state: "unpaired" }>((resolve) => {
          resolveRefresh = resolve;
        }),
    )
    .mockResolvedValue({ state: "unpaired" });
  window.kennel = {
    app: {
      discoverCodex: vi.fn(async ({ projectId }: { projectId: string }) => ({
        state: "installed",
        installationId: projectId,
        version: projectId,
        source: "path",
      })),
      getCodexPairing,
      pairCodex: vi.fn(),
    } as never,
  } as never;
  const { rerender } = render(<CodexPairingSection projectId="project-a" />);
  await screen.findByRole("button", { name: "Refresh status" });
  fireEvent.click(screen.getByRole("button", { name: "Refresh status" }));
  rerender(<CodexPairingSection projectId="project-b" />);
  await screen.findByText(/Codex project-b/);
  resolveRefresh({ state: "unpaired" });
  await new Promise((resolve) => setTimeout(resolve, 0));
  expect(screen.getByText(/Codex project-b/)).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Pair Codex" })).toBeEnabled();
});
