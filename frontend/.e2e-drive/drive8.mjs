import { launch } from './drivelib.mjs';
const { browser, page } = await launch();
await page.goto('http://127.0.0.1:5199/#/work?view=outcomes&portfolio=scratch&project=scratch', { waitUntil: 'domcontentloaded' });
await page.waitForTimeout(2000);
await page.locator('button:has-text("New Outcome")').first().click();
await page.waitForTimeout(1500);
const info = await page.evaluate(() => {
  const vis = el => { const r = el.getBoundingClientRect(); return r.width > 0 && r.height > 0; };
  return [...document.querySelectorAll('button')].filter(vis).map(b => ({
    testid: b.getAttribute('data-testid'), type: b.type, disabled: b.disabled,
    text: JSON.stringify(b.innerText), aria: b.getAttribute('aria-label')
  })).filter(b => b.testid || /continue|worker|orchestrator|voice/i.test(b.text + (b.aria || '')));
});
console.log(JSON.stringify(info, null, 1));
await browser.close();
