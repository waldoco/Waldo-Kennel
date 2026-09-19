import { launch } from './drivelib.mjs';
const { browser, page, errors } = await launch();
await page.goto('http://127.0.0.1:5199/#/work?view=outcomes&portfolio=scratch&project=scratch', { waitUntil: 'domcontentloaded' });
await page.waitForTimeout(2000);
await page.locator('button:has-text("New Outcome")').first().click();
await page.waitForTimeout(1200);
await page.locator('textarea').first().fill("A file named E2E-PROOF.md exists in the project root whose first line is exactly: kennel e2e flow proof");
// find the + button near the textarea footer
const plus = page.locator('button[aria-label="Worker agent"]');
console.log('worker btn count:', await plus.count());
const allAria = await page.evaluate(() => [...document.querySelectorAll('button[aria-label]')].filter(b => b.getBoundingClientRect().width > 0).map(b => b.getAttribute('aria-label')));
console.log('aria buttons:', JSON.stringify(allAria));
// try clicking the + (first icon button inside the form area)
const form = page.locator('form, [data-testid*=intake], main').first();
const plusBtn = page.locator('button:has(svg)').nth(16);
console.log('trying worker agent click then screenshot');
await plus.click({ force: true, timeout: 5000 }).catch(e => console.log('fail:', e.message.split('\n')[0]));
await page.waitForTimeout(900);
await page.screenshot({ path: '/tmp/shots/09-after-worker-click.png' });
const pop = await page.evaluate(() => document.body.innerText.slice(-800));
console.log('BODY TAIL:', JSON.stringify(pop));
await browser.close();
