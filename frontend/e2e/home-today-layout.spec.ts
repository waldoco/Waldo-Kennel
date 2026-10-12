import { expect, test } from "@playwright/test";

test.use({
	userAgent:
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	viewport: { width: 1512, height: 982 },
});

test("Today keeps one authored column with the Brief's completion control inside it", async ({ page }) => {
	await page.goto("/#/home?homePhase=morning&homeContext=catch_up");

	const column = page.locator(".home-today-body");
	const finish = page.getByRole("button", { name: "Finish morning brief" });
	await expect(column).toBeVisible();
	await expect(finish).toBeVisible();

	const columnBox = await column.boundingBox();
	const finishBox = await finish.boundingBox();
	expect(columnBox).not.toBeNull();
	expect(finishBox).not.toBeNull();
	expect(columnBox!.width).toBeLessThanOrEqual(640);
	expect(finishBox!.x).toBeGreaterThanOrEqual(columnBox!.x);
	expect(finishBox!.x + finishBox!.width).toBeLessThanOrEqual(columnBox!.x + columnBox!.width);
});

test("Today keeps exactly one expanded Quick capture and no horizontal overflow", async ({ page }) => {
	await page.goto("/#/home?homePhase=afternoon&homeContext=before_next");

	await expect(page.getByRole("textbox", { name: "Quick capture" })).toHaveCount(1);
	await expect(page.getByRole("heading", { name: "Before your next thing" })).toBeVisible();

	const hasOverflow = await page.evaluate(
		() => document.documentElement.scrollWidth > document.documentElement.clientWidth,
	);
	expect(hasOverflow).toBe(false);
});
