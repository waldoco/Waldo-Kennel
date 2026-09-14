import { expect, test, type Locator, type Page } from "@playwright/test";

// Regression guard for #366 (macOS): the sidebar's "Kennel" brand must stay
// readable in both chrome systems. Populated board routes use the Figma shell
// without TitlebarNav; standard routes keep the navigation cluster.
//
// macOS-only: TitlebarNav (and the bug) gate on navigator.userAgent looking like
// a Mac, read once at module load. Force a Mac UA so this is deterministic
// regardless of the host/CI OS.
test.use({
	userAgent:
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
});

const brand = (page: Page) => page.getByText("Kennel", { exact: true });

// Two boxes overlap iff they intersect on both axes.
function overlaps(a: { x: number; y: number; width: number; height: number }, b: typeof a) {
	return a.x < b.x + b.width && a.x + a.width > b.x && a.y < b.y + b.height && a.y + a.height > b.y;
}

// The brand <span> has `truncate` (overflow:hidden), so it stays "visible" even
// when clipped to nothing. Compare scroll vs client width to prove the wordmark
// is actually fully rendered, not just present-but-clipped.
async function isTruncated(span: Locator) {
	return span.evaluate((el) => el.scrollWidth > el.clientWidth + 1);
}

async function expectBrandClearsCluster(page: Page) {
	const cluster = page.locator('[data-slot="titlebar-nav"]');
	await expect(cluster).toBeVisible();
	const span = brand(page);
	await expect(span).toBeVisible();

	const clusterBox = await cluster.boundingBox();
	const brandBox = await span.boundingBox();
	expect(clusterBox).not.toBeNull();
	expect(brandBox).not.toBeNull();

	expect(overlaps(brandBox!, clusterBox!)).toBe(false);
	expect(await isTruncated(span)).toBe(false);
}

async function expectFigmaBoardBrand(page: Page) {
	await expect(page.locator('[data-slot="titlebar-nav"]')).toHaveCount(0);
	const span = brand(page);
	await expect(span).toBeVisible();
	expect(await isTruncated(span)).toBe(false);
}

test("Home route: brand clears the macOS titlebar cluster and stays readable", async ({ page }) => {
	await page.goto("/#/home");
	await expect(page.getByRole("navigation", { name: "Home destinations" })).toBeVisible();
	await expectBrandClearsCluster(page);
});

test("project board route: brand clears the macOS titlebar cluster and stays readable", async ({ page }) => {
	await page.goto("/");
	await expect(page.getByText("Projects")).toBeVisible();

	// In-app nav to /projects/:id (a hard load boots the router at the board).
	await page.locator('[data-sidebar="menu-button"]').filter({ hasText: "kennel-design" }).first().click();
	// The active project row marks itself aria-current=page once navigation lands.
	await expect(page.locator('[aria-current="page"]')).toBeVisible();

	await expectFigmaBoardBrand(page);
});

test("brand stays readable when navigating from the Work project overview to a session", async ({ page }) => {
	// /projects/:id redirects into the project Work overview in Work launch mode.
	await page.goto("/#/projects/kennel-design");
	await expect(page).toHaveURL(/#\/work\?/);
	await expect(page.getByText("Projects").first()).toBeVisible();
	await expectFigmaBoardBrand(page);

	await page.getByRole("button", { name: "Open Build screenshot-ready dashboard data" }).click();
	await expect(page.getByTestId("session-detail")).toBeVisible();

	await expectFigmaBoardBrand(page);
});
