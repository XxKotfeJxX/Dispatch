import { expect, test } from '@playwright/test'
test('console shell renders', async ({page})=>{
 await page.route('**/api/v1/dashboard', route=>route.fulfill({json:{data:{notifications_24h:0,delivered:0,failed:0,waiting_retry:0,dead_letter:0,ai_fallback_rate:0,deliveries_by_channel:{},recent_failures:[]}}}))
 await page.goto('/')
 await expect(page.getByRole('heading',{name:'Delivery overview'})).toBeVisible()
 await expect(page.getByText('Dispatch')).toBeVisible()
})

test('integrations catalog uses branded one-click cards and separates developer tools', async ({page})=>{
 await page.route('**/api/v1/connectors', route=>route.fulfill({json:{data:[
  {id:'demo',name:'Demo source',summary:'Generate a safe sample event.',category:'Testing',auth:'none',transport:'internal',availability:'available',configured:true,capabilities:['sample events'],fields:null,setup_hint:'Ready.'},
  {id:'discord',name:'Discord',summary:'Receive allowlisted bot messages.',category:'Messaging',auth:'app_install',transport:'gateway',availability:'setup_required',configured:false,capabilities:['Gateway'],fields:null,setup_hint:'Set DISCORD_CLIENT_ID first.'},
  {id:'github',name:'GitHub',summary:'Receive repository events.',category:'Development',auth:'app_install',transport:'webhook',availability:'setup_required',configured:false,capabilities:['issues'],fields:null,setup_hint:'Set GITHUB_APP_SLUG first.'},
  {id:'google',name:'Google / Gmail',summary:'Receive Gmail changes.',category:'Productivity',auth:'oauth2',transport:'webhook',availability:'setup_required',configured:false,capabilities:['OAuth 2.0'],fields:null,setup_hint:'Set Google OAuth credentials first.'},
  {id:'youtube',name:'YouTube',summary:'Receive channel updates.',category:'Media',auth:'none',transport:'websub',availability:'available',configured:true,capabilities:['uploads'],fields:[{name:'channel_id',label:'Channel ID',type:'text',required:true,secret:false}],setup_hint:'Ready.'},
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
 await expect(page.getByRole('button',{name:'Connect Discord'})).toBeVisible()
 await expect(page.getByRole('button',{name:'Connect GitHub'})).toBeVisible()
 await expect(page.getByRole('button',{name:'Connect Google / Gmail'})).toBeVisible()
 await expect(page.getByRole('button',{name:'Connect YouTube'})).toBeVisible()
 await expect(page.getByRole('button',{name:/Telegram|Viber|ChatGPT/})).toHaveCount(0)
 await page.getByRole('button',{name:'Connect GitHub'}).hover()
 await expect(page.getByText('GitHub').last()).toBeVisible()
 await page.getByRole('button',{name:'About GitHub'}).hover()
 await expect(page.getByText('Receive repository events.')).toBeVisible()
 await page.getByRole('button',{name:'Connect Discord'}).click()
 await expect(page.getByRole('heading',{name:'Discord'}).last()).toBeVisible()
 await expect(page.getByText(/do not need to enter tokens/)).toBeVisible()
 await page.getByRole('button',{name:'Close availability message'}).click()
 await page.getByRole('button',{name:'Test Dispatch'}).click()
 await expect(page.locator('form').getByRole('heading',{name:'Demo source'})).toBeVisible()
 await expect(page.getByRole('button',{name:'Connect',exact:true})).toBeVisible()
 await page.getByRole('button',{name:'Close'}).click()
 await page.getByRole('button',{name:'Developer tools'}).click()
 await expect(page.getByText('GitHub production')).toBeVisible()
 await expect(page.getByText('/ingest/v1/github-production')).toBeVisible()
 await expect(page.getByRole('option',{name:/discord/})).toBeAttached()
 await expect(page.getByRole('button',{name:'Create source'})).toBeVisible()
})
