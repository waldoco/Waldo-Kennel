import { chromium } from 'playwright';
export async function boot(steps) {
  const browser = await chromium.launch();
  const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 }, storageState: '/tmp/shots/state.json' }).catch(() => null);
  return ctx;
}
export async function launch() {
  const browser = await chromium.launch();
  let ctx;
  try { ctx = await browser.newContext({ viewport: { width: 1440, height: 900 }, storageState: '/tmp/shots/state.json' }); }
  catch { ctx = await browser.newContext({ viewport: { width: 1440, height: 900 } }); }
  const page = await ctx.newPage();
  const errors = [];
  page.on('pageerror', e => errors.push('PAGEERROR: ' + e.message));
  await page.goto('http://127.0.0.1:5199/#/work', { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(2500);
  const skip = page.locator('button:has-text("Skip tour")');
  if (await skip.count()) { await skip.click(); await page.waitForTimeout(800); await ctx.storageState({ path: '/tmp/shots/state.json' }); }
  return { browser, ctx, page, errors };
}
export async function dumpControls(page, n = 50) {
  const dump = await page.evaluate(() => {
    const vis = el => { const r = el.getBoundingClientRect(); return r.width > 0 && r.height > 0; };
    return [...document.querySelectorAll('button, a, input, textarea, [role=tab], select, [contenteditable]')]
      .filter(vis).map(el => ({ tag: el.tagName.toLowerCase(), text: (el.innerText || el.placeholder || el.getAttribute('aria-label') || '').trim().slice(0, 70) })).slice(0, 50);
  });
  console.log(JSON.stringify(dump));
}
