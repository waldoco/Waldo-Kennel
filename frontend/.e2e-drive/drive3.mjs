import { launch, dumpControls } from './drivelib.mjs';
const { browser, page, errors } = await launch();
await page.locator('text=Scratch').first().click();
await page.waitForTimeout(2000);
console.log('URL:', page.url());
await page.screenshot({ path: '/tmp/shots/04-project-chosen.png' });
await dumpControls(page);
console.log('ERRORS:', JSON.stringify(errors.slice(0, 8)));
await browser.close();
