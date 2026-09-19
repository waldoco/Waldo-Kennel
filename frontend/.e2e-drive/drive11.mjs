import { launch } from './drivelib.mjs';
const { browser, page, errors } = await launch();
await page.goto('http://127.0.0.1:5199/#/work?view=outcomes&portfolio=scratch&project=scratch', { waitUntil: 'domcontentloaded' });
await page.waitForTimeout(2000);
await page.locator('button:has-text("New Outcome")').first().click();
await page.waitForTimeout(1200);
const ta = page.locator('textarea').first();
await ta.fill("A file named E2E-PROOF.md exists in the project root whose first line is exactly: kennel e2e flow proof");
await ta.focus();
await page.keyboard.press('Meta+Enter');
await page.waitForTimeout(2500);
console.log('after Meta+Enter URL:', page.url());
let tail = await page.evaluate(() => document.body.innerText.slice(-500));
console.log('TAIL1:', JSON.stringify(tail.slice(-300)));
await page.screenshot({ path: '/tmp/shots/10-after-metaenter.png' });
const submit = page.locator('button[data-testid="intake-capture-submit"]');
console.log('submit disabled?', await submit.isDisabled().catch(() => 'n/a'));
if (!(await submit.isDisabled().catch(() => true))) {
  await submit.click({ force: true });
  await page.waitForTimeout(3000);
  console.log('after submit click URL:', page.url());
  await page.screenshot({ path: '/tmp/shots/11-after-submit.png' });
}
for (let i = 0; i < 4; i++) { await page.waitForTimeout(8000); await page.screenshot({ path: `/tmp/shots/11-watch-${i}.png` }); console.log('t+' + (i+1)*8 + 's:', page.url()); }
console.log('ERRORS:', JSON.stringify(errors.slice(0, 5)));
await browser.close();
