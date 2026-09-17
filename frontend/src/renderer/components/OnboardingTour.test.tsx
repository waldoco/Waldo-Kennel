import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useSettings, useUpdateReasoning } from "../hooks/useSettings";
import { aoBridge } from "../lib/bridge";
import { useUiStore } from "../stores/ui-store";
import { OnboardingTour } from "./OnboardingTour";

const ctx = vi.hoisted(() => ({
	agents: {
		authorized: [{ id: "codex", label: "Codex" }],
		installed: [
			{ id: "codex", label: "Codex" },
			{ id: "claude-code", label: "Claude Code" },
		],
		supported: [],
	} as {
		authorized: { id: string; label: string }[];
		installed: { id: string; label: string }[];
		supported: { id: string; label: string }[];
	},
	isPending: false,
	installTmux: vi.fn(),
	updateReasoning: vi.fn(),
}));

vi.mock("../hooks/useAgentsQuery", () => ({
	refreshAgentsIfStale: vi.fn(async () => undefined),
	useAgentsQuery: () => ({
		data: ctx.agents,
		isPending: ctx.isPending,
		isError: false,
		refetch: vi.fn(),
	}),
}));

vi.mock("../hooks/useSettings", () => ({
	useSettings: vi.fn(),
	useUpdateReasoning: vi.fn(),
}));

function resetStore() {
	useUiStore.setState({
		defaultAgentId: "",
		hasCompletedOnboarding: false,
		isOnboardingOpen: false,
		sessionsViewMode: "board",
		createProjectNonce: 0,
	});
}

