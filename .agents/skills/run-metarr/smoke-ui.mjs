#!/usr/bin/env node
/**
 * Smoke-test the Metarr UI via Playwright.
 * Navigates to localhost:5173, attempts login if credentials are provided,
 * verifies the dashboard renders, and takes a screenshot.
 *
 * Usage: node smoke-ui.mjs [admin-username] [admin-password]
 * Exit code: 0 on success, 1 on failure
 */

import { chromium } from 'playwright';
import fs from 'fs';

const SCREENSHOT_DIR = '/tmp/metarr-ui-screenshots';
if (!fs.existsSync(SCREENSHOT_DIR)) {
  fs.mkdirSync(SCREENSHOT_DIR, { recursive: true });
}

const adminUsername = process.argv[2] || '';
const adminPassword = process.argv[3] || '';
const BASE_URL = 'http://localhost:5173';

async function main() {
  const browser = await chromium.launch();
  const page = await browser.newPage();

  try {
    console.log(`=== Navigating to ${BASE_URL} ===`);
    await page.goto(BASE_URL, { waitUntil: 'load', timeout: 10000 });

    if (adminUsername && adminPassword) {
      console.log('=== Credentials provided, attempting login ===');

      // Wait for the login form to be visible
      await page.waitForSelector('form', { timeout: 5000 });

      // Fill username field (first plain text input in the form)
      const usernameInput = page.locator('form input:first-of-type');
      await usernameInput.fill(adminUsername);
      console.log('Filled username field');

      // Fill password field
      const passwordInput = page.locator('input[type="password"]');
      await passwordInput.fill(adminPassword);
      console.log('Filled password field');

      // Click the submit button
      const submitButton = page.locator('button:has-text("Sign in")');
      await submitButton.click();
      console.log('Clicked Sign in button');

      // Wait for dashboard to load (System page shows a heading "System")
      console.log('=== Waiting for dashboard to render ===');
      await page.waitForSelector('text=System', { timeout: 10000 });
      console.log('Dashboard rendered (found "System" heading)');
    } else {
      console.log('=== No credentials provided, verifying login screen presence ===');
      // Just verify the login form is present
      await page.waitForSelector('form', { timeout: 5000 });
      console.log('Login form found');
    }

    // Take a screenshot
    const screenshotPath = `${SCREENSHOT_DIR}/ui-smoke-${Date.now()}.png`;
    await page.screenshot({ path: screenshotPath });
    console.log(`Screenshot saved to ${screenshotPath}`);
    const latestLink = `${SCREENSHOT_DIR}/latest.png`;
    if (fs.existsSync(latestLink)) {
      fs.unlinkSync(latestLink);
    }
    fs.symlinkSync(screenshotPath, latestLink);
    console.log(`Latest: ${latestLink}`);

    console.log('=== UI smoke test passed ===');
    process.exit(0);
  } catch (error) {
    console.error('ERROR:', error.message);
    const errorScreenshotPath = `${SCREENSHOT_DIR}/error-${Date.now()}.png`;
    try {
      await page.screenshot({ path: errorScreenshotPath });
      console.error(`Error screenshot saved to ${errorScreenshotPath}`);
    } catch {
      // ignore
    }
    process.exit(1);
  } finally {
    await browser.close();
  }
}

main();