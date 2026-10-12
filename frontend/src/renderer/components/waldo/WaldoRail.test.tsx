import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { WaldoRail } from "./WaldoRail";

describe("WaldoRail", () => {
  it("shows no plausible conversation when Waldo is unconfigured", () => {
    render(
      <WaldoRail
        contextLabel="Home · Today"
        onClose={vi.fn()}
        previewEnabled={false}
      />,
    );

    expect(
      screen.getByRole("heading", { name: "Waldo isn't connected yet" }),
    ).toBeInTheDocument();
    expect(screen.getByText("Back")).toBeInTheDocument();
    expect(
      screen.getByText("Home and Work remain available without a model connection."),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("textbox", { name: "Message Waldo" }),
    ).not.toBeInTheDocument();
    expect(screen.queryByRole("tab", { name: "Conversation" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "New conversation" })).not.toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Observation preview" })).not.toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Agent activity preview" })).not.toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Result preview" })).not.toBeInTheDocument();
    expect(screen.queryByText("Preparing your meeting brief")).not.toBeInTheDocument();
  });

  it("labels the deterministic preview surface as non-live", () => {
    render(
      <WaldoRail
        contextLabel="Home · Today"
        onClose={vi.fn()}
        previewEnabled
      />,
    );

    expect(
      screen.getByRole("status", { name: "Waldo preview boundary" }),
    ).toHaveTextContent("Interaction preview");
    expect(screen.getByText(/No model or agent is running/)).toBeInTheDocument();
    expect(screen.getByText("Home · Today")).toBeInTheDocument();
    expect(screen.getByRole("textbox", { name: "Message Waldo" })).toBeEnabled();
    expect(screen.getByText(/messages are not sent or saved/i)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Send/ })).not.toBeInTheDocument();
  });

  it("keeps the bounded task, evidence, approval, and return path in Activity", async () => {
    const user = userEvent.setup();
    render(
      <WaldoRail
        contextLabel="Work · Pricing outcome"
        onClose={vi.fn()}
        previewEnabled
      />,
    );

    await user.click(screen.getByRole("tab", { name: "Activity" }));
    await user.click(screen.getByRole("button", { name: "Inspect run" }));

    const activity = screen.getByRole("region", { name: "Agent activity preview" });
    expect(activity).toHaveTextContent("Prepare the pricing workshop brief");
    expect(activity).toHaveTextContent("Completion condition");
    expect(activity).toHaveTextContent("Calendar, decision note, current Work context");
    expect(within(activity).getAllByRole("listitem")).toHaveLength(4);
    expect(activity).toHaveTextContent("Wait before any outward action");
    expect(activity).toHaveTextContent("Approval required");
    expect(activity).toHaveTextContent("2 source notes attached");
    expect(activity).toHaveTextContent("Returns to Work · no Outcome created");
  });

  it("keeps proposal review local and non-authoritative", async () => {
    const user = userEvent.setup();
    render(
      <WaldoRail
        contextLabel="Home · Today"
        onClose={vi.fn()}
        previewEnabled
      />,
    );

    await user.click(screen.getByRole("button", { name: "Review proposed command" }));

    expect(
      screen.getByRole("status", { name: "Proposal preview status" }),
    ).toHaveTextContent("Nothing was created, sent, or saved");
  });

  it("returns from the Work rail through the Inspector tab", async () => {
    const user = userEvent.setup();
    const onReturnToInspector = vi.fn();
    render(
      <WaldoRail
        contextLabel="Work · Session"
        onClose={vi.fn()}
        onReturnToInspector={onReturnToInspector}
        previewEnabled={false}
      />,
    );

    await user.click(screen.getByRole("tab", { name: "Inspector" }));

    expect(onReturnToInspector).toHaveBeenCalledTimes(1);
  });

  it("separates Conversation from Activity and starts in the contextual episode", () => {
    render(
      <WaldoRail
        contextLabel="Home · Today"
        onClose={vi.fn()}
        previewEnabled
      />,
    );

    expect(screen.getByRole("tab", { name: "Conversation" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    expect(screen.getByRole("tab", { name: "Activity" })).toHaveAttribute(
      "aria-selected",
      "false",
    );
    expect(screen.getByText("Contextual", { selector: "span" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Pricing workshop" })).toBeInTheDocument();
  });

  it("offers a fresh episode without inventing a prior exchange", async () => {
    const user = userEvent.setup();
    render(
      <WaldoRail
        contextLabel="Home · Today"
        onClose={vi.fn()}
        previewEnabled
      />,
    );

    await user.click(screen.getByRole("button", { name: "New conversation" }));

    expect(screen.getByText("Fresh", { selector: "span" })).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "Start a focused conversation" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("group", { name: "Suggested prompts" })).toBeInTheDocument();
    expect(
      screen.queryByText("Prepare me for the pricing workshop and show what needs my approval."),
    ).not.toBeInTheDocument();
  });

  it("lets the user detach suggested context without changing the native surface", async () => {
    const user = userEvent.setup();
    render(
      <WaldoRail
        contextLabel="Work · Pricing outcome"
        onClose={vi.fn()}
        previewEnabled
      />,
    );

    expect(screen.getByText("Work · Pricing outcome")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Detach context" }));

    expect(screen.getByText("No attached context")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Pricing workshop" })).toBeInTheDocument();
  });

  it("keeps observation, candidate, proposal, and approval semantics separate", () => {
    render(
      <WaldoRail
        contextLabel="Home · Today"
        onClose={vi.fn()}
        previewEnabled
      />,
    );

    const observation = screen.getByRole("region", { name: "Observation preview" });
    expect(observation).toHaveTextContent("Noticed");
    expect(observation).toHaveTextContent("Candidate");
    expect(observation).toHaveTextContent("Not admitted to Memory or responsibility");

    const proposal = screen.getByRole("region", { name: "Proposal preview" });
    expect(proposal).toHaveTextContent("Proposal");
    expect(proposal).toHaveTextContent("Needs you");
    expect(proposal).toHaveTextContent("No command has run");
  });

  it("returns to a topic with a short result and keeps the outcome unknown", async () => {
    const user = userEvent.setup();
    render(
      <WaldoRail
        contextLabel="Work · Launch readiness"
        onClose={vi.fn()}
        previewEnabled
      />,
    );

    await user.click(screen.getByRole("button", { name: "Launch readiness" }));

    expect(screen.getByText("Returning", { selector: "span" })).toBeInTheDocument();
    const result = screen.getByRole("region", { name: "Result preview" });
    expect(result).toHaveTextContent("Result ready");
    expect(result).toHaveTextContent("Outcome · Unknown");
    expect(result).not.toHaveTextContent("Three remaining checks");

    await user.click(screen.getByRole("button", { name: "Show result detail" }));
    expect(result).toHaveTextContent("Three remaining checks");
    expect(result).toHaveTextContent("No verification or AcceptanceDecision exists");
  });

  it("makes the bounded run fully inspectable without creating a specialist roster", async () => {
    const user = userEvent.setup();
    render(
      <WaldoRail
        contextLabel="Work · Pricing outcome"
        onClose={vi.fn()}
        previewEnabled
      />,
    );

    await user.click(screen.getByRole("tab", { name: "Activity" }));

    expect(screen.queryByRole("region", { name: "Observation preview" })).not.toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Proposal preview" })).not.toBeInTheDocument();
    const activity = screen.getByRole("region", { name: "Agent activity preview" });
    expect(activity).toHaveTextContent("Running");
    expect(activity).toHaveTextContent("Current step");
    expect(activity).not.toHaveTextContent("Bounded research specialist");

    await user.click(screen.getByRole("button", { name: "Inspect run" }));

    expect(activity).toHaveTextContent("Goal");
    expect(activity).toHaveTextContent("Completion condition");
    expect(activity).toHaveTextContent("Scope");
    expect(activity).toHaveTextContent("Delegation");
    expect(activity).toHaveTextContent("Bounded research specialist");
    expect(activity).toHaveTextContent("Permitted sources");
    expect(activity).toHaveTextContent("Plan");
    expect(activity).toHaveTextContent("Authority");
    expect(activity).toHaveTextContent("No external messages, mutations, or acceptance");
    expect(activity).toHaveTextContent("Approval required");
    expect(activity).toHaveTextContent("Evidence");
    expect(activity).toHaveTextContent("Return path");
    expect(activity).toHaveTextContent("Result ready");
    expect(activity).toHaveTextContent("Outcome · Unknown");
    expect(activity).toHaveTextContent("Delegated under Waldo for this run only");
    expect(activity).not.toHaveTextContent("Verified outcome");
    expect(activity).not.toHaveTextContent("Done");
  });

  it("previews a governed specialist only inside Waldo Activity", async () => {
    const user = userEvent.setup();
    render(
      <WaldoRail
        contextLabel="Work · Pricing outcome"
        onClose={vi.fn()}
        previewEnabled
      />,
    );

    expect(screen.queryByRole("button", { name: "Create specialist" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("tab", { name: "Activity" }));
    await user.click(screen.getByRole("button", { name: "Create specialist" }));

    const builder = screen.getByRole("region", { name: "Specialist preview builder" });
    expect(builder).toHaveTextContent("Preview only");
    expect(within(builder).getByLabelText("Purpose")).toHaveValue(
      "Research the open questions for the pricing workshop",
    );
    expect(within(builder).getByLabelText("Scope")).toHaveValue(
      "This pricing workshop only",
    );
    expect(within(builder).getByLabelText("Permitted sources and tools")).toHaveValue(
      "Calendar, decision note, current Work context",
    );
    expect(within(builder).getByLabelText("Authority ceiling")).toHaveValue(
      "Read and draft only; no outward action",
    );
    expect(within(builder).getByLabelText("Budget")).toHaveValue("20 minutes");
    expect(within(builder).getByLabelText("Completion condition")).toHaveValue(
      "Return a source-linked brief with unresolved questions",
    );
    expect(within(builder).getByLabelText("Evidence expectation")).toHaveValue(
      "Cite each claim and distinguish observation from inference",
    );
    expect(within(builder).getByLabelText("Waldo return destination")).toHaveValue(
      "Work · Pricing outcome",
    );

    await user.click(within(builder).getByRole("button", { name: "Preview specialist" }));

    const specialist = screen.getByRole("article", { name: "Research brief specialist" });
    expect(specialist).toHaveTextContent("Ready · preview only");
    expect(specialist).toHaveTextContent("This pricing workshop only");
    expect(specialist).toHaveTextContent("Nothing was created, connected, run, or saved");
    expect(specialist).toHaveTextContent("Returns to Work · Pricing outcome");
    await user.click(within(specialist).getByRole("button", { name: "Pause specialist" }));
    expect(specialist).toHaveTextContent("Paused · preview only");
    await user.click(within(specialist).getByRole("button", { name: "Revoke specialist" }));
    expect(specialist).toHaveTextContent("Revoked · preview only");
  });

  it("switches modes with tablist arrow keys", async () => {
    const user = userEvent.setup();
    render(
      <WaldoRail
        contextLabel="Home · Today"
        onClose={vi.fn()}
        previewEnabled
      />,
    );

    const conversation = screen.getByRole("tab", { name: "Conversation" });
    conversation.focus();
    await user.keyboard("{ArrowRight}");

    expect(screen.getByRole("tab", { name: "Activity" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    expect(screen.getByRole("tab", { name: "Activity" })).toHaveFocus();
  });
});
