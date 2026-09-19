import { chromium } from 'playwright';
const browser = await chromium.launch();
const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
const errors = [];
page.on('pageerror', e => errors.push('PAGEERROR: ' + e.message));
page.on('console', m => { if (m.type() === 'error') errors.push('CONSOLE: ' + m.text().slice(0, 200)); });
await page.goto('http://127.0.0.1:5199/', { waitUntil: 'networkidle', timeout: 30000 }).catch(e => errors.push('NAV: ' + e.message));
await page.waitForTimeout(2500);
console.log('URL:', page.url());
console.log('TITLE:', await page.title());
await page.screenshot({ path: '/tmp/shots/01-landing.png', fullPage: false });
const controls = await page.evaluate(() => {
  const vis = el => { const r = el.getBoundingClientRect(); return r.width > 0 && r.height > 0; };
  const btns = [...document.querySelectorAll('button, a, input, textarea, [role=tab], select')].filter(vis).map(el => ({
    tag: el.tagName.toLowerCase(), text: (el.innerText || el.placeholder || el.getAttribute('aria-label') || '').trim().slice(0, 60)
  }));
  return btns.slice(0, 60);
});
console.log(JSON.stringify(controls, null, 1));
console.log('ERRORS:', JSON.stringify(errors.slice(0, 10)));
await browser.close();
