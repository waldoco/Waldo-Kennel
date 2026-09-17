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
