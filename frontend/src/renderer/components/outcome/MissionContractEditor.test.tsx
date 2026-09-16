import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import type { ContractRevisionRecord } from "../../hooks/useOutcome";
const { save } = vi.hoisted(() => ({ save: vi.fn() }));
vi.mock("../../hooks/useOutcome", () => ({ useReviseOutcomeContract: () => ({ save, pending: false, reset: vi.fn() }) }));
import { MissionContractEditor } from "./MissionContractEditor";
const contract: ContractRevisionRecord = {
 id: "r1", number: 1, goal: "Review sources", review: "Owner reviews citations", constraints: ["Read only"], nonGoals: [],
 criteria: [{ criterionId: "c1", contractRevisionId: "r1", position: 1, text: "Sources cited" }], successCriteria: ["Sources cited"],
 evidenceExpectations: [{ criterionId: "c1", descriptions: ["Repository path"] }, { criterionId: "c1", descriptions: ["Quoted source"] }],
 temporalCondition: "Today", facets: [{ kind: "research", summary: "Source audit", requirements: ["Citations"] }],
 createdAt: "2026-09-11T00:00:00Z",
};
it("exposes stable selectors for the packaged Outcome journey harness", async () => {
 const user = userEvent.setup();
 render(<MissionContractEditor outcomeId="one" contract={contract} disabled={false} />);
 expect(screen.getByTestId("contract-edit")).toBeInTheDocument();
 await user.click(screen.getByTestId("contract-edit"));
 expect(screen.getByTestId("contract-goal")).toBeInTheDocument();
 expect(screen.getByTestId("contract-criterion-0")).toBeInTheDocument();
 expect(screen.getByTestId("contract-evidence-0")).toBeInTheDocument();
 expect(screen.getByTestId("contract-save")).toBeInTheDocument();
 expect(screen.getByTestId("contract-discard")).toBeInTheDocument();
});
it("saves a new revision preserving supporting details", async () => {
 save.mockResolvedValue({});
 const user = userEvent.setup();
 render(<MissionContractEditor outcomeId="one" contract={contract} disabled={false} />);
 await user.click(screen.getByRole("button", { name: "Edit Contract" }));
 await user.clear(screen.getByRole("textbox", { name: "Desired result" }));
 await user.type(screen.getByRole("textbox", { name: "Desired result" }), "Review provider lifecycle");
 await user.click(screen.getByRole("button", { name: "Save new revision" }));
 await waitFor(() => expect(save).toHaveBeenCalledWith(expect.objectContaining({
  expectedRevision: 1, goal: "Review provider lifecycle", successCriteria: ["Sources cited"],
  criterionEvidence: [["Repository path", "Quoted source"]], temporalCondition: "Today", facets: contract.facets,
  authorityCeiling: expect.objectContaining({ executeLocal: false, useNetwork: false }),
 })));
});
it("preserves unsaved edits when a newer Contract arrives", async () => {
 const user = userEvent.setup();
 const { rerender } = render(<MissionContractEditor outcomeId="one" contract={contract} disabled={false} />);
 await user.click(screen.getByRole("button", { name: "Edit Contract" }));
 await user.type(screen.getByRole("textbox", { name: "Desired result" }), " with my edits");
 rerender(<MissionContractEditor outcomeId="one" contract={{ ...contract, id: "r2", number: 2 }} disabled={false} />);
 expect(screen.getByRole("textbox", { name: "Desired result" })).toHaveValue("Review sources with my edits");
 expect(screen.getByRole("button", { name: "Save new revision" })).toBeDisabled();
 expect(screen.getByRole("alert")).toHaveTextContent("Your draft is preserved");
 expect(screen.getByRole("button", { name: "Discard draft" })).toBeEnabled();
});
