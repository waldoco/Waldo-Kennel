import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, it, vi } from "vitest";

const { get, post, apiErrorCode } = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn(), apiErrorCode: vi.fn(() => undefined as string | undefined) }));

vi.mock("../../lib/api-client", () => ({
	apiClient: { GET: get, POST: post },
	apiErrorCode,
	apiErrorMessage: (error: Error) => error.message,
}));

import { OutcomeDocumentsPanel } from "./OutcomeDocumentsPanel";

const selectedContext = {
	id: "docs-1",
	outcomeId: "outcome-1",
	revision: 1,
	digest: "digest-1",
	state: "selected",
	selectedAt: "2026-09-11T00:00:00Z",
	changedSources: [],
	sources: [{ id: "source-1", name: "brief.md", sourcePath: "/tmp/brief.md", contentDigest: "source-digest", position: 0, sizeBytes: 12 }],
};

beforeEach(() => {
	vi.clearAllMocks();
	apiErrorCode.mockReturnValue(undefined);
	get.mockResolvedValue({ data: { documentContext: selectedContext } });
	post.mockImplementation(async (_url: string, request: { body: { paths?: string[]; expectedDigest?: string } }) => ({
		data: {
			documentContext: request.body.paths
				? selectedContext
				: { ...selectedContext, state: "approved", approvedAt: "2026-09-11T00:01:00Z" },
		},
	}));
});

function renderPanel() {
	return render(
		<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })}>
			<OutcomeDocumentsPanel outcomeId="outcome-1" />
		</QueryClientProvider>,
	);
}

it("selects local paths, then requires explicit approval of the returned digest", async () => {
	const user = userEvent.setup();
	renderPanel();
	const file = new File(["source"], "brief.md", { type: "text/markdown" });
	Object.defineProperty(file, "path", { value: "/tmp/brief.md" });
	fireEvent.change(await screen.findByLabelText("Choose local files"), { target: { files: [file] } });
	await user.click(screen.getByRole("button", { name: "Select and snapshot" }));

	await waitFor(() => expect(post).toHaveBeenCalledWith(
		"/api/v1/outcomes/{outcomeId}/documents",
		expect.objectContaining({ body: { paths: ["/tmp/brief.md"] } }),
	));
	await user.click(await screen.findByRole("button", { name: "Approve this source scope" }));
	await waitFor(() => expect(post).toHaveBeenCalledWith(
		"/api/v1/outcomes/{outcomeId}/documents/approval",
		expect.objectContaining({ body: { expectedDigest: "digest-1" } }),
	));
});

it("treats no selected documents as the normal optional state", async () => {
	apiErrorCode.mockReturnValue("DOCUMENT_CONTEXT_STALE");
	get.mockResolvedValue({ data: undefined, error: { code: "DOCUMENT_CONTEXT_STALE", message: "This Outcome has no selected documents" } });
	renderPanel();
	expect(await screen.findByTestId("outcome-documents-optional")).toBeInTheDocument();
	expect(screen.getByText("Optional")).toBeInTheDocument();
	expect(screen.queryByRole("alert")).not.toBeInTheDocument();
	expect(screen.queryByRole("button", { name: /refresh/i })).not.toBeInTheDocument();
});

it("still surfaces real document-context failures", async () => {
	apiErrorCode.mockReturnValue("DOCUMENT_CONTEXT_UNAVAILABLE");
	get.mockResolvedValue({ data: undefined, error: { code: "DOCUMENT_CONTEXT_UNAVAILABLE", message: "Supplied-document context is not wired in this daemon" } });
	renderPanel();
	expect(await screen.findByRole("alert")).toHaveTextContent("Supplied-document context is not wired in this daemon");
	expect(screen.queryByTestId("outcome-documents-optional")).not.toBeInTheDocument();
});
