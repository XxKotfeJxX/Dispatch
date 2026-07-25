import { expect, test } from '@playwright/test'
test('console shell renders', async ({page})=>{
 await page.route('**/api/v1/dashboard', route=>route.fulfill({json:{data:{notifications_24h:0,delivered:0,failed:0,waiting_retry:0,dead_letter:0,ai_fallback_rate:0,deliveries_by_channel:{},recent_failures:[]}}}))
 await page.goto('/')
 await expect(page.getByRole('heading',{name:'Delivery overview'})).toBeVisible()
 await expect(page.getByText('Dispatch')).toBeVisible()
})

test('integrations catalog exposes one-click and advanced connector flows', async ({page})=>{
 await page.route('**/api/v1/connectors', route=>route.fulfill({json:{data:[
  {id:'demo',name:'Demo source',summary:'Generate a safe sample event.',category:'Testing',auth:'none',transport:'internal',availability:'available',configured:true,capabilities:['sample events'],fields:[],setup_hint:'Ready.'},
  {id:'telegram',name:'Telegram',summary:'Receive bot messages.',category:'Messaging',auth:'bot_token',transport:'webhook',availability:'available',configured:false,capabilities:['bot messages'],fields:[{name:'bot_token',label:'Bot token',type:'password',required:true,secret:true}],setup_hint:'Public HTTPS required.'},
  {id:'webhook',name:'Universal webhook',summary:'Any JSON producer.',category:'Advanced',auth:'api_key',transport:'webhook',availability:'available',configured:true,capabilities:['HMAC'],fields:[],setup_hint:'Advanced fallback.'},
 ],connections:[]}}))
 await page.route('**/api/v1/sources', route=>route.fulfill({json:{data:[{
  id:'src_demo',name:'GitHub production',slug:'github-production',provider:'github',
  recipient_id:'rec_demo',auth_mode:'hmac_sha256',signature_header:'X-Hub-Signature-256',
  mapping:{id_path:'hook.id',event_type_path:'action',subject_path:'repository.full_name',
   body_path:'head_commit.message',default_event_type:'github.event',
   default_subject:'GitHub event',requested_channels:['email']},
  enabled:true,created_at:'2026-07-25T00:00:00Z',
 }],providers:['generic','github','gitlab','discord','slack','stripe','sentry','grafana']}}))
 await page.route('**/api/v1/recipients', route=>route.fulfill({json:{data:[{
  id:'rec_demo',name:'Operations',email:'ops@example.test',
  preferences:{default_channels:['email']},
 }]}}))
 await page.goto('/integrations')
 await expect(page.getByRole('heading',{name:'Integrations'})).toBeVisible()
 await expect(page.getByText('Demo source')).toBeVisible()
 await page.getByRole('button',{name:'Connect'}).first().click()
 await expect(page.locator('form').getByRole('heading',{name:'Demo source'})).toBeVisible()
 await expect(page.getByRole('button',{name:'Verify and connect'})).toBeVisible()
 await page.getByRole('button',{name:'Close'}).click()
 await page.getByRole('button',{name:'Advanced webhooks'}).click()
 await expect(page.getByText('GitHub production')).toBeVisible()
 await expect(page.getByText('/ingest/v1/github-production')).toBeVisible()
 await page.getByRole('button',{name:'New source'}).click()
 await expect(page.getByRole('option',{name:/discord/})).toBeAttached()
 await expect(page.getByRole('button',{name:'Create source'})).toBeVisible()
})
