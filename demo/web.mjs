// Web part of the skill-atlas demo. Run through record.sh, which starts `skill-atlas serve`. Prints the video's path.
// Uses the installed Google Chrome, or the browser at CHROME_PATH if it is set.
import { chromium } from 'playwright';

const base = 'http://127.0.0.1:8080';
const repo = 'https://github.com/openai/skills';
const org = 'https://github.com/JetBrains';

// Analyzing an organization takes a while, so its map is built before recording; the server keeps it for 5 minutes.
const warmStart = Date.now();
const warm = await fetch(`${base}/scan?org=${org}`);
if (!warm.ok) throw new Error(`analyzing ${org} failed: ${warm.status}`);
const warmSeconds = Math.round((Date.now() - warmStart) / 1000);

const browser = await chromium.launch(process.env.CHROME_PATH ? { executablePath: process.env.CHROME_PATH } : { channel: 'chrome' });
const context = await browser.newContext({
  viewport: { width: 1920, height: 1080 },
  recordVideo: { dir: 'out/web-video', size: { width: 1920, height: 1080 } },
});

// The video records the viewport at 1x, so zoom pages to 150% (like Cmd+ in a browser) to keep the text readable.
// There is no address bar in the recording, so each page shows a caption with the current step and URL.
await context.addInitScript(() => {
  const zoom = () => { document.documentElement.style.zoom = '1.5'; };
  if (document.documentElement) zoom();
  else new MutationObserver((_, observer) => {
    if (document.documentElement) { zoom(); observer.disconnect(); }
  }).observe(document, { childList: true });

  window.__caption = () => {
    const step = sessionStorage.getItem('caption');
    if (!step || !document.body) return;
    let bar = document.getElementById('demo-caption');
    if (!bar) {
      bar = document.createElement('div');
      bar.id = 'demo-caption';
      bar.style.cssText = 'position:fixed;left:0;right:0;bottom:0;padding:10px 20px;background:#1e1e2e;' +
        'color:#fff;font:16px system-ui,sans-serif;z-index:9999';
      document.body.appendChild(bar);
    }
    const url = decodeURIComponent(location.pathname + location.search);
    bar.innerHTML = '<div style="font-weight:600"></div><div style="font:13px ui-monospace,monospace;color:#bac2de"></div>';
    bar.children[0].textContent = step;
    bar.children[1].textContent = 'http://127.0.0.1:8080' + url;
  };
  // Chrome sends video frames only when the page repaints, so a still page can leave a stale frame in the video.
  // An invisible 2px animation keeps it repainting.
  const tick = () => {
    const style = document.createElement('style');
    style.textContent = '@keyframes demo-tick { to { background: #fefefe } }' +
      '#demo-tick { position: fixed; top: 0; left: 0; width: 2px; height: 2px; background: #fff;' +
      'animation: demo-tick .5s infinite alternate; z-index: 9999 }';
    const dot = document.createElement('div');
    dot.id = 'demo-tick';
    document.head.appendChild(style);
    document.body.appendChild(dot);
  };
  document.addEventListener('DOMContentLoaded', () => { tick(); window.__caption(); });
});

const page = await context.newPage();
const pause = (ms) => page.waitForTimeout(ms);
const caption = (text) => page.evaluate((t) => { sessionStorage.setItem('caption', t); window.__caption(); }, text);
const highlight = (locator) => locator.evaluate((el) => { el.style.outline = '3px solid #f5a623'; el.style.outlineOffset = '2px'; });
const scroll = async (dy, steps) => {
  for (let i = 0; i < steps; i++) { await page.mouse.wheel(0, dy / steps); await pause(30); }
};
const scrollToTop = () => page.evaluate(() => window.scrollTo({ top: 0, behavior: 'smooth' }));

await page.goto(base + '/');
await caption('skill-atlas serve: the start page explains how to open a map');
await pause(4000);

await caption('Open /scan?repo=<github-repository-url> to see the map of a repository');
await pause(2500);
await page.goto(`${base}/scan?repo=${repo}`);
await pause(2500);
await scroll(2400, 80);
await pause(1000);
await scrollToTop();
await pause(1500);

await caption('Filter: only skills whose name or description contains the text are shown');
const filter = page.getByRole('searchbox');
await highlight(filter);
await filter.click();
await filter.pressSequentially('design', { delay: 180 });
await pause(600);
const button = page.getByRole('button', { name: 'Filter' });
await highlight(button);
await pause(500);
await button.click();
await page.waitForURL(/filter=design/);
await pause(4000);

