import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
const { get, post, connection } = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn(), connection: { value: "connected" } }));
vi.mock("../../lib/api-client", () => ({ apiClient: { GET: get, POST: post }, apiErrorCode: () => undefined, apiErrorMessage: () => "Unavailable", hasTrustedApiBaseUrl: () => true }));
vi.mock("../../hooks/useEventsConnection", () => ({useEventsConnection: () => connection.value}));
vi.mock("../../hooks/useSettings", () => ({useSettings: () => ({settings:{reasoning:{ready:true}}})}));
vi.mock("../../hooks/useWorkspaceQuery", () => ({useWorkspaceQuery: () => ({data:[]})}));
vi.mock("../settings/ReasoningSettingsSection", () => ({ReasoningSettingsSection: () => null}));
vi.mock("./OutcomeDecideAuthorizeSurface", () => ({OutcomeDecideAuthorizeSurface: () => null}));
vi.mock("./OutcomeProveCloseSurface", () => ({OutcomeProveCloseSurface: () => null}));
vi.mock("./MissionReplanForm", () => ({MissionReplanForm: () => null}));
vi.mock("./MissionUsage", () => ({MissionUsage: () => null}));
import { OutcomeMissionPanel } from "./OutcomeMissionPanel";

function mount(revision: number, state: string, attemptPhase?: string, options?: { runPending?: boolean }) {
	connection.value = state;
	const plan = {id:"plan", outcomeId:"out", number:1, contractRevisionNumber:1, status:"approved", workUnits:[], grants:[]};
	const contract = {id:"contract", number:revision, goal:"Keep source", criteria:[], constraints:[], nonGoals:[], review:"Owner review"};
	const attempt = {id:"attempt", outcomeId:"out", planRevisionId:"plan", workUnitId:"unit", number:1, contractRevisionNumber:1, status:"running", sessions:[], observations:[], receipts:[], presentation:{phase:attemptPhase, unconfirmed:attemptPhase === "unconfirmed", nextAction:"Review liveness"}};
	let releaseRun: (() => void) | undefined;
	get.mockImplementation(async (url: string) => {
		if(url.endsWith("/run")) {
			const response = {data:{runState:{outcomeId:"out", projectId:"project", state:"needs_you", freshness:{contractRevisionNumber:revision, planRevisionId:"plan", proofGeneration:0}, eligibleActions:[{action:"start",available:true}]}}};
			if (options?.runPending) return new Promise((resolve) => { releaseRun = () => resolve(response); });
			return response;
		}
		if(url.endsWith("/plan")) return {data:{plan}};
		if(url.endsWith("/schedule")) return {data:{schedule:{outcomeId:"out", plan, workUnits:[], nextRunnableWorkUnitId:"unit"}}};
		if(url.endsWith("/attempts")) return {data:{attempts:attemptPhase ? [attempt] : []}};
		if(url.endsWith("/proof")) return {data:{proof:{criteria:[], decisions:[]}}};
		return {data:{outcome:{id:"out", title:"Preserve source", currentRevisionNumber:revision, currentRevision:contract, history:[contract], latestPlan:plan, updatedAt:"2026-09-10T00:00:00Z"}}};
	});
	post.mockReset().mockResolvedValue({data:{attempt}});
	const cache = new QueryClient({defaultOptions:{queries:{retry:false},mutations:{retry:false}}});
	render(<QueryClientProvider client={cache}><OutcomeMissionPanel outcomeId="out" projectId="project" stage="act_observe" expanded={false} onExpand={() => {}} onClose={() => {}} /></QueryClientProvider>);
	return { releaseRun: () => releaseRun?.() };
}

it("keeps the glance truthful while run state is loading", async () => {
	const { releaseRun } = mount(1, "connected", undefined, { runPending: true });
	const glance = within(await screen.findByTestId("mission-glance"));
	expect(glance.getByText("Reading run status…")).toBeInTheDocument();
	expect(glance.queryByText("Live updates connected")).not.toBeInTheDocument();
	releaseRun();
});

it.each([[2,"connected"],[1,"disconnected"]] as const)("withholds new admission for revision %s and SSE %s", async (revision, state) => {
	mount(revision,state);
	expect(await screen.findByTestId("outcome-run-start")).toBeDisabled();
	expect(post).not.toHaveBeenCalled();
	const glance = within(screen.getByTestId("mission-glance"));
	if (state === "disconnected") {
		expect(glance.getByText("Updates disconnected. Reconnect or refresh before deciding.")).toBeInTheDocument();
	} else {
		expect(glance.queryByText("Live updates connected")).not.toBeInTheDocument();
	}
});

it.each([[2,"connected","executing","cancel"],[2,"connected","unconfirmed","contain"],[1,"disconnected","executing","cancel"],[1,"disconnected","unconfirmed","contain"]] as const)("keeps %s/%s %s safety action %s callable", async (revision,state,phase,action) => {
	mount(revision,state,phase);
	const control = await screen.findByTestId(`outcome-run-${action}`);
	expect(control).toBeEnabled();
	await userEvent.click(control);
	await waitFor(() => expect(post).toHaveBeenCalled());
	expect(post.mock.calls[0][0]).toContain(action === "cancel" ? "/cancel" : "/recovery");
});
