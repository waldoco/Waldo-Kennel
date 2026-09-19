import { launch } from './drivelib.mjs';
const { browser, page } = await launch();
await page.goto('http://127.0.0.1:5199/#/work?view=outcomes&portfolio=scratch&project=scratch', { waitUntil: 'domcontentloaded' });
await page.waitForTimeout(2000);
await page.locator('button:has-text("New Outcome")').first().click();
await page.waitForTimeout(1500);
const html = await page.evaluate(() => {
  const vis = el => { const r = el.getBoundingClientRect(); return r.width > 0 && r.height > 0; };
  return [...document.querySelectorAll('button')].filter(vis).map(b => b.outerHTML.slice(0, 300));
});
console.log(JSON.stringify(html, null, 1));
await browser.close();