describe("OnboardingTour", () => {
	beforeEach(() => {
		window.localStorage.clear();
		ctx.agents = {
			authorized: [{ id: "codex", label: "Codex" }],
			installed: [
				{ id: "codex", label: "Codex" },
				{ id: "claude-code", label: "Claude Code" },
			],
			supported: [],
		};
		ctx.isPending = false;
		ctx.installTmux.mockReset();
		ctx.installTmux.mockResolvedValue({ status: "installed" });
		aoBridge.app.installTmux = ctx.installTmux;
		ctx.updateReasoning.mockReset();
		ctx.updateReasoning.mockResolvedValue({
			provider: "codex",
			ready: true,
			verified: false,
		});
		vi.mocked(useSettings).mockReturnValue({
			settings: {
				defaultSessionMode: "tui",
				chatHarnesses: ["codex"],
				repositoryContext: {
					maxFiles: null,
					maxBytes: null,
					maxVisited: null,
					effectiveMaxFiles: 32,
					effectiveMaxBytes: 96 * 1024,
					effectiveMaxVisited: 20_000,
				},
				reasoning: {
					provider: "openai",
					model: "",
					effort: "",
					configured: true,
					ready: true,
					keyConfigured: true,
					verified: false,
				},
			},
			isLoading: false,
			error: undefined,
		});
		vi.mocked(useUpdateReasoning).mockReturnValue({
			update: ctx.updateReasoning,
			saving: false,
			error: undefined,
		});
		resetStore();
	});

	it("waits for the daemon before offering setup, then opens on a first run", () => {
		const { rerender } = render(<OnboardingTour daemonReady={false} />);
		expect(screen.queryByTestId("onboarding-tour")).not.toBeInTheDocument();

		rerender(<OnboardingTour daemonReady />);
		expect(screen.getByTestId("onboarding-tour")).toBeInTheDocument();
		expect(
			screen.getByText("Bring an Outcome. Keep the final say."),
		).toBeInTheDocument();
		expect(screen.getByLabelText("Step 1 of 5")).toBeInTheDocument();
	});

	it("stays closed once the tour has been finished before", () => {
		useUiStore.setState({ hasCompletedOnboarding: true });
		render(<OnboardingTour daemonReady />);

		expect(screen.queryByTestId("onboarding-tour")).not.toBeInTheDocument();
	});

	it("walks forward and back through the first-run path", () => {
		render(<OnboardingTour daemonReady />);

		expect(screen.getByRole("button", { name: "Back" })).toBeDisabled();
		fireEvent.click(screen.getByRole("button", { name: /Set up Kennel/ }));
		expect(screen.getByText("Check the local runtime")).toBeInTheDocument();
		expect(screen.getByLabelText("Step 2 of 5")).toBeInTheDocument();

		fireEvent.click(screen.getByRole("button", { name: /Continue/ }));
		expect(screen.getByText("Connect your first provider")).toBeInTheDocument();

		fireEvent.click(screen.getByRole("button", { name: "Back" }));
		expect(screen.getByText("Check the local runtime")).toBeInTheDocument();
	});

	it("connects Codex from live inventory and never claims project pairing", async () => {
		render(<OnboardingTour daemonReady />);
		fireEvent.click(screen.getByRole("button", { name: /Set up Kennel/ }));
		fireEvent.click(screen.getByRole("button", { name: /Continue/ }));
		fireEvent.click(screen.getByRole("button", { name: "Use Codex" }));
		await waitFor(() =>
			expect(ctx.updateReasoning).toHaveBeenCalledWith({
				provider: "codex",
				model: "",
				effort: "",
			}),
		);
		expect(useUiStore.getState().defaultAgentId).toBe("codex");
		expect(
			screen.getByText(/native confirmation before the first pairing/),
		).toBeInTheDocument();
	});

	it("shows an honest Codex empty state with recovery", () => {
		ctx.agents = { authorized: [], installed: [], supported: [] };
		render(<OnboardingTour daemonReady />);
		fireEvent.click(screen.getByRole("button", { name: /Set up Kennel/ }));
		fireEvent.click(screen.getByRole("button", { name: /Continue/ }));
		expect(screen.getByText("Codex not found")).toBeInTheDocument();
		expect(
			screen.getByRole("button", { name: "Check again" }),
		).toBeInTheDocument();
	});

	it("opens the real Project registration flow from the final step", () => {
		render(<OnboardingTour daemonReady />);
		fireEvent.click(screen.getByRole("button", { name: /Set up Kennel/ }));
		for (let i = 0; i < 3; i++)
			fireEvent.click(screen.getByRole("button", { name: /Continue/ }));
		fireEvent.click(
			screen.getByRole("button", { name: "Choose a Project folder" }),
		);
		expect(screen.queryByTestId("onboarding-tour")).not.toBeInTheDocument();
		expect(useUiStore.getState().createProjectNonce).toBe(1);
	});

	it("keeps runtime readiness unknown after Homebrew exits successfully", async () => {
		render(<OnboardingTour daemonReady />);
		fireEvent.click(screen.getByRole("button", { name: /Set up Kennel/ }));
		fireEvent.click(
			screen.getByRole("button", { name: "Install tmux with Homebrew" }),
		);
		await screen.findByText(/Homebrew finished installing tmux/);
		expect(
			screen.queryByText("tmux was installed successfully."),
		).not.toBeInTheDocument();
	});

	it("maps ready without verified to selection-only copy", async () => {
		render(<OnboardingTour daemonReady />);
		fireEvent.click(screen.getByRole("button", { name: /Set up Kennel/ }));
		fireEvent.click(screen.getByRole("button", { name: /Continue/ }));
		fireEvent.click(screen.getByRole("button", { name: "Use Codex" }));
		await screen.findByText(/has not verified a model call yet/);
		expect(screen.queryByText(/ready for planning/)).not.toBeInTheDocument();
	});

	it("only saves the local worker preference after reasoning selection succeeds", async () => {
		ctx.updateReasoning.mockRejectedValueOnce(new Error("offline"));
		render(<OnboardingTour daemonReady />);
		fireEvent.click(screen.getByRole("button", { name: /Set up Kennel/ }));
		fireEvent.click(screen.getByRole("button", { name: /Continue/ }));
		fireEvent.click(screen.getByRole("button", { name: "Use Codex" }));
		await screen.findByText(/setting could not be saved/);
		expect(useUiStore.getState().defaultAgentId).toBe("");
	});

	it("focuses and announces each step, traps Tab, and Escape preserves close semantics", async () => {
		const opener = document.createElement("button");
		opener.textContent = "Open setup";
		document.body.appendChild(opener);
		opener.focus();
		render(<OnboardingTour daemonReady />);
		await waitFor(() =>
			expect(
				screen.getByText("Bring an Outcome. Keep the final say."),
			).toHaveFocus(),
		);
		fireEvent.click(screen.getByRole("button", { name: /Set up Kennel/ }));
		expect(screen.getByText("Check the local runtime")).toHaveFocus();
		expect(screen.getByText(/Step 2 of 5: System check/)).toHaveAttribute(
			"aria-live",
			"polite",
		);
		const dialog = screen.getByRole("dialog");
		fireEvent.keyDown(dialog, { key: "Tab" });
		expect(dialog.contains(document.activeElement)).toBe(true);
		fireEvent.keyDown(dialog, { key: "Escape" });
		expect(screen.queryByTestId("onboarding-tour")).not.toBeInTheDocument();
		expect(useUiStore.getState().hasCompletedOnboarding).toBe(true);
		opener.remove();
	});

	it("treats skipping as answered so the tour does not return next launch", () => {
		render(<OnboardingTour daemonReady />);
		fireEvent.click(screen.getByRole("button", { name: "Skip tour" }));

		expect(useUiStore.getState().hasCompletedOnboarding).toBe(true);
		expect(window.localStorage.getItem("kennel.onboarding.completed")).toBe(
			"true",
		);
	});
});
