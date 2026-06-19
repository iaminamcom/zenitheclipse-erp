const $ = (sel, root=document) => root.querySelector(sel);
const $$ = (sel, root=document) => Array.from(root.querySelectorAll(sel));

let appData = null;
let currentModule = 'dashboard';
let activeTabs = {};
let editing = {};

const navItems = [
  ['dashboard','Dashboard','Owner control center'],
  ['master','Master Data','Customers, suppliers, products, accounts'],
  ['sales','Sales','Leads, RFQ, quotations, proformas, orders'],
  ['accounting','Accounting','Vouchers, journal, receivable/payable'],
  ['transport','Transport','Shipments, trucks, containers, BL, delivery notes'],
  ['documents','Documents','Serial numbers, print/PDF, QR verification'],
  ['adminLegal','Admin & Legal','Employees, contracts, tasks, compliance'],
  ['approvals','Approvals & Audit','Control, permissions, immutable history'],
  ['reports','Reports','Management reports and CSV exports'],
  ['ai','AI Assistant','Offline rule-based assistant MVP'],
  ['settings','Settings','Company, backup, password']
];

const currencies = ['AED','USD','EUR','OMR','GBP','CNY','AFN','RUB','IRR'];
const statuses = ['Draft','Pending Approval','Approved','Active','Open','Closed','Paid','Partially Paid','Cancelled','Rejected','Completed'];
const approvalStatuses = ['Not Requested','Pending','Approved','Rejected'];
const riskLevels = ['Low','Medium','High'];
const yesNo = ['No','Yes'];

