import { expect, test } from '@playwright/test'
test('console shell renders', async ({page})=>{
 await page.route('**/api/v1/dashboard', route=>route.fulfill({json:{data:{notifications_24h:0,delivered:0,failed:0,waiting_retry:0,dead_letter:0,ai_fallback_rate:0,deliveries_by_channel:{},recent_failures:[]}}}))
 await page.goto('/')
 await expect(page.getByRole('heading',{name:'Delivery overview'})).toBeVisible()
 await expect(page.getByText('Dispatch')).toBeVisible()
})
