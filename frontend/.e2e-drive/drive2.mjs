import { chromium } from 'playwright';
const browser = await chromium.launch();
const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
const errors = [];
page.on('pageerror', e => errors.push('PAGEERROR: ' + e.message));
await page.goto('http://127.0.0.1:5199/', { waitUntil: 'domcontentloaded', timeout: 30000 });
await page.waitForTimeout(3000);
// Walk the onboarding tour via its primary button until it disappears
for (let i = 1; i <= 8; i++) {
  const tour = page.locator('text=/\\d of 5/').first();
  if (!(await tour.count())) { console.log('tour gone at step', i); break; }
  const label = await page.locator('button:has-text("Set up Kennel"), button:has-text("Continue"), button:has-text("Next"), button:has-text("Finish"), button:has-text("Done"), button:has-text("Get started")').first();
  const btnText = (await label.count()) ? (await label.innerText()) : null;
  console.log('step', i, 'primary button:', btnText);
  await page.screenshot({ path: `/tmp/shots/02-tour-${i}.png` });
  if (!btnText) break;
  // Some steps may need a selection first; try clicking primary, and if it is disabled, dump visible buttons
  const disabled = await label.isDisabled().catch(() => false);
  if (disabled) {
    const btns = await page.evaluate(() => [...document.querySelectorAll('button')].filter(b => b.getBoundingClientRect().width > 0).map(b => b.innerText.trim().slice(0, 40)));
    console.log('primary disabled; visible buttons:', JSON.stringify(btns));
    break;
  }
  await label.click();
  await page.waitForTimeout(1200);
}
console.log('URL now:', page.url());
await page.screenshot({ path: '/tmp/shots/03-after-tour.png' });
console.log('ERRORS:', JSON.stringify(errors.slice(0, 8)));
await browser.close();
