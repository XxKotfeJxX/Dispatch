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
  {id:'telegram',name:'Telegram',summary:'Connect your personal Telegram account.',category:'Messaging',auth:'user_session',transport:'gateway',availability:'setup_required',configured:true,capabilities:['private chats','groups'],fields:[{name:'phone_number',label:'Phone number',type:'tel',required:true,secret:false,placeholder:'+380…'}],setup_hint:'Ready.'},
  {id:'discord',name:'Discord',summary:'Receive allowlisted bot messages.',category:'Messaging',auth:'app_install',transport:'gateway',availability:'setup_required',configured:false,capabilities:['Gateway'],fields:null,setup_hint:'Set DISCORD_CLIENT_ID first.'},
  {id:'github',name:'GitHub',summary:'Receive repository events.',category:'Development',auth:'app_install',transport:'webhook',availability:'setup_required',configured:false,capabilities:['issues'],fields:null,setup_hint:'Set GITHUB_APP_SLUG first.'},
  {id:'google',name:'Google',summary:'Connect Google services.',category:'Productivity',auth:'oauth2',transport:'polling',availability:'setup_required',configured:false,capabilities:['OAuth 2.0'],fields:null,setup_hint:'Set Google OAuth credentials first.'},
  {id:'youtube',name:'YouTube',summary:'Receive subscription uploads.',category:'Media',auth:'oauth2',transport:'polling',availability:'available',configured:true,capabilities:['subscriptions','uploads'],fields:[],setup_hint:'Ready.'},
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
 await page.route('**/api/v1/connectors/telegram/auth/start', route=>route.fulfill({json:{
  auth_id:'tg_auth_test',step:'code',message:'Enter the code Telegram sent.',
 }}))
 await page.goto('/integrations')
 await expect(page.getByRole('heading',{name:'Integrations'})).toBeVisible()
 await expect(page.getByRole('button',{name:'Connect Discord'})).toBeVisible()
 await expect(page.getByRole('button',{name:'Connect GitHub'})).toBeVisible()
 await expect(page.getByRole('button',{name:'Connect Google'})).toBeVisible()
 await expect(page.getByRole('button',{name:'Connect YouTube'})).toBeVisible()
 await expect(page.getByRole('button',{name:'Connect Telegram'})).toBeVisible()
 await expect(page.getByRole('button',{name:/Viber|ChatGPT/})).toHaveCount(0)
 await page.getByRole('button',{name:'Connect Telegram'}).click()
 await page.getByLabel('Phone number').fill('+380501234567')
 await page.getByRole('button',{name:'Send sign-in code'}).click()
 await expect(page.getByLabel('Telegram sign-in code')).toBeVisible()
 await page.getByRole('button',{name:'Close'}).click()
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
 await expect(page.getByRole('button',{name:'Cancel'})).toBeVisible()
 await page.getByRole('button',{name:'Cancel'}).click()
 await expect(page.getByRole('button',{name:'New source'})).toBeVisible()
 await page.getByRole('button',{name:'New source'}).click()
 await expect(page.getByRole('button',{name:'Create source'})).toBeVisible()
})

test('template editor exposes guided variables, preview and conditions', async ({page})=>{
 let created: Record<string, unknown> | undefined
 await page.route('**/api/v1/templates', async route=>{
  if(route.request().method()==='POST'){
   created=route.request().postDataJSON()
   await route.fulfill({status:201,json:{data:{id:'tpl_test',...created}}})
   return
  }
  await route.fulfill({json:{data:[]}})
 })
 await page.goto('/templates')
 await expect(page.getByRole('heading',{name:'Templates',exact:true})).toBeVisible()
 await page.getByRole('button',{name:'New template'}).click()
 await page.getByPlaceholder('For example: Important GitHub activity').fill('GitHub review')
 await page.getByLabel('Apply to service').selectOption('github')
 const message=page.getByLabel('Notification message')
 await message.focus()
 await page.getByRole('button',{name:'Repository'}).click()
 await expect(message).toHaveValue(/{{repository}}/)
 await page.getByRole('button',{name:'Add condition'}).click()
 await page.getByPlaceholder('Value to match').fill('octocat')
 await page.getByRole('button',{name:'Create template'}).click()
 expect(created).toMatchObject({
  name:'GitHub review',
  service:'github',
  conditions:[{field:'sender',operator:'contains',value:'octocat'}],
 })
 await expect(page.getByRole('button',{name:'New template'})).toBeVisible()
})

test('recipient setup guides email, Telegram, Mailpit and webhook destinations', async ({page})=>{
 let created: Record<string, unknown> | undefined
 await page.route('**/api/v1/recipients', async route=>{
  if(route.request().method()==='POST'){
   created=route.request().postDataJSON()
   await route.fulfill({status:201,json:{data:{id:'rec_test',...created}}})
   return
  }
  await route.fulfill({json:{data:[]}})
 })
 await page.route('**/api/v1/recipient-setups/mailpit', route=>route.fulfill({status:201,json:{
  data:{id:'rst_mailpit',kind:'mailpit',name:'Local inbox',target:'alex@dispatch.local',
   status:'pending',expires_at:'2026-07-26T20:00:00Z'},
  mailpit_url:'http://localhost:8025',
 }}))
 await page.goto('/recipients')
 await page.getByRole('button',{name:'Add destination'}).click()
 const destination=page.getByLabel('Where should notifications arrive?')
 await expect(destination.getByRole('option',{name:'Email / Gmail'})).toBeAttached()
 await expect(destination.getByRole('option',{name:'Telegram'})).toBeAttached()
 await expect(destination.getByRole('option',{name:'Dispatch Mailpit'})).toBeAttached()
 await expect(destination.getByRole('option',{name:'Other service'})).toBeAttached()
 await page.getByLabel('Destination name').fill('Primary email')
 await page.getByLabel('Email address').fill('alex@example.com')
 await page.getByRole('button',{name:'Add destination',exact:true}).click()
 expect(created).toMatchObject({
  name:'Primary email',destination_type:'email',email:'alex@example.com',
  preferences:{default_channels:['email']},
 })
 await page.getByRole('button',{name:'Add destination'}).click()
 await page.getByLabel('Destination name').fill('Telegram alerts')
 await destination.selectOption('telegram')
 await expect(page.getByText(/phone number and username are not requested/i)).toBeVisible()
 await expect(page.getByRole('button',{name:'Connect Telegram'})).toBeVisible()
 await destination.selectOption('mailpit')
 await page.getByLabel('Destination name').fill('Local inbox')
 await page.getByLabel('Mailpit mailbox name').fill('alex')
 await page.getByRole('button',{name:'Send verification code'}).click()
 await expect(page.getByLabel('Mailpit verification code')).toBeVisible()
 await expect(page.getByRole('link',{name:/Open Mailpit/})).toBeVisible()
})

test('notifications search keeps icon clearance and has no manual composer', async ({page})=>{
 await page.route('**/api/v1/notifications?limit=100', route=>route.fulfill({json:{
  data:[{id:'not_test',idempotency_key:'test',recipient_id:'rec_test',
   event_type:'telegram.message',subject:'Telegram message',body:'Hello',
   priority:'normal',status:'delivered',requested_channels:[],created_at:'2026-07-26T19:00:00Z'}],
  total:1,
 }}))
 await page.goto('/notifications')
 const search=page.getByPlaceholder('Search notifications…')
 await expect(search).toBeVisible()
 await expect(search).toHaveCSS('padding-left','40px')
 await expect(page.getByRole('button',{name:'Compose'})).toHaveCount(0)
})
