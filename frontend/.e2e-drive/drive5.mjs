import { launch, dumpControls } from './drivelib.mjs';
const { browser, page, errors } = await launch();
await page.goto('http://127.0.0.1:5199/#/work?view=outcomes&portfolio=scratch&project=scratch', { waitUntil: 'domcontentloaded' });
await page.waitForTimeout(2000);
await page.locator('button:has-text("New Outcome")').first().click();
await page.waitForTimeout(1500);
const ta = page.locator('textarea').first();
await ta.fill("A file named E2E-PROOF.md exists in the project root whose first line is exactly: kennel e2e flow proof");
await page.screenshot({ path: '/tmp/shots/06-outcome-typed.png' });
await page.locator('button:has-text("Continue")').first().click();
await page.waitForTimeout(4000);
console.log('URL after Continue:', page.url());
await page.screenshot({ path: '/tmp/shots/07-after-continue.png' });
await dumpControls(page);
for (let i = 0; i < 6; i++) {
  await page.waitForTimeout(10000);
  console.log('t+' + (i + 1) * 10 + 's URL:', page.url());
  await page.screenshot({ path: `/tmp/shots/07-watch-${i}.png` });
}
console.log('ERRORS:', JSON.stringify(errors.slice(0, 8)));
await browser.close();