const schemas = {
  contacts: {
    title:'Customers & Suppliers', desc:'KYC/KYB, balances, credit limit, risk, customer/supplier master records.',
    fields:[
      f('type','Type','select',['Customer','Supplier','Both'],true), f('name','Name','text',null,true), f('email','Email'), f('phone','Phone'),
      f('country','Country'), f('address','Address','textarea'), f('currency','Currency','select',currencies), f('creditLimit','Credit Limit','number'),
      f('balance','Opening Balance','number'), f('kycStatus','KYC/KYB Status','select',['Pending','Approved','Rejected','Expired']), f('riskLevel','Risk Level','select',riskLevels), f('status','Status','select',['Active','Inactive','Blocked','Draft']), f('notes','Notes','textarea')
    ], columns:['number','type','name','country','currency','creditLimit','balance','kycStatus','riskLevel','status']
  },
  products: {
    title:'Products / Services / Transport Price Items', desc:'Product, cargo, service, HS code, unit, price list base.',
    fields:[f('name','Name','text',null,true),f('category','Category','select',['Product','Service','Transport','Port Charge','Customs','Other']),f('hsCode','HS Code'),f('unit','Unit'),f('cost','Cost','number'),f('salePrice','Sale Price','number'),f('currency','Currency','select',currencies),f('stockQty','Stock Qty','number'),f('status','Status','select',['Active','Inactive']),f('notes','Notes','textarea')], columns:['number','name','category','hsCode','unit','cost','salePrice','currency','status']
  },
  accounts: {
    title:'Chart of Accounts', desc:'Basic GL account list for cash, bank, receivables, payables, income, expense.',
    fields:[f('name','Account Name','text',null,true),f('type','Account Type','select',['Asset','Liability','Equity','Income','Expense']),f('currency','Currency','select',currencies),f('openingBalance','Opening Balance','number'),f('status','Status','select',['Active','Inactive']),f('notes','Notes','textarea')], columns:['number','name','type','currency','openingBalance','status']
  },
  leads: {
    title:'Leads & RFQ', desc:'Sales pipeline, RFQ, customer follow-up and expected deal profit.',
    fields:[f('customerName','Customer / Lead Name','text',null,true),f('contactPerson','Contact Person'),f('email','Email'),f('phone','Phone'),f('route','Route'),f('cargo','Cargo'),f('expectedAmount','Expected Amount','number'),f('expectedCost','Expected Cost','number'),f('followUpDate','Follow-up Date','date'),f('status','Status','select',['New','RFQ Received','Quoted','Won','Lost']),f('notes','Notes','textarea')], columns:['number','customerName','route','cargo','expectedAmount','expectedCost','followUpDate','status']
  },
  quotations: docSchema('Quotations','RFQ to quotation with automatic job reference and approval control.'),
  proformas: docSchema('Proforma Invoices','Convert accepted quotation to proforma invoice.'),
  invoices: docSchema('Commercial Invoices','Commercial invoices linked to job reference, accounting and delivery.'),
  packingLists: {
    title:'Packing Lists', desc:'Packing list document linked to job reference and invoice.',
    fields:[common('jobRef'),common('customerName'),common('supplierName'),f('cargoDescription','Cargo Description','textarea'),f('packages','Packages'),f('grossWeight','Gross Weight'),f('netWeight','Net Weight'),f('containerNo','Container No'),f('sealNo','Seal No'),common('currency'),common('amount'),f('status','Status','select',statuses),common('approvalStatus'),f('notes','Notes','textarea')], columns:['number','jobRef','customerName','packages','grossWeight','containerNo','sealNo','status']
  },
  shipments: {
    title:'Shipments / Transport Orders', desc:'Shipment booking, truck/container tracking, route, cost and live status.',
    fields:[common('jobRef'),common('customerName'),common('supplierName'),f('shipmentStatus','Shipment Status','select',['Booked','Port Entry','Loaded','In Transit','Border','Customs','Delivered','Closed']),f('origin','Origin'),f('destination','Destination'),f('pol','POL'),f('pod','POD'),f('fpod','FPOD'),f('containerNo','Container No'),f('sealNo','Seal No'),f('truckNo','Truck No'),f('trailerNo','Trailer No'),f('driverName','Driver Name'),f('driverMobile','Driver Mobile'),f('blNo','BL No'),f('doNo','DO No'),f('bayanNo','Bayan / Customs No'),f('route','Route'),f('eta','ETA','date'),f('etd','ETD','date'),common('currency'),common('amount'),common('cost'),common('profit'),f('detentionRisk','Detention/Demurrage Risk','select',['No','Watch','High']),f('status','Status','select',statuses),f('notes','Notes','textarea')], columns:['number','jobRef','customerName','shipmentStatus','origin','destination','containerNo','truckNo','driverName','amount','cost','profit','status']
  },
  bls: {
    title:'Bill of Lading Module', desc:'BL draft, shipper/consignee, vessel, container, seal, amendments and approval.',
    fields:[common('jobRef'),f('shipper','Shipper'),f('consignee','Consignee'),f('notifyParty','Notify Party'),f('vesselVoyage','Vessel / Voyage'),f('pol','POL'),f('pod','POD'),f('fpod','FPOD'),f('containerNo','Container No'),f('sealNo','Seal No'),f('cargoDescription','Cargo Description','textarea'),f('grossWeight','Gross Weight'),f('netWeight','Net Weight'),f('hsCode','HS Code'),f('freightTerms','Freight Terms','select',['Prepaid','Collect','Third Party']),f('releaseType','Release Type','select',['Original BL','Seaway BL','Telex Release','eBL']),f('surrenderStatus','Surrender Status','select',['Not Surrendered','Surrendered','N/A']),f('amendmentRecord','BL Amendment Record','textarea'),f('carrierCommunication','Carrier / Agent Communication','textarea'),f('riskLevel','Risk Level','select',riskLevels),f('status','Status','select',['Draft','Pending Approval','Approved','Released','Surrendered','Cancelled']),common('approvalStatus'),f('notes','Notes','textarea')], columns:['number','jobRef','shipper','consignee','vesselVoyage','pol','pod','containerNo','sealNo','releaseType','surrenderStatus','approvalStatus','status']
  },
  deliveryNotes: {
    title:'Delivery Notes / Proof of Delivery', desc:'Driver, truck, GPS, receiver signature, photos note and automatic invoice trigger.',
    fields:[common('jobRef'),common('customerName'),f('truckNo','Truck No'),f('driverName','Driver Name'),f('driverMobile','Driver Mobile'),f('containerNo','Container No'),f('sealNo','Seal No'),f('productDescription','Product Description','textarea'),f('quantity','Quantity','number'),f('weight','Weight'),f('loadingLocation','Loading Location'),f('deliveryLocation','Delivery Location'),f('deliveryAt','Delivery Date & Time','datetime-local'),f('receiverName','Receiver Name'),f('receiverSignature','Receiver Signature / Name'),f('gpsLocation','GPS Location'),f('deliveryPhotos','Photo References / Notes','textarea'),f('proofStatus','Proof Status','select',['Pending Delivery','Delivered','Receiver Signed','Invoice Triggered','Issue Reported']),f('autoInvoiceTriggered','Auto Invoice Triggered','select',yesNo),f('status','Status','select',statuses),common('approvalStatus'),f('notes','Notes','textarea')], columns:['number','jobRef','customerName','truckNo','driverName','containerNo','sealNo','receiverName','proofStatus','autoInvoiceTriggered','status']
  },
  vouchers: {
    title:'Payment / Receipt / Transfer Vouchers', desc:'Cash, bank, receivable, payable, supplier and customer voucher control.',
    fields:[f('voucherType','Voucher Type','select',['Payment Voucher','Receipt Voucher','Internal Transfer Voucher'],true),f('date','Date','date'),f('accountName','Account'),f('partyName','Customer / Supplier / Employee'),common('currency'),common('amount'),f('paymentMode','Payment Mode','select',['Cash','Bank Transfer','Cheque','Card','Other']),f('bankName','Bank Name'),f('reference','Bank / Cheque Reference'),f('status','Status','select',statuses),common('approvalStatus'),f('notes','Notes','textarea')], columns:['number','voucherType','date','accountName','partyName','currency','amount','paymentMode','approvalStatus','status']
  },
  journals: {
    title:'Journal Entries', desc:'Manual GL posting MVP for accounting control and audit.',
    fields:[f('date','Date','date'),f('description','Description','textarea'),f('debitAccount','Debit Account'),f('creditAccount','Credit Account'),common('currency'),common('amount'),f('reference','Reference'),f('status','Status','select',statuses),f('notes','Notes','textarea')], columns:['number','date','description','debitAccount','creditAccount','currency','amount','reference','status']
  },
  contracts: {
    title:'Contracts & Legal Files', desc:'Customer/supplier/agency/transport contracts, expiry reminder, version and risk flag.',
    fields:[f('contractType','Contract Type','select',['Customer Contract','Supplier Contract','Agency Agreement','Transport Agreement','Employment','Power of Attorney','Legal Case','Other']),f('partyName','Party Name'),f('title','Title','text',null,true),f('startDate','Start Date','date'),f('endDate','End / Expiry Date','date'),f('version','Version'),f('riskLevel','Risk Level','select',riskLevels),f('esignatureStatus','E-signature Status','select',['Not Sent','Sent','Signed','Rejected','N/A']),f('status','Status','select',['Draft','Pending Approval','Approved','Active','Expired','Cancelled']),common('approvalStatus'),f('notes','Notes','textarea')], columns:['number','contractType','partyName','title','startDate','endDate','riskLevel','esignatureStatus','approvalStatus','status']
  },
  employees: {
    title:'Employees / HR', desc:'Employee files, department, role, document expiry and status.',
    fields:[f('name','Employee Name','text',null,true),f('department','Department','select',['Management','Accounting','Admin','Legal','Transport','Sales','Operations','Warehouse','Driver']),f('role','Role'),f('email','Email'),f('mobile','Mobile'),f('branch','Branch'),f('documentsExpiry','Main Document Expiry','date'),f('status','Status','select',['Active','On Leave','Inactive','Terminated']),f('notes','Notes','textarea')], columns:['number','name','department','role','mobile','branch','documentsExpiry','status']
  },
  tasks: {
    title:'Tasks & Internal Notices', desc:'Department tasks, due dates, priority and performance tracking.',
    fields:[f('title','Task / Notice Title','text',null,true),f('department','Department','select',['Management','Accounting','Admin','Legal','Transport','Sales','Operations','Warehouse']),f('assignedTo','Assigned To'),f('dueDate','Due Date','date'),f('priority','Priority','select',['Low','Medium','High','Urgent']),f('status','Status','select',['Open','In Progress','Waiting','Completed','Cancelled']),f('notes','Notes','textarea')], columns:['number','title','department','assignedTo','dueDate','priority','status']
  },
  documents: {
    title:'General Documents', desc:'Letters, gate pass, loading order, statements, authorization and company letterhead documents.',
    fields:[f('docType','Document Type','select',['Transport Order','Loading Order','Gate Pass Request','Statement of Account','Contract','Employment Letter','Authorization Letter','Company Letterhead','Other']),common('jobRef'),f('title','Title','text',null,true),common('customerName'),f('language','Language','select',['English','Arabic','Russian','Persian','Chinese']),f('status','Status','select',statuses),common('approvalStatus'),f('notes','Document Body / Notes','textarea')], columns:['number','docType','jobRef','title','customerName','language','approvalStatus','status']
  },
  priceLists: {
    title:'Price Lists & Commission', desc:'Sales and transport price list plus commission calculation fields.',
    fields:[f('name','Price Name','text',null,true),f('route','Route'),f('productService','Product / Service'),common('currency'),f('cost','Cost','number'),f('salePrice','Sale Price','number'),f('commissionPercent','Commission %','number'),f('validFrom','Valid From','date'),f('validTo','Valid To','date'),f('status','Status','select',['Active','Expired','Draft']),f('notes','Notes','textarea')], columns:['number','name','route','productService','currency','cost','salePrice','commissionPercent','validFrom','validTo','status']
  },
  compliance: {
    title:'Compliance / KYC / KYB', desc:'Beneficial owner, sanctions, source of funds, risk and compliance approval records.',
    fields:[f('partyName','Customer / Company Name','text',null,true),f('partyType','Party Type','select',['Customer','Supplier','Agent','Bank','Other']),f('beneficialOwner','Beneficial Owner'),f('countryRisk','Country Risk','select',['Low','Medium','High','Prohibited']),f('sanctionsScreening','Sanctions Screening','select',['Not Checked','Clear','Potential Match','Blocked']),f('sourceOfFunds','Source of Funds'),f('sourceOfWealth','Source of Wealth'),f('transactionPurpose','Transaction Purpose'),f('approvalRequired','Approval Required','select',yesNo),f('status','Status','select',['Draft','Pending Review','Approved','Rejected','Expired']),common('approvalStatus'),f('notes','Notes','textarea')], columns:['number','partyName','partyType','beneficialOwner','countryRisk','sanctionsScreening','approvalRequired','approvalStatus','status']
  }
};

