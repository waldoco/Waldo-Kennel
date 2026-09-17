import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { BoundedComposer } from "./BoundedComposer";

describe("BoundedComposer", () => {
	it("carries typed text back to the host via onChange", () => {
		const onChange = vi.fn();
		render(<BoundedComposer onChange={onChange} onSubmit={vi.fn()} submitLabel="Send" value="" />);
		fireEvent.change(screen.getByRole("textbox"), { target: { value: "hi" } });
		expect(onChange).toHaveBeenCalledWith("hi");
	});

	it("disables submit until there is non-whitespace content", () => {
		render(<BoundedComposer onChange={vi.fn()} onSubmit={vi.fn()} submitLabel="Send" value="   " />);
		expect(screen.getByTestId("bounded-composer-submit")).toBeDisabled();
	});

	it("submits on click when there is content", () => {
		const onSubmit = vi.fn();
		render(<BoundedComposer onChange={vi.fn()} onSubmit={onSubmit} submitLabel="Send" value="do the thing" />);
		fireEvent.click(screen.getByTestId("bounded-composer-submit"));
		expect(onSubmit).toHaveBeenCalledTimes(1);
	});

	it("submits on Enter, but not Shift+Enter, from the keyboard", () => {
		const onSubmit = vi.fn();
		render(<BoundedComposer onChange={vi.fn()} onSubmit={onSubmit} submitLabel="Send" value="do the thing" />);
		const textarea = screen.getByRole("textbox");
		fireEvent.keyDown(textarea, { key: "Enter", shiftKey: true });
		expect(onSubmit).not.toHaveBeenCalled();
		fireEvent.keyDown(textarea, { key: "Enter" });
		expect(onSubmit).toHaveBeenCalledTimes(1);
	});

	it("distinguishes disabled (host policy) from pending (activity), both by visible text", () => {
		const { rerender } = render(<BoundedComposer disabled onChange={vi.fn()} onSubmit={vi.fn()} submitLabel="Send" value="x" />);
		expect(screen.getByRole("textbox")).toBeDisabled();
		expect(screen.getByTestId("bounded-composer-submit")).toHaveTextContent("Send");

		rerender(<BoundedComposer onChange={vi.fn()} onSubmit={vi.fn()} pending submitLabel="Send" value="x" />);
		expect(screen.getByRole("textbox")).toBeDisabled();
		expect(screen.getByTestId("bounded-composer-submit")).toHaveTextContent("Sending…");
		expect(screen.getByTestId("bounded-composer-submit")).toBeDisabled();
	});

	it("renders host-owned scope and source disclosure, and neither by default", () => {
		const { rerender } = render(<BoundedComposer onChange={vi.fn()} onSubmit={vi.fn()} submitLabel="Send" value="" />);
		expect(screen.queryByTestId("bounded-composer-scope")).not.toBeInTheDocument();
		expect(screen.queryByTestId("bounded-composer-source")).not.toBeInTheDocument();

		rerender(
			<BoundedComposer
				onChange={vi.fn()}
				onSubmit={vi.fn()}
				scopeDisclosure="Scoped to WorkUnit wu-42"
				sourceDisclosure="Posting as codex"
				submitLabel="Send"
				value=""
			/>,
		);
		expect(screen.getByTestId("bounded-composer-scope")).toHaveTextContent("Scoped to WorkUnit wu-42");
		expect(screen.getByTestId("bounded-composer-source")).toHaveTextContent("Posting as codex");
	});
});
