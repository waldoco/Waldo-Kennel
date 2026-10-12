import { render, screen } from "@testing-library/react";
import { expect, test, vi } from "vitest";
import { ConfirmDialog } from "./ConfirmDialog";

test("renders a centred Waldo confirm dialog on the raised plane", () => {
	render(
		<ConfirmDialog
			open
			title="Kill session?"
			description="This will stop the session."
			confirmLabel="Confirm kill"
			destructive
			busy
			onConfirm={vi.fn()}
			onOpenChange={vi.fn()}
		/>,
	);

	const dialog = screen.getByRole("dialog", { name: "Kill session?" });

	expect(dialog).toHaveClass("left-[50%]", "top-[50%]", "z-overlay", "bg-wd-raised", "rounded-xl", "p-6");
	expect(screen.getByRole("button", { name: "Confirm kill" }).querySelector("svg")).toBeNull();
});