function f(name,label,type='text',options=null,required=false){ return {name,label,type,options,required}; }
function common(name){
  const map={jobRef:f('jobRef','Job Reference'),customerName:f('customerName','Customer Name'),supplierName:f('supplierName','Supplier Name'),currency:f('currency','Currency','select',currencies),amount:f('amount','Amount / Revenue','number'),cost:f('cost','Cost','number'),profit:f('profit','Profit','number'),approvalStatus:f('approvalStatus','Approval Status','select',approvalStatuses)};
  return map[name];
}
function docSchema(title,desc){
  return {title,desc,fields:[common('jobRef'),common('customerName'),common('supplierName'),f('validUntil','Valid Until','date'),common('currency'),common('amount'),common('cost'),common('profit'),f('taxVat','Tax / VAT','number'),f('paymentTerms','Payment Terms'),f('deliveryTerms','Delivery Terms'),f('linesText','Line Items: description | qty | unit | price','textarea'),f('status','Status','select',statuses),common('approvalStatus'),f('notes','Notes','textarea')], columns:['number','jobRef','customerName','supplierName','currency','amount','cost','profit','taxVat','approvalStatus','status']};
}

const groups = {
  master:['contacts','products','accounts'],
  sales:['leads','quotations','proformas','invoices','packingLists','priceLists'],
  accounting:['vouchers','journals','accounts','contacts'],
  transport:['shipments','bls','deliveryNotes'],
  documents:['documents','packingLists','vouchers'],
  adminLegal:['employees','contracts','tasks','compliance']
};

function api(path, opts={}){
  return fetch(path, {credentials:'same-origin', headers:{'Content-Type':'application/json', ...(opts.headers||{})}, ...opts}).then(async r=>{
    if(!r.ok){ throw new Error(await r.text() || r.statusText); }
    const ct = r.headers.get('content-type') || '';
    return ct.includes('json') ? r.json() : r.text();
  });
}