await caption('Group similar skills: skills whose names start with the same word form a group; the filter is kept');
const group = page.getByLabel('Group similar skills');
await highlight(group);
await pause(1200);
await group.click();
await page.waitForURL(/group=on/);
await pause(3000);
await scroll(700, 30);
await pause(2500);
await scrollToTop();
await pause(1000);

await caption('Clear the filter: the whole map, grouped. The URL keeps the choices, so it can be bookmarked or shared');
await highlight(page.getByRole('searchbox'));
await page.getByRole('searchbox').click({ clickCount: 3 });
await pause(500);
await page.keyboard.press('Backspace');
await pause(700);
await page.keyboard.press('Enter');
await page.waitForURL((url) => url.searchParams.get('filter') === '');
await pause(2500);
await scroll(3000, 100);
await pause(1000);
await scrollToTop();
await pause(3000);

// Clicking a star reloads the page, so wait for the button of the new state to appear. The button is looked up within
// scope, e.g. a repository's section of an organization's map, where other repositories can have skills of that name.
const toggleStar = async (action, name, scope = page) => {
  const button = scope.getByRole('button', { name: `${action} ${name}`, exact: true });
  await button.evaluate((el) => el.scrollIntoView({ behavior: 'smooth', block: 'center' }));
  await pause(1500);
  await highlight(button);
  await pause(1000);
  await button.click();
  await scope.getByRole('button', { name: `${action === 'Star' ? 'Unstar' : 'Star'} ${name}`, exact: true }).waitFor();
};

await caption('Stars: ☆ stars a skill. Starred skills are shown first, in the "Starred" group when grouping is on');
await toggleStar('Star', 'playwright');
await pause(3500);

await caption('The other groups stay as they were: gh-address-comments is still in the gh group');
await toggleStar('Star', 'gh-fix-ci');
await pause(2500);
const ghGroup = page.locator('section').filter({ has: page.getByRole('heading', { name: 'gh', exact: true }) });
await ghGroup.evaluate((el) => el.scrollIntoView({ behavior: 'smooth', block: 'center' }));
await pause(1200);
await highlight(ghGroup);
await pause(3500);
await scrollToTop();
await pause(1500);

await caption('Without grouping, starred skills lead the list');
const ungroup = page.getByLabel('Group similar skills');
await highlight(ungroup);
await pause(1200);
await ungroup.click();
await page.waitForURL((url) => !url.searchParams.has('group'));
await pause(4000);

await caption('★ unstars a skill. Stars are saved, so they are kept when the server restarts');
await toggleStar('Unstar', 'playwright');
await pause(3500);

await page.goto(base + '/');
await caption('The start page also explains how to open the map of an organization');
const orgLink = page.getByRole('link', { name: `/scan?org=${org}` });
await highlight(orgLink);
await pause(3500);
await orgLink.click();
await page.waitForURL(/org=/);
await caption(`The map of an organization: a section for each repository with skills (analyzing JetBrains took ${warmSeconds}s before this recording)`);
await pause(3500);
await scroll(4000, 120);
await pause(1000);
await scrollToTop();
await pause(1500);

await caption('Filter and grouping work across all repositories; repositories without matching skills are hidden');
const orgFilter = page.getByRole('searchbox');
await highlight(orgFilter);
await orgFilter.click();
await orgFilter.pressSequentially('gradle', { delay: 180 });
await pause(600);
await page.keyboard.press('Enter');
await page.waitForURL(/filter=gradle/);
await pause(2500);
const orgGroup = page.getByLabel('Group similar skills');
await highlight(orgGroup);
await pause(1200);
await orgGroup.click();
await page.waitForURL(/group=on/);
await pause(3000);
await scroll(1500, 50);
await pause(2500);
await scrollToTop();
await pause(3000);

await caption("Stars work on an organization's map too: each repository's starred skills come first in its section");
const kotlinSection = page.locator('section.repo').filter({ has: page.getByRole('heading', { name: /^JetBrains\/kotlin / }) });
await toggleStar('Star', 'build-bump-gradle-version', kotlinSection);
await pause(2000);
await highlight(kotlinSection.getByRole('heading', { name: 'Starred', exact: true }));
await pause(4000);
await scrollToTop();
await pause(2000);

const video = page.video();
await context.close();
await browser.close();
console.log(await video.path());
