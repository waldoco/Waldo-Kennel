import { expect, test } from "@playwright/test";

// Work routes intentionally use the product shell without titlebar history
// chrome. Exercise the persistent arrows between two Home destinations.
test("titlebar back/forward arrows traverse Home history", async ({ page }) => {
	await page.goto("/#/home");
	await expect(page).toHaveURL(/\/home$/);
	await page.getByRole("link", { name: "Open loops" }).click();
	await expect(page).toHaveURL(/home\/open-loops/);

	const back = page.getByRole("button", { name: "Go back" });
	const forward = page.getByRole("button", { name: "Go forward" });

	await expect(back).toBeVisible();
	await expect(forward).toBeVisible();
	await expect(forward).toBeDisabled();
	await expect(back).toBeEnabled();

	await back.click();
	await expect(page).toHaveURL(/\/home$/);
	await expect(forward).toBeEnabled();

	await forward.click();
	await expect(page).toHaveURL(/home\/open-loops/);
});
