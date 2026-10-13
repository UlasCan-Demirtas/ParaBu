import puppeteer from 'puppeteer';

(async () => {
  try {
    const browser = await puppeteer.launch({ headless: 'new' });
    const page = await browser.newPage();
    
    page.on('console', msg => console.log('PAGE LOG:', msg.text()));
    page.on('pageerror', err => console.log('PAGE ERROR:', err.message));
    page.on('requestfailed', request => {
      if (!request.url().includes('favicon')) {
        console.log('REQUEST FAILED:', request.url(), request.failure()?.errorText);
      }
    });

    await page.goto('http://localhost:5174/', { waitUntil: 'domcontentloaded' });
    
    await new Promise(r => setTimeout(r, 5000));
    
    // Check if body is empty or has a specific error
    const bodyHTML = await page.evaluate(() => document.body.innerHTML);
    if (bodyHTML.includes('id="root"')) {
        const rootHTML = await page.evaluate(() => document.getElementById('root').innerHTML);
        console.log('ROOT HTML LENGTH:', rootHTML.length);
        if (rootHTML.length < 50) {
            console.log('ROOT HTML CONTENT:', rootHTML);
        }
    }

    await browser.close();
  } catch (err) {
    console.error("Puppeteer Script Error:", err);
  }
})();