function money(n,c='AED'){ return `${c} ${Number(n||0).toLocaleString(undefined,{maximumFractionDigits:2})}`; }
function num(v){ const n = Number(String(v??'').replace(/,/g,'')); return Number.isFinite(n)?n:0; }
function esc(s){ return String(s??'').replace(/[&<>'"]/g,m=>({'&':'&amp;','<':'&lt;','>':'&gt;',"'":'&#039;','"':'&quot;'}[m])); }
function dateOnly(v){ return v ? String(v).slice(0,10) : ''; }
function label(s){ return String(s).replace(/([A-Z])/g,' $1').replace(/^./,m=>m.toUpperCase()); }
function toast(msg){ const t=$('#toast'); t.textContent=msg; t.classList.remove('hidden'); setTimeout(()=>t.classList.add('hidden'),2600); }
function records(entity){ return (appData?.entities?.[entity] || []); }
function statusBadge(v){ const s=String(v||'Draft'); let cls=''; if(/approved|active|paid|closed|completed|delivered/i.test(s)) cls='ok'; if(/pending|draft|watch/i.test(s)) cls='warn'; if(/cancel|reject|blocked|high|issue/i.test(s)) cls='bad'; return `<span class="badge ${cls}">${esc(s)}</span>`; }

async function boot(){
  bindLogin();
  bindShell();
  try { await loadData(); showApp(); } catch(e) { showLogin(); }
}
function bindLogin(){
  $('#loginForm').addEventListener('submit', async ev=>{
    ev.preventDefault(); $('#loginError').textContent='';
    try{ await api('/api/login',{method:'POST',body:JSON.stringify({username:$('#loginUsername').value,password:$('#loginPassword').value})}); await loadData(); showApp(); toast('Logged in'); }
    catch(e){ $('#loginError').textContent=e.message; }
  });
}
function bindShell(){
  $('#logoutBtn').onclick = async ()=>{ try{ await api('/api/logout',{method:'POST',body:'{}'});}catch{} location.reload(); };
  $('#backupBtn').onclick = ()=>{ window.open('/api/backup','_blank'); };
}
async function loadData(){ appData = await api('/api/data'); }
function showLogin(){ $('#loginView').classList.remove('hidden'); $('#appView').classList.add('hidden'); }
function showApp(){ $('#loginView').classList.add('hidden'); $('#appView').classList.remove('hidden'); renderNav(); render(); }
function renderNav(){
  $('#nav').innerHTML = navItems.map(([id,name])=>`<button data-nav="${id}" class="${id===currentModule?'active':''}"><span>${name}</span>${navCount(id)}</button>`).join('');
  $$('[data-nav]').forEach(b=>b.onclick=()=>{currentModule=b.dataset.nav; renderNav(); render();});
}
function navCount(id){
  if(id==='approvals'){ const n=records('approvals').filter(x=>x.status==='Pending').length; return n?`<span class="badge warn">${n}</span>`:''; }
  const ents=groups[id]||[]; const n=ents.reduce((a,e)=>a+records(e).length,0); return n?`<span class="badge">${n}</span>`:'';
}
function setTitle(title,sub){ $('#pageTitle').textContent=title; $('#pageSubtitle').textContent=sub||''; $('#companyName').textContent=appData?.settings?.companyName||''; $('#currentUser').textContent=appData?.currentUser?.name||appData?.currentUser?.username||''; }
function render(){
  renderNav();
  const map={dashboard:renderDashboard,master:()=>renderGroup('master','Master Data','Customers, suppliers, products and chart of accounts'),sales:()=>renderGroup('sales','Sales Department','Leads, quotations, proformas, invoices, packing lists and price controls'),accounting:()=>renderGroup('accounting','Accounting Department','Vouchers, journal entries, accounts, balances and statements'),transport:()=>renderGroup('transport','Transport Department','Shipments, containers, BL and delivery notes'),documents:()=>renderGroup('documents','Document Engine','Serial-number documents, PDF/print and verification'),adminLegal:()=>renderGroup('adminLegal','Admin & Legal','Employees, contracts, tasks and compliance'),approvals:renderApprovals,reports:renderReports,ai:renderAI,settings:renderSettings};
  (map[currentModule]||renderDashboard)();
}

function dashboardStats(){
  const inv=records('invoices'), vch=records('vouchers'), sh=records('shipments'), dn=records('deliveryNotes'), bl=records('bls');
  const receipts=vch.filter(v=>/receipt/i.test(v.voucherType)).reduce((s,v)=>s+num(v.amount),0);
  const payments=vch.filter(v=>/payment/i.test(v.voucherType)).reduce((s,v)=>s+num(v.amount),0);
  const invoiceTotal=inv.filter(x=>x.status!=='Cancelled').reduce((s,v)=>s+num(v.amount)+num(v.taxVat),0);
  const profit=[...inv,...sh].reduce((s,v)=>s+num(v.profit),0);
  return {invoiceTotal, receipts, payments, cash:receipts-payments, receivables:invoiceTotal-receipts, payables:payments, profit, openShipments:sh.filter(x=>!/closed|cancelled/i.test(x.status||'')).length, pendingBL:bl.filter(x=>/pending|draft/i.test((x.approvalStatus||'')+(x.status||''))).length, pendingDN:dn.filter(x=>/pending/i.test((x.proofStatus||'')+(x.status||''))).length, pendingApprovals:records('approvals').filter(x=>x.status==='Pending').length};
}
function renderDashboard(){
  setTitle('Management Dashboard','Cash, bank, receivables, shipments, BL, delivery notes, approvals and profit overview');
  const s=dashboardStats(); const c=appData.settings?.baseCurrency||'AED';
  $('#content').innerHTML = `
    <div class="grid cols-4">
      ${kpi('Cash Balance',money(s.cash,c))}${kpi('Receivables',money(s.receivables,c))}${kpi('Total Invoices',money(s.invoiceTotal,c))}${kpi('Profit',money(s.profit,c))}
      ${kpi('Open Shipments',s.openShipments)}${kpi('Pending BL',s.pendingBL)}${kpi('Pending Delivery Notes',s.pendingDN)}${kpi('Pending Approvals',s.pendingApprovals)}
    </div>
    <div class="grid cols-2" style="margin-top:18px">
      <div class="card"><h2>Pending Approvals</h2>${miniTable('approvals',['number','entity','referenceNo','requestedBy','reason','status'],records('approvals').filter(x=>x.status==='Pending').slice(0,8))}</div>
      <div class="card"><h2>Live Shipment Dashboard</h2>${miniTable('shipments',['number','jobRef','customerName','shipmentStatus','containerNo','truckNo','detentionRisk'],records('shipments').slice(-8).reverse())}</div>
    </div>
    <div class="grid cols-3" style="margin-top:18px">
      <div class="card"><h3>Document Control</h3><div class="pillbox">${['quotations','proformas','invoices','packingLists','bls','deliveryNotes','vouchers'].map(e=>`<span class="pill">${label(e)}: ${records(e).length}</span>`).join('')}</div></div>
      <div class="card"><h3>Expiry / Risk Alerts</h3>${riskAlerts()}</div>
      <div class="card"><h3>System Notes</h3><p class="note">This is a local MVP. It records data, audit history, serial numbers, approvals and printable documents. Production deployment needs server database, MFA, backups and formal testing.</p></div>
    </div>`;
}
function kpi(name,val){ return `<div class="kpi"><small>${esc(name)}</small><b>${esc(val)}</b></div>`; }
function miniTable(entity,cols,list){ if(!list.length) return '<div class="empty">No records yet.</div>'; return `<div class="table-wrap"><table><thead><tr>${cols.map(c=>`<th>${label(c)}</th>`).join('')}</tr></thead><tbody>${list.map(r=>`<tr>${cols.map(c=>`<td>${c==='status'||c==='approvalStatus'||c==='detentionRisk'?statusBadge(r[c]):esc(r[c]??'')}</td>`).join('')}</tr>`).join('')}</tbody></table></div>`; }
function riskAlerts(){
  const alerts=[]; const today=new Date(); const in45=new Date(Date.now()+45*864e5);
  for(const e of ['contracts','employees']) for(const r of records(e)){ const d=r.endDate||r.documentsExpiry; if(d){ const dt=new Date(d); if(dt<=in45) alerts.push(`${r.number||''} ${r.title||r.name||''} expires ${dateOnly(d)}`); } if(r.riskLevel==='High') alerts.push(`${r.number||''} ${r.title||r.name||''} high risk`); }
  for(const r of records('shipments')) if(r.detentionRisk==='High') alerts.push(`${r.number} detention/demurrage high risk`);
  return alerts.length?alerts.slice(0,8).map(a=>`<p class="report-line"><span>${esc(a)}</span></p>`).join(''):'<div class="empty">No urgent alerts.</div>';
}

function renderGroup(group,title,sub){
  const ents=groups[group]; activeTabs[group]=activeTabs[group]||ents[0];
  setTitle(title,sub);
  $('#content').innerHTML = `<div class="tabs">${ents.map(e=>`<button data-tab="${e}" class="${activeTabs[group]===e?'active':''}">${schemas[e]?.title||label(e)}</button>`).join('')}</div><div id="entityHost"></div>`;
  $$('[data-tab]').forEach(b=>b.onclick=()=>{activeTabs[group]=b.dataset.tab; renderGroup(group,title,sub);});
  renderEntity(activeTabs[group], $('#entityHost'));
}

function renderEntity(entity, host){
  const schema=schemas[entity]; const list=records(entity).slice().reverse(); const edit=editing[entity]||{};
  host.innerHTML = `<div class="split">
    <div class="card"><h2>${schema.title}</h2><p class="muted">${schema.desc}</p>${entityForm(entity,schema,edit)}</div>
    <div class="card"><div class="toolbar"><input placeholder="Search ${schema.title}" id="search_${entity}"><button class="light" data-new="${entity}">New</button><button class="light" data-csv="${entity}">Export CSV</button></div><div id="table_${entity}"></div></div>
  </div>`;
  bindForm(entity, schema);
  const search=$(`#search_${entity}`); search.oninput=()=>drawTable(entity,schema,search.value);
  $('[data-new]').onclick=()=>{editing[entity]={}; renderEntity(entity,host)};
  $('[data-csv]').onclick=()=>downloadCSV(entity, records(entity));
  drawTable(entity,schema,'');
}
function entityForm(entity,schema,rec){
  return `<form class="form" id="form_${entity}"><input type="hidden" name="id" value="${esc(rec.id||'')}">
    ${rec.number?`<div class="note"><b>${esc(rec.number)}</b>${rec.jobRef?` &nbsp; Job: ${esc(rec.jobRef)}`:''}</div>`:''}
    ${schema.fields.map(fieldHTML(rec)).join('')}
    <div class="toolbar"><button type="submit">Save</button>${rec.id?`<button type="button" class="light" data-print-current="${entity}">Print / PDF</button><button type="button" class="warn" data-approval-current="${entity}">Request Approval</button>`:''}</div>
  </form>`;
}
function fieldHTML(rec){ return fld=>{
  const val = fld.name==='linesText' ? linesToText(rec.lines||[]) : (rec[fld.name] ?? '');
  const req=fld.required?'required':'';
  if(fld.type==='textarea') return `<label>${fld.label}<textarea name="${fld.name}" ${req}>${esc(val)}</textarea></label>`;
  if(fld.type==='select') return `<label>${fld.label}<select name="${fld.name}" ${req}><option value=""></option>${(fld.options||[]).map(o=>`<option ${String(val)===String(o)?'selected':''}>${esc(o)}</option>`).join('')}</select></label>`;
  return `<label>${fld.label}<input name="${fld.name}" type="${fld.type||'text'}" value="${esc(fld.type==='date'?dateOnly(val):val)}" ${req}></label>`;
};}
function bindForm(entity,schema){
  const form=$(`#form_${entity}`);
  form.onsubmit=async ev=>{
    ev.preventDefault();
    const rec={}; new FormData(form).forEach((v,k)=>{ if(k==='linesText') return; rec[k]=v; });
    if(!rec.id) delete rec.id;
    for(const fld of schema.fields){ if(fld.type==='number') rec[fld.name]=num(rec[fld.name]); }
    if(['quotations','proformas','invoices','shipments'].includes(entity)){ rec.amount=num(rec.amount); rec.cost=num(rec.cost); rec.profit = rec.profit===''||rec.profit==null ? rec.amount-rec.cost : num(rec.profit); }
    const linesText=form.elements['linesText']?.value; if(linesText!==undefined) rec.lines=parseLines(linesText);
    if(rec.autoInvoiceTriggered==='Yes') rec.autoInvoiceTriggered=true; if(rec.autoInvoiceTriggered==='No') rec.autoInvoiceTriggered=false;
    try{ const out=await api('/api/save',{method:'POST',body:JSON.stringify({entity,record:rec})}); await loadData(); editing[entity]=out.record; render(); toast('Saved'); }
    catch(e){ alert(e.message); }
  };
  const p=$('[data-print-current]'); if(p) p.onclick=()=>window.open(`/print?entity=${entity}&id=${encodeURIComponent(editing[entity].id)}`,'_blank');
  const a=$('[data-approval-current]'); if(a) a.onclick=()=>requestApproval(entity, editing[entity].id);
}
function drawTable(entity,schema,filter){
  let list=records(entity).slice().reverse(); const q=String(filter||'').toLowerCase(); if(q) list=list.filter(r=>JSON.stringify(r).toLowerCase().includes(q));
  const cols=schema.columns||['number','name','status'];
  $(`#table_${entity}`).innerHTML = !list.length ? '<div class="empty">No records yet. Add the first record from the form.</div>' : `<div class="table-wrap"><table><thead><tr>${cols.map(c=>`<th>${label(c)}</th>`).join('')}<th>Actions</th></tr></thead><tbody>${list.map(r=>`<tr>${cols.map(c=>`<td>${cell(r,c)}</td>`).join('')}<td>${actionButtons(entity,r)}</td></tr>`).join('')}</tbody></table></div>`;
  $$(`[data-edit-${entity}]`).forEach(b=>b.onclick=()=>{ editing[entity]=records(entity).find(x=>x.id===b.dataset[`edit${camel(entity)}`]); render(); });
  $$(`[data-print-${entity}]`).forEach(b=>b.onclick=()=>window.open(`/print?entity=${entity}&id=${encodeURIComponent(b.dataset[`print${camel(entity)}`])}`,'_blank'));
  $$(`[data-approve-${entity}]`).forEach(b=>b.onclick=()=>requestApproval(entity,b.dataset[`approve${camel(entity)}`]));
  $$(`[data-cancel-${entity}]`).forEach(b=>b.onclick=()=>cancelRecord(entity,b.dataset[`cancel${camel(entity)}`]));
  $$(`[data-convert]`).forEach(b=>b.onclick=()=>convertRecord(b.dataset.from,b.dataset.id,b.dataset.to));
}
function camel(s){ return s.replace(/-([a-z])/g,(_,m)=>m.toUpperCase()); }
function cell(r,c){ if(/status|risk|proof|detention/i.test(c)) return statusBadge(r[c]); if(/amount|cost|profit|balance|limit|price/i.test(c)) return esc(r[c]!==undefined?Number(r[c]||0).toLocaleString(): ''); return esc(r[c]??''); }
function actionButtons(entity,r){
  let html=`<div class="actions"><button class="light" data-edit-${entity}="${esc(r.id)}">Edit</button><button class="light" data-print-${entity}="${esc(r.id)}">Print</button><button class="warn" data-approve-${entity}="${esc(r.id)}">Approval</button>`;
  if(entity==='quotations') html+=`<button data-convert data-from="quotations" data-id="${esc(r.id)}" data-to="proformas">To PI</button><button data-convert data-from="quotations" data-id="${esc(r.id)}" data-to="invoices">To Invoice</button><button data-convert data-from="quotations" data-id="${esc(r.id)}" data-to="shipments">To Shipment</button>`;
  if(entity==='proformas') html+=`<button data-convert data-from="proformas" data-id="${esc(r.id)}" data-to="invoices">To Invoice</button><button data-convert data-from="proformas" data-id="${esc(r.id)}" data-to="packingLists">To Packing</button>`;
  if(entity==='shipments') html+=`<button data-convert data-from="shipments" data-id="${esc(r.id)}" data-to="bls">Create BL</button><button data-convert data-from="shipments" data-id="${esc(r.id)}" data-to="deliveryNotes">Create DN</button>`;
  if(entity==='deliveryNotes') html+=`<button data-convert data-from="deliveryNotes" data-id="${esc(r.id)}" data-to="invoices">Trigger Invoice</button>`;
  html+=`<button class="danger" data-cancel-${entity}="${esc(r.id)}">Cancel</button></div>`;
  return html;
}
function parseLines(txt){ return String(txt||'').split('\n').map(x=>x.trim()).filter(Boolean).map(line=>{ const [description='',qty='1',unit='',price='0']=line.split('|').map(x=>x.trim()); return {description,qty:num(qty),unit,price:num(price),amount:num(qty)*num(price)}; }); }
function linesToText(lines){ return (lines||[]).map(l=>`${l.description||''} | ${l.qty||''} | ${l.unit||''} | ${l.price||''}`).join('\n'); }
async function requestApproval(entity,id){ const reason=prompt('Reason for approval request:','Manager approval required'); if(!reason) return; try{ await api('/api/request-approval',{method:'POST',body:JSON.stringify({entity,id,reason})}); await loadData(); render(); toast('Approval requested'); }catch(e){alert(e.message);} }
async function cancelRecord(entity,id){ const reason=prompt('Cancellation reason is required. No permanent deletion is allowed:'); if(!reason) return; try{ await api('/api/cancel',{method:'POST',body:JSON.stringify({entity,id,reason})}); await loadData(); render(); toast('Cancelled with audit trail'); }catch(e){alert(e.message);} }
async function convertRecord(from,id,to){ try{ const out=await api('/api/convert',{method:'POST',body:JSON.stringify({fromEntity:from,id,toEntity:to})}); await loadData(); editing[to]=out.record; currentModule = Object.entries(groups).find(([g,ents])=>ents.includes(to))?.[0] || currentModule; activeTabs[currentModule]=to; render(); toast(`Converted to ${label(to)}`); }catch(e){alert(e.message);} }

function renderApprovals(){
  setTitle('Approvals & Immutable Audit','Manager control: approve, reject, cancellation records and full history');
  const approvals=records('approvals').slice().reverse(); const audit=(appData.audit||[]).slice().reverse().slice(0,300);
  $('#content').innerHTML=`<div class="grid cols-2"><div class="card"><h2>Approval Queue</h2>${approvalTable(approvals)}</div><div class="card"><h2>Audit Trail</h2>${auditTable(audit)}</div></div>`;
  $$('[data-ap-action]').forEach(b=>b.onclick=()=>approvalAction(b.dataset.id,b.dataset.apAction));
}
function approvalTable(list){ if(!list.length) return '<div class="empty">No approvals.</div>'; return `<div class="table-wrap"><table><thead><tr><th>No</th><th>Module</th><th>Reference</th><th>Reason</th><th>By</th><th>Status</th><th>Action</th></tr></thead><tbody>${list.map(a=>`<tr><td>${esc(a.number)}</td><td>${esc(a.entity)}</td><td>${esc(a.referenceNo)}</td><td>${esc(a.reason)}</td><td>${esc(a.requestedBy)}</td><td>${statusBadge(a.status)}</td><td class="actions">${a.status==='Pending'?`<button class="ok" data-id="${esc(a.id)}" data-ap-action="approve">Approve</button><button class="danger" data-id="${esc(a.id)}" data-ap-action="reject">Reject</button>`:''}</td></tr>`).join('')}</tbody></table></div>`; }
function auditTable(list){ if(!list.length) return '<div class="empty">No audit records.</div>'; return `<div class="table-wrap"><table><thead><tr><th>Time</th><th>User</th><th>Action</th><th>Module</th><th>Ref</th><th>Details</th></tr></thead><tbody>${list.map(a=>`<tr><td>${esc(String(a.at||'').replace('T',' ').slice(0,19))}</td><td>${esc(a.user)}</td><td>${esc(a.action)}</td><td>${esc(a.entity)}</td><td>${esc(a.ref)}</td><td>${esc(a.details)}</td></tr>`).join('')}</tbody></table></div>`; }
async function approvalAction(id,action){ const notes=prompt(`${action} notes:`, action==='approve'?'Approved':'Rejected'); if(notes===null) return; try{ await api('/api/approval',{method:'POST',body:JSON.stringify({approvalID:id,action,notes})}); await loadData(); render(); toast('Approval updated'); }catch(e){alert(e.message);} }

function renderReports(){
  setTitle('Reports','Profit, cash flow, branch/company control and export tools');
  const s=dashboardStats(); const c=appData.settings?.baseCurrency||'AED';
  const profitByCustomer={}; for(const r of [...records('invoices'),...records('shipments')]) profitByCustomer[r.customerName||'Unknown']=(profitByCustomer[r.customerName||'Unknown']||0)+num(r.profit);
  const routeProfit={}; for(const r of records('shipments')) routeProfit[r.route||`${r.origin||''}-${r.destination||''}`]=(routeProfit[r.route||'Unknown']||0)+num(r.profit);
  $('#content').innerHTML=`<div class="grid cols-3"><div class="card"><h2>Financial Summary</h2>${line('Cash balance',money(s.cash,c))}${line('Receivables',money(s.receivables,c))}${line('Payments/Payables',money(s.payables,c))}${line('Profit/Loss',money(s.profit,c))}</div><div class="card"><h2>Profit by Customer</h2>${objectLines(profitByCustomer,c)}</div><div class="card"><h2>Profit by Route</h2>${objectLines(routeProfit,c)}</div></div><div class="card" style="margin-top:18px"><h2>Exports</h2><div class="toolbar">${Object.keys(schemas).map(e=>`<button class="light" onclick="downloadCSV('${e}', records('${e}'))">Export ${label(e)}</button>`).join('')}</div></div>`;
}
function line(k,v){ return `<div class="report-line"><span>${esc(k)}</span><b>${esc(v)}</b></div>`; }
function objectLines(o,c){ const entries=Object.entries(o).sort((a,b)=>b[1]-a[1]).slice(0,10); return entries.length?entries.map(([k,v])=>line(k,money(v,c))).join(''):'<div class="empty">No data yet.</div>'; }

function renderAI(){
  setTitle('AI Assistant MVP','Offline rule-based extraction and management hints. It does not approve payments.');
  $('#content').innerHTML=`<div class="grid cols-2"><div class="card"><h2>Paste document text</h2><p class="muted">Paste invoice, BL, bank statement, delivery note or email text. This local MVP extracts common fields using rules.</p><textarea id="aiText" class="ai-box" placeholder="Paste text here..."></textarea><div class="toolbar" style="margin-top:12px"><button id="analyzeBtn">Analyze</button><button class="light" id="sampleAi">Load Sample</button></div><p class="note">AI cannot approve payments alone. Final approval must stay with authorized managers.</p></div><div class="card"><h2>Result</h2><div id="aiResult" class="empty">No analysis yet.</div></div></div>`;
  $('#sampleAi').onclick=()=>{$('#aiText').value='BL No: BL-2026-0099\nContainer: ABCD1234567\nSeal: SEAL7788\nInvoice Amount USD 12500\nCustomer: Demo Customer LLC\nDelivery Date: 2026-06-30';};
  $('#analyzeBtn').onclick=analyzeText;
}
function analyzeText(){
  const t=$('#aiText').value; const find=(re)=>((t.match(re)||[])[1]||'');
  const amount=find(/(?:amount|total|invoice amount)\s*[:\- ]*([A-Z]{3})?\s*([0-9,.]+)/i); const currency=(t.match(/\b(AED|USD|EUR|OMR|GBP|CNY|AFN)\b/)||[])[1]||'';
  const fields={company:find(/(?:customer|company|consignee|shipper)\s*[:\- ]*([^\n]+)/i), bl:find(/\bBL\s*(?:No|Number)?\s*[:\- ]*([A-Z0-9\-\/]+)/i), container:find(/\b([A-Z]{4}\d{7})\b/), seal:find(/seal\s*(?:no|number)?\s*[:\- ]*([A-Z0-9\-]+)/i), amount:amount?`${currency} ${amount}`:'', date:find(/(?:date|delivery date)\s*[:\- ]*([0-9]{4}\-[0-9]{2}\-[0-9]{2}|[0-9]{1,2}\/[0-9]{1,2}\/[0-9]{2,4})/i)};
  const warnings=[]; if(fields.container && !/^[A-Z]{4}\d{7}$/.test(fields.container)) warnings.push('Container number format should be 4 letters + 7 digits.'); if(/sanction|blocked|prohibited/i.test(t)) warnings.push('Compliance risk keyword detected. Manager review required.'); if(/duplicate|same invoice/i.test(t)) warnings.push('Possible duplicate invoice wording detected.'); if(!fields.amount && /invoice|bank|payment/i.test(t)) warnings.push('No amount found; check document manually.');
  $('#aiResult').innerHTML=`<h3>Extracted Fields</h3>${Object.entries(fields).map(([k,v])=>line(label(k),v||'<span class="muted">Not found</span>')).join('')}<h3>Risk / Control Hints</h3>${warnings.length?warnings.map(w=>`<p class="note danger-note">${esc(w)}</p>`).join(''):'<p class="note">No major warning found by local rules.</p>'}`;
}

function renderSettings(){
  setTitle('Settings','Company profile, backup/restore and password'); const s=appData.settings||{};
  $('#content').innerHTML=`<div class="grid cols-2"><div class="card"><h2>Company Profile</h2><form class="form" id="settingsForm">${['companyName','branch','baseCurrency','supportedCurrencies','address','email','phone','watermark','documentFooter'].map(k=>`<label>${label(k)}${k==='address'||k==='documentFooter'?`<textarea name="${k}">${esc(s[k]||'')}</textarea>`:`<input name="${k}" value="${esc(s[k]||'')}">`}</label>`).join('')}<button>Save Settings</button></form></div><div class="card"><h2>Security / Backup</h2><p class="note">Data is stored locally in <b>${esc(appData.dataFile||'zenith_erp_data.json')}</b>. Keep secure backups.</p><div class="toolbar"><button onclick="window.open('/api/backup','_blank')">Download Backup JSON</button></div><h3>Restore Backup JSON</h3><input type="file" id="restoreFile" accept="application/json"><button class="warn" id="restoreBtn">Restore</button><h3>Change Password</h3><form class="form" id="pwForm"><input type="password" name="oldPassword" placeholder="Old password"><input type="password" name="newPassword" placeholder="New password, min 8 characters"><button>Change Password</button></form></div></div>`;
  $('#settingsForm').onsubmit=async ev=>{ev.preventDefault(); const r={}; new FormData(ev.target).forEach((v,k)=>r[k]=v); try{await api('/api/settings',{method:'POST',body:JSON.stringify(r)}); await loadData(); render(); toast('Settings saved');}catch(e){alert(e.message);}};
  $('#pwForm').onsubmit=async ev=>{ev.preventDefault(); const r={}; new FormData(ev.target).forEach((v,k)=>r[k]=v); try{await api('/api/change-password',{method:'POST',body:JSON.stringify(r)}); ev.target.reset(); await loadData(); toast('Password changed');}catch(e){alert(e.message);}};
  $('#restoreBtn').onclick=async()=>{ const file=$('#restoreFile').files[0]; if(!file) return alert('Choose a backup JSON file.'); if(!confirm('Restore will replace current ERP data after creating a local safety backup. Continue?')) return; try{await fetch('/api/restore',{method:'POST',credentials:'same-origin',body:await file.text()}).then(async r=>{if(!r.ok) throw new Error(await r.text())}); await loadData(); render(); toast('Backup restored');}catch(e){alert(e.message);} };
}

function downloadCSV(entity,list){
  if(!list.length) return alert('No records to export.'); const keys=[...new Set(list.flatMap(r=>Object.keys(r)))]; const rows=[keys.join(',')].concat(list.map(r=>keys.map(k=>`"${String(r[k]??'').replace(/"/g,'""')}"`).join(','))); const blob=new Blob([rows.join('\n')],{type:'text/csv'}); const a=document.createElement('a'); a.href=URL.createObjectURL(blob); a.download=`${entity}_${new Date().toISOString().slice(0,10)}.csv`; a.click(); URL.revokeObjectURL(a.href);
}

boot();
