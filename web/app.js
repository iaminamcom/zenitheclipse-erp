'use strict';

const STORE_KEY = 'zenith-eclipse-erp-ultimate-data-v3';
let API_MODE = false;
let S = null;
let USER = null;
let USERS_CACHE = [];

const DOC_TYPES = {
  quotation: { title: 'Quotation', plural: 'Quotations', suffix: 'QTN', page: 'quotations', icon: '📝' },
  pi: { title: 'Proforma Invoice', plural: 'Proforma Invoices', suffix: 'PI', page: 'pi', icon: '📑' },
  invoice: { title: 'Commercial Invoice', plural: 'Commercial Invoices', suffix: 'CI', page: 'invoices', icon: '🧾' },
  packing: { title: 'Packing List', plural: 'Packing Lists', suffix: 'PL', page: 'packing', icon: '🚢' },
  agreement: { title: 'Agreement', plural: 'Agreements', suffix: 'AGR', page: 'agreements', icon: '🤝' },
  purchase: { title: 'Purchase Order', plural: 'Purchase Orders', suffix: 'PO', page: 'purchasing', icon: '🛒' },
  delivery: { title: 'Delivery Note', plural: 'Delivery Notes', suffix: 'DN', page: 'inventory', icon: '🚚' },
  receipt: { title: 'Receipt Voucher', plural: 'Receipt Vouchers', suffix: 'RCPT', page: 'accounting', icon: '💳' }
};
const FLOW_TYPES = ['quotation', 'pi', 'invoice', 'packing', 'agreement'];
const CURRENCIES = ['USD', 'AED', 'AFN', 'CNY', 'EUR', 'GBP', 'OMR', 'PKR'];
const STATUS_LIST = ['Draft', 'Sent', 'Pending Approval', 'Accepted', 'Approved', 'Converted', 'Partial', 'Paid', 'Completed', 'Cancelled'];
const DEAL_MODES = [
  { value: 'combined', label: 'Product + Transportation', icon: '🚀', hint: 'Goods and freight together in one quotation/invoice' },
  { value: 'product', label: 'Product sale', icon: '📦', hint: 'Physical goods, HS code, stock and weights' },
  { value: 'transport', label: 'Transportation only', icon: '🚚', hint: 'Freight, truck, container, route, BL and delivery charges' },
  { value: 'service', label: 'Service only', icon: '🧾', hint: 'Customs, documentation, handling or other services' }
];
const LINE_KINDS = [
  { value: 'product', label: 'Product', icon: '📦' },
  { value: 'transport', label: 'Transportation', icon: '🚚' },
  { value: 'service', label: 'Service', icon: '🧾' },
  { value: 'charge', label: 'Other charge', icon: '➕' },
  { value: 'discount', label: 'Discount', icon: '−' }
];

const PAGE_META = {
  dashboard: { title: 'Dashboard', sub: 'Business overview, alerts, receivables and workflow control', icon: '📊' },
  chains: { title: 'Business Chains', sub: 'One serial number connects quotation, PI, invoice, packing list and agreement', icon: '🔗' },
  customers: { title: 'Customers', sub: 'Customer accounts, contact details, statements and credit control', icon: '👥' },
  suppliers: { title: 'Suppliers', sub: 'Supplier accounts, purchase side and payable contacts', icon: '🏭' },
  products: { title: 'Products, Transport & Services', sub: 'Choose goods, transport charges, customs/services and route prices', icon: '🛒' },
  inventory: { title: 'Inventory, Transport & Logistics', sub: 'Stock, containers, BL, seal, route, POL/POD and delivery status', icon: '🚚' },
  quotations: { title: 'Quotations', sub: 'Professional offers with customer approval and revision control', icon: '📝' },
  pi: { title: 'Proforma Invoices', sub: 'PI documents connected to quotation serial numbers', icon: '📑' },
  invoices: { title: 'Commercial Invoices', sub: 'Official invoices, tax fields, payment status and PDF print', icon: '🧾' },
  packing: { title: 'Packing Lists', sub: 'Container, seal, BL, weight, packages and shipping data', icon: '🚢' },
  agreements: { title: 'Agreements', sub: 'Agreement documents using the same letterhead and base serial', icon: '🤝' },
  purchasing: { title: 'Purchasing', sub: 'Purchase orders, supplier bills and landed cost control', icon: '🛒' },
  accounting: { title: 'Accounting', sub: 'Receipts, payments, cash, bank and multi-currency balances', icon: '💳' },
  expenses: { title: 'Expenses', sub: 'Office, bank, logistics and operational expense tracking', icon: '💸' },
  reports: { title: 'Reports & Audit', sub: 'Statements, aging, profit, VAT summary and audit trail', icon: '📈' },
  assistant: { title: 'Smart Command Center', sub: 'Fast commands for unpaid invoices, conversions and business checks', icon: '🤖' },
  letterhead: { title: 'Letterhead Designer', sub: 'Logo, footer, stamp, signature and document print design', icon: '🎨' },
  serials: { title: 'Serial Number Manager', sub: 'One serial chain, revisions, prefixes and duplicate protection', icon: '🔢' },
  users: { title: 'Employees & Access', sub: 'Self-signup, approvals, roles and employee account control', icon: '🔐' },
  settings: { title: 'Settings & Backup', sub: 'Company profile, server mode, password, backup and restore', icon: '⚙️' }
};

function $(id) { return document.getElementById(id); }
function esc(v) { return String(v ?? '').replace(/[&<>'"]/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&quot;' }[c])); }
function num(v) { const n = Number(String(v ?? '').replace(/,/g, '')); return Number.isFinite(n) ? n : 0; }
function money(v, cur = '') { const out = cleanMoney(v).toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 }); return cur ? `${out} ${cur}` : out; }
function cleanMoney(v) { return Math.round(num(v) * 100) / 100; }
function today() { return new Date().toISOString().slice(0, 10); }
function plusDays(days) { const d = new Date(); d.setDate(d.getDate() + days); return d.toISOString().slice(0, 10); }
function nowISO() { return new Date().toISOString(); }
function uid(prefix = 'id') { return `${prefix}-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 9)}`; }
function clone(x) { return JSON.parse(JSON.stringify(x)); }
function titleCase(s) { return String(s || '').replace(/[-_]/g, ' ').replace(/\b\w/g, c => c.toUpperCase()); }
function isAdmin() { return USER?.role === 'admin' || !API_MODE; }
function roleCanEdit() { return !API_MODE || !['viewer'].includes(USER?.role); }
function baseCurrency() { return S?.company?.baseCurrency || S?.settings?.baseCurrency || 'USD'; }
function logoSrc() { return S?.company?.logoData || S?.company?.logoUrl || 'assets/zenith-logo.jpeg'; }
function leafSrc() { return S?.company?.leafUrl || 'assets/maple-leaf.png'; }
function iconOptions() { return CURRENCIES.map(c => `<option value="${c}">${c}</option>`).join(''); }
function setHash(hash) { location.hash = hash; }

window.addEventListener('DOMContentLoaded', boot);
window.addEventListener('hashchange', () => { if (S) renderPage(); });

document.addEventListener('click', async (event) => {
  const btn = event.target.closest('[data-action]');
  if (!btn) return;
  const action = btn.dataset.action;
  try {
    if (action === 'logout') return logout();
    if (action === 'backup') return downloadBackup();
    if (action === 'export-xlsx') return exportXLSX();
    if (action === 'export-csv') return exportCSV(btn.dataset.table || 'documents');
    if (action === 'open-party') return openPartyModal(btn.dataset.kind, btn.dataset.id || 'new');
    if (action === 'save-party') return savePartyFromModal();
    if (action === 'delete-party') return deleteParty(btn.dataset.kind, btn.dataset.id);
    if (action === 'open-product') return openProductModal(btn.dataset.id || 'new');
    if (action === 'save-product') return saveProductFromModal();
    if (action === 'delete-product') return deleteProduct(btn.dataset.id);
    if (action === 'create-doc') return createDoc(btn.dataset.type, btn.dataset.base || '');
    if (action === 'save-doc') return saveDocFromEditor();
    if (action === 'delete-doc') return deleteDoc(btn.dataset.id);
    if (action === 'print-doc') return printDoc(btn.dataset.id);
    if (action === 'convert-doc') return convertDoc(btn.dataset.id, btn.dataset.target);
    if (action === 'mark-doc-status') return markDocStatus(btn.dataset.id, btn.dataset.status);
    if (action === 'add-line') return addItemLine();
    if (action === 'add-line-kind') return addItemLine(btn.dataset.kind || 'product');
    if (action === 'remove-line') { btn.closest('tr')?.remove(); recalcEditorTotals(); return; }
    if (action === 'open-payment') return openPaymentModal(btn.dataset.id || 'new', btn.dataset.doc || '');
    if (action === 'save-payment') return savePaymentFromModal();
    if (action === 'delete-payment') return deletePayment(btn.dataset.id);
    if (action === 'open-expense') return openExpenseModal(btn.dataset.id || 'new');
    if (action === 'save-expense') return saveExpenseFromModal();
    if (action === 'delete-expense') return deleteExpense(btn.dataset.id);
    if (action === 'save-company') return saveCompanySettings();
    if (action === 'save-serials') return saveSerialSettings();
    if (action === 'next-serial') return allocateSerialPreview();
    if (action === 'close-modal') return closeModal();
    if (action === 'restore-backup') return restoreBackup();
    if (action === 'change-password') return changePassword();
    if (action === 'user-action') return userAction(btn.dataset.username, btn.dataset.userAction, btn.dataset.role || '');
    if (action === 'run-command') return runCommand();
    if (action === 'create-chain') return createDoc('quotation', '');
    if (action === 'copy-approval') return copyApprovalText(btn.dataset.id);
    if (action === 'save-signup-settings') return saveSignupSettings();
  } catch (err) {
    console.error(err);
    toast(err.message || String(err), 'error');
  }
});

document.addEventListener('change', (event) => {
  const el = event.target;
  if (el.matches('.product-select')) fillLineFromProduct(el);
  if (el.matches('.recalc')) recalcEditorTotals();
  if (el.id === 'mobileRoute') setHash(el.value);
  if (el.matches('[data-image-upload]')) readImageUpload(el);
});

document.addEventListener('input', (event) => {
  if (event.target.matches('.recalc')) recalcEditorTotals();
});

async function boot() {
  const backend = await tryBackend();
  if (!backend) {
    API_MODE = false;
    USER = { username: 'local', fullName: 'Local User', role: 'admin', status: 'active' };
    S = loadLocalData();
    renderShell();
  }
  if ('serviceWorker' in navigator && location.protocol.startsWith('http')) {
    navigator.serviceWorker.register('sw.js').catch(() => {});
  }
}

async function tryBackend() {
  if (!location.protocol.startsWith('http')) return false;
  try {
    const res = await fetch('/api/me', { credentials: 'include' });
    API_MODE = true;
    if (res.status === 401) {
      const info = await safeJSON(res);
      showLogin('', info?.signupsEnabled !== false);
      return true;
    }
    if (res.ok) {
      const me = await res.json();
      USER = me.user || me;
      await loadAPIData();
      renderShell();
      return true;
    }
  } catch (err) {
    return false;
  }
  return false;
}

async function safeJSON(res) { try { return await res.json(); } catch { return null; } }

function showLogin(message = '', signupsEnabled = true) {
  document.body.classList.add('login-mode');
  $('app').className = 'login-page';
  $('app').innerHTML = `
    <section class="login-card">
      <div class="login-hero">
        <img src="assets/zenith-logo.jpeg" alt="Zenith Eclipse logo">
        <h1>Zenith Eclipse ERP Ultimate</h1>
        <p>Accounting, quotation, proforma invoice, commercial invoice, packing list, agreements, logistics, reports and employee accounts in one server-ready system.</p>
        <div class="hero-points">
          <div>🔗 One serial chain from quotation to packing list</div>
          <div>🔐 Employee self-signup with admin approval</div>
          <div>🎨 Letterhead matching your Zenith sample</div>
          <div>🌐 Ready for company server deployment</div>
        </div>
      </div>
      <div class="login-forms">
        <div class="tabs">
          <button class="tab active" data-login-tab="login" type="button">Login</button>
          <button class="tab" data-login-tab="register" type="button" ${signupsEnabled ? '' : 'disabled'}>Create employee account</button>
        </div>
        <form class="form-panel active" id="loginForm">
          <h2>Login</h2>
          <p>Use admin first, then approve employee accounts from Employees & Access.</p>
          ${message ? `<div class="form-message ok">${esc(message)}</div>` : ''}
          <div id="loginMsg"></div>
          <div class="field-grid one">
            <label>Username<input name="username" autocomplete="username" value="admin" required></label>
            <label>Password<input name="password" type="password" autocomplete="current-password" placeholder="Password" required></label>
          </div>
          <br><button class="btn block" type="submit">Login</button>
          <p class="tiny muted"><strong>First login:</strong> admin / admin123. Change it after entering.</p>
        </form>
        <form class="form-panel" id="registerForm">
          <h2>Create employee account</h2>
          <p>Employees can request accounts. Admin approval is required before they can login.</p>
          <div id="registerMsg"></div>
          <div class="field-grid">
            <label>Full name<input name="fullName" required></label>
            <label>Email<input name="email" type="email"></label>
            <label>Username<input name="username" autocomplete="username" required></label>
            <label>Password<input name="password" type="password" autocomplete="new-password" required minlength="6"></label>
          </div>
          <br><button class="btn block" type="submit">Request account</button>
          ${signupsEnabled ? '' : '<p class="risk">Self signup is disabled by admin.</p>'}
        </form>
      </div>
    </section>`;
  document.querySelectorAll('[data-login-tab]').forEach(tab => tab.addEventListener('click', () => {
    document.querySelectorAll('[data-login-tab]').forEach(x => x.classList.remove('active'));
    tab.classList.add('active');
    document.querySelectorAll('.form-panel').forEach(x => x.classList.remove('active'));
    $(tab.dataset.loginTab + 'Form').classList.add('active');
  }));
  $('loginForm').addEventListener('submit', async e => {
    e.preventDefault();
    const fd = new FormData(e.currentTarget);
    const msg = $('loginMsg');
    msg.innerHTML = '';
    try {
      const res = await fetch('/api/login', { method: 'POST', credentials: 'include', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(Object.fromEntries(fd)) });
      const data = await safeJSON(res);
      if (!res.ok) throw new Error(data?.error || 'Login failed');
      USER = data.user || data;
      await loadAPIData();
      renderShell();
    } catch (err) {
      msg.innerHTML = `<div class="form-message error">${esc(err.message)}</div>`;
    }
  });
  $('registerForm').addEventListener('submit', async e => {
    e.preventDefault();
    const fd = new FormData(e.currentTarget);
    const msg = $('registerMsg');
    msg.innerHTML = '';
    try {
      const res = await fetch('/api/register', { method: 'POST', credentials: 'include', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(Object.fromEntries(fd)) });
      const data = await safeJSON(res);
      if (!res.ok) throw new Error(data?.error || 'Registration failed');
      msg.innerHTML = `<div class="form-message ok">Account requested. Admin must approve it first.</div>`;
      e.currentTarget.reset();
    } catch (err) {
      msg.innerHTML = `<div class="form-message error">${esc(err.message)}</div>`;
    }
  });
}

async function loadAPIData() {
  const res = await fetch('/api/data', { credentials: 'include' });
  if (!res.ok) throw new Error('Could not load server data');
  S = await res.json();
  normalizeState();
}

function loadLocalData() {
  try {
    const raw = localStorage.getItem(STORE_KEY);
    if (raw) {
      S = JSON.parse(raw);
      normalizeState();
      return S;
    }
  } catch {}
  S = seedState();
  normalizeState();
  localStorage.setItem(STORE_KEY, JSON.stringify(S));
  return S;
}

function seedState() {
  const now = nowISO();
  return {
    version: '3.0.0',
    company: {
      name: 'ZENITH ECLIPSE CO', legalName: 'ZENITH ECLIPSE CO', slogan: 'NURTURING FIELDS OF TOMORROW WEAVING WORLDWIDE PATHWAYS OF PROSPERITY',
      address: 'Citadel Tower, office 204, Business Bay, Dubai, UAE', city: 'Dubai', country: 'United Arab Emirates', phone: '+93 77 404 7259', email: 'sales@zenitheclipse.com', website: 'http://www.zenitheclipse.com', taxId: '',
      bankName: '', bankAccount: '', bankIban: '', bankSwift: '', baseCurrency: 'USD', logoUrl: 'assets/zenith-logo.jpeg', leafUrl: 'assets/maple-leaf.png', stampText: 'Authorized Signature',
      defaultTerms: 'Payment terms are subject to final written confirmation. Bank charges are borne by the sender unless agreed otherwise. All documents remain valid only with the official serial number and verification code.', defaultNotes: 'Thank you for your business.'
    },
    settings: { serialPrefix: 'ZE', serialCode: 'HRBTC', serialYear: '2026', serialPadding: 4, nextSerial: 1, signupsEnabled: true, defaultRole: 'staff', approvalRequired: true, lockBeforeDate: '', revisionMode: true },
    customers: [{ id: 'cus-demo-1', type: 'customer', name: 'Haroon Rezwan and Bradaran Amar khil Trade Co', contact: 'MR. Abdul Qasum', email: '', phone: '', address: 'Shop# 19, Faisal Sharif Market, Mandawi, Kabul AFG', city: 'Kabul', country: 'Afghanistan', currency: 'USD', taxId: '', notes: 'Sample customer from uploaded letterhead', createdAt: now, updatedAt: now }],
    suppliers: [{ id: 'sup-demo-1', type: 'supplier', name: 'Demo Supplier Co.', contact: 'Sales Team', email: 'supplier@example.com', phone: '+86 000 0000', address: 'Shenzhen', city: 'Shenzhen', country: 'China', currency: 'USD', taxId: '', notes: '', createdAt: now, updatedAt: now }],
    products: [
      { id: 'prd-goods-general', category: 'product', sku: 'PRD-GEN', name: 'General Trading Product', description: 'Physical goods/product line with HS code, quantity, cartons and weight', hsCode: '', unit: 'Unit', costPrice: 0, salePrice: 0, currency: 'USD', stockQty: 0, warehouse: 'Dubai Warehouse', notes: '', createdAt: now, updatedAt: now },
      { id: 'prd-fertilizer', category: 'product', sku: 'PRD-AGR-001', name: 'Agricultural Product / Fertilizer', description: 'Sample product line for product sale quotations and invoices', hsCode: '', unit: 'Bag', costPrice: 0, salePrice: 0, currency: 'USD', stockQty: 0, warehouse: 'Dubai Warehouse', notes: '', createdAt: now, updatedAt: now },
      { id: 'prd-sea-20', category: 'transport', sku: 'TRN-SEA-20', name: 'Sea Freight 20FT Container', description: 'Sea freight service for one 20FT container', hsCode: '', unit: 'Container', costPrice: 900, salePrice: 1200, currency: 'USD', stockQty: 0, warehouse: 'Logistics Desk', notes: '', createdAt: now, updatedAt: now },
      { id: 'prd-truck', category: 'transport', sku: 'TRN-TRUCK', name: 'Truck Transportation', description: 'Road transportation / trucking service', hsCode: '', unit: 'Trip', costPrice: 0, salePrice: 0, currency: 'USD', stockQty: 0, warehouse: 'Logistics Desk', notes: '', createdAt: now, updatedAt: now },
      { id: 'prd-doc-clear', category: 'service', sku: 'SRV-DOC-CLR', name: 'Customs Clearance & Documentation', description: 'Customs documentation and clearance service', hsCode: '', unit: 'Service', costPrice: 80, salePrice: 150, currency: 'USD', stockQty: 0, warehouse: 'Office', notes: '', createdAt: now, updatedAt: now }
    ],
    documents: [], payments: [], expenses: [], auditLogs: [{ id: 'aud-local', time: now, user: 'local', action: 'System created', entity: 'database', details: 'Local offline database initialized' }]
  };
}

function normalizeState() {
  S = S || {};
  S.version = S.version || '2.0.0';
  S.company = S.company || {};
  S.settings = S.settings || {};
  S.company.name = S.company.name || 'ZENITH ECLIPSE CO';
  S.company.legalName = S.company.legalName || S.company.name;
  S.company.slogan = S.company.slogan || 'NURTURING FIELDS OF TOMORROW WEAVING WORLDWIDE PATHWAYS OF PROSPERITY';
  S.company.email = S.company.email || 'sales@zenitheclipse.com';
  S.company.phone = S.company.phone || '+93 77 404 7259';
  S.company.address = S.company.address || 'Citadel Tower, office 204, Business Bay, Dubai, UAE';
  S.company.website = S.company.website || 'http://www.zenitheclipse.com';
  S.company.baseCurrency = S.company.baseCurrency || 'USD';
  S.company.logoUrl = S.company.logoUrl || 'assets/zenith-logo.jpeg';
  S.company.leafUrl = S.company.leafUrl || 'assets/maple-leaf.png';
  S.company.defaultTerms = S.company.defaultTerms || 'Payment terms are subject to final written confirmation.';
  S.company.defaultNotes = S.company.defaultNotes || 'Thank you for your business.';
  S.settings.serialPrefix = S.settings.serialPrefix || 'ZE';
  S.settings.serialCode = S.settings.serialCode || 'HRBTC';
  S.settings.serialYear = S.settings.serialYear || String(new Date().getFullYear());
  S.settings.serialPadding = num(S.settings.serialPadding) || 4;
  S.settings.nextSerial = num(S.settings.nextSerial) || 1;
  if (S.settings.signupsEnabled === undefined) S.settings.signupsEnabled = true;
  S.settings.defaultRole = S.settings.defaultRole || 'staff';
  if (S.settings.approvalRequired === undefined) S.settings.approvalRequired = true;
  if (S.settings.revisionMode === undefined) S.settings.revisionMode = true;
  for (const k of ['customers', 'suppliers', 'products', 'documents', 'payments', 'expenses', 'auditLogs']) S[k] = Array.isArray(S[k]) ? S[k] : [];
  const defaultProducts = [
    { id: 'prd-goods-general', category: 'product', sku: 'PRD-GEN', name: 'General Trading Product', description: 'Physical goods/product line with HS code, quantity, cartons and weight', hsCode: '', unit: 'Unit', costPrice: 0, salePrice: 0, currency: baseCurrency(), stockQty: 0, warehouse: 'Dubai Warehouse', notes: '', createdAt: nowISO(), updatedAt: nowISO() },
    { id: 'prd-fertilizer', category: 'product', sku: 'PRD-AGR-001', name: 'Agricultural Product / Fertilizer', description: 'Sample product line for product sale quotations and invoices', hsCode: '', unit: 'Bag', costPrice: 0, salePrice: 0, currency: baseCurrency(), stockQty: 0, warehouse: 'Dubai Warehouse', notes: '', createdAt: nowISO(), updatedAt: nowISO() },
    { id: 'prd-sea-20', category: 'transport', sku: 'TRN-SEA-20', name: 'Sea Freight 20FT Container', description: 'Sea freight service for one 20FT container', hsCode: '', unit: 'Container', costPrice: 900, salePrice: 1200, currency: baseCurrency(), stockQty: 0, warehouse: 'Logistics Desk', notes: '', createdAt: nowISO(), updatedAt: nowISO() },
    { id: 'prd-truck', category: 'transport', sku: 'TRN-TRUCK', name: 'Truck Transportation', description: 'Road transportation / trucking service', hsCode: '', unit: 'Trip', costPrice: 0, salePrice: 0, currency: baseCurrency(), stockQty: 0, warehouse: 'Logistics Desk', notes: '', createdAt: nowISO(), updatedAt: nowISO() },
    { id: 'prd-doc-clear', category: 'service', sku: 'SRV-DOC-CLR', name: 'Customs Clearance & Documentation', description: 'Customs documentation and clearance service', hsCode: '', unit: 'Service', costPrice: 80, salePrice: 150, currency: baseCurrency(), stockQty: 0, warehouse: 'Office', notes: '', createdAt: nowISO(), updatedAt: nowISO() }
  ];
  for (const def of defaultProducts) if (!S.products.some(p => String(p.sku || '').toUpperCase() === String(def.sku).toUpperCase())) S.products.push(clone(def));
  for (const p of S.products) {
    p.category = normalizeProductCategory(p.category || inferProductCategory(p));
    p.currency = p.currency || baseCurrency();
    p.unit = p.unit || (p.category === 'transport' ? 'Trip' : p.category === 'service' ? 'Service' : 'Unit');
    p.costPrice = num(p.costPrice); p.salePrice = num(p.salePrice); p.stockQty = num(p.stockQty);
  }
  for (const d of S.documents) {
    d.items = Array.isArray(d.items) ? d.items.map(normalizeLineItem) : [];
    d.dealMode = normalizeDealMode(d.dealMode || inferDealMode(d.items));
    d.baseSerial = d.baseSerial || deriveBaseFromNumber(d.number) || '';
    d.chainId = d.chainId || d.baseSerial || d.id;
    d.revision = num(d.revision);
    d.verificationCode = d.verificationCode || generateVerification(d);
  }
}

function normalizeProductCategory(v) {
  v = String(v || '').toLowerCase().replace('transportation', 'transport').replace('freight', 'transport').replace('goods', 'product');
  return ['product', 'transport', 'service', 'charge', 'discount'].includes(v) ? v : 'product';
}
function inferProductCategory(p = {}) {
  const text = `${p.sku || ''} ${p.name || ''} ${p.description || ''} ${p.unit || ''} ${p.warehouse || ''}`.toLowerCase();
  if (/freight|transport|truck|container|logistic|shipping|vessel|voyage|delivery|port/.test(text)) return 'transport';
  if (/customs|clearance|documentation|handling|service/.test(text)) return 'service';
  return 'product';
}
function inferItemKind(it = {}) {
  const text = `${it.description || ''} ${it.unit || ''}`.toLowerCase();
  if (/freight|transport|truck|container|logistic|shipping|vessel|voyage|delivery|port/.test(text)) return 'transport';
  if (/customs|clearance|documentation|handling|service/.test(text)) return 'service';
  if (/discount|rebate/.test(text)) return 'discount';
  return 'product';
}
function normalizeLineKind(v) { return normalizeProductCategory(v || 'product'); }
function normalizeLineItem(it = {}) {
  const product = S?.products?.find(p => p.id === it.productId);
  const itemKind = normalizeLineKind(it.itemKind || it.category || (product ? product.category : inferItemKind(it)));
  return { ...it, itemKind, category: itemKind, qty: num(it.qty), unitPrice: num(it.unitPrice), netWeight: num(it.netWeight), grossWeight: num(it.grossWeight), packages: num(it.packages) };
}
function normalizeDealMode(v) {
  v = String(v || '').toLowerCase().replace('transportation', 'transport');
  return ['combined', 'product', 'transport', 'service'].includes(v) ? v : 'combined';
}
function inferDealMode(items = []) {
  const kinds = new Set((items || []).map(x => normalizeLineKind(x.itemKind || x.category || inferItemKind(x))));
  if (kinds.has('product') && (kinds.has('transport') || kinds.has('service') || kinds.has('charge'))) return 'combined';
  if (kinds.has('transport')) return 'transport';
  if (kinds.has('product')) return 'product';
  return 'service';
}
function dealModeLabel(v) { const m = DEAL_MODES.find(x => x.value === normalizeDealMode(v)); return m ? `${m.icon} ${m.label}` : '🚀 Product + Transportation'; }
function lineKindLabel(v) { const k = LINE_KINDS.find(x => x.value === normalizeLineKind(v)); return k ? `${k.icon} ${k.label}` : '📦 Product'; }
function lineKindOptions(value = 'product') { return LINE_KINDS.map(k => `<option value="${k.value}" ${normalizeLineKind(value) === k.value ? 'selected' : ''}>${k.icon} ${k.label}</option>`).join(''); }
function dealModeOptions(value = 'combined') { return DEAL_MODES.map(m => `<option value="${m.value}" ${normalizeDealMode(value) === m.value ? 'selected' : ''}>${m.icon} ${m.label}</option>`).join(''); }
function productOptions(selected = '') {
  const groups = { product: 'Products / Goods', transport: 'Transportation', service: 'Services', charge: 'Other Charges', discount: 'Discounts' };
  let html = '<option value="">Custom line</option>';
  for (const [cat, label] of Object.entries(groups)) {
    const list = (S.products || []).filter(p => normalizeProductCategory(p.category) === cat);
    if (!list.length) continue;
    html += `<optgroup label="${esc(label)}">` + list.map(p => `<option value="${p.id}" ${p.id === selected ? 'selected' : ''}>${esc(p.sku || p.name)} · ${esc(p.name)}</option>`).join('') + '</optgroup>';
  }
  return html;
}
function categoryPill(v) { const kind = normalizeLineKind(v); const cls = kind === 'product' ? 'info' : kind === 'transport' ? 'good' : kind === 'service' ? 'warn' : ''; return `<span class="tag ${cls}">${lineKindLabel(kind)}</span>`; }

async function saveData(message = 'Saved') {
  if (!roleCanEdit()) throw new Error('Your role is read-only. Ask admin for staff or manager permission.');
  normalizeState();
  if (API_MODE) {
    const res = await fetch('/api/data', { method: 'PUT', credentials: 'include', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(S) });
    const data = await safeJSON(res);
    if (!res.ok) throw new Error(data?.error || 'Save failed');
  } else {
    localStorage.setItem(STORE_KEY, JSON.stringify(S));
  }
  toast(message, 'ok');
  renderShell(true);
}

function addAudit(action, entity, details, entityId = '') {
  S.auditLogs = S.auditLogs || [];
  S.auditLogs.push({ id: uid('aud'), time: nowISO(), user: USER?.username || 'local', action, entity, entityId, details });
  if (S.auditLogs.length > 2000) S.auditLogs = S.auditLogs.slice(-1500);
}

function renderShell(forceRoute = true) {
  document.body.classList.remove('login-mode');
  const nav = Object.entries(PAGE_META).map(([key, meta]) => `<a href="#${key}" data-page="${key}"><span class="ico">${meta.icon}</span>${meta.title}</a>`).join('');
  const mobile = Object.entries(PAGE_META).map(([key, meta]) => `<option value="${key}">${meta.icon} ${meta.title}</option>`).join('');
  $('app').className = 'app-shell';
  $('app').innerHTML = `
    <aside class="sidebar">
      <div class="brand"><img src="${esc(logoSrc())}" alt="Zenith"><div><h1>Zenith ERP Ultimate</h1><small>${esc(S.company.name)}</small></div></div>
      <nav class="nav" id="navLinks">${nav}</nav>
      <div class="sidebar-footer">
        <span class="mode-pill">${API_MODE ? '🌐 Server mode' : '📱 Offline mode'}</span>
        <div><strong>User:</strong> ${esc(USER?.username || 'local')} · ${esc(USER?.role || 'admin')}</div>
        <div id="saveStatus">Ready</div>
      </div>
    </aside>
    <main class="main">
      <header class="topbar">
        <div class="top-title"><div class="top-ico" id="pageIcon">📊</div><div><h2 id="pageTitle">Dashboard</h2><div id="pageSub" class="sub"></div></div></div>
        <div class="toolbar">
          <span class="status active">${esc(baseCurrency())}</span>
          <button class="btn secondary small" data-action="backup">Backup</button>
          <button class="btn secondary small" data-action="export-xlsx">Excel</button>
          ${API_MODE ? '<button class="btn secondary small" data-action="logout">Logout</button>' : ''}
        </div>
        <div class="mobile-nav"><select id="mobileRoute">${mobile}</select></div>
      </header>
      <section class="content" id="content"></section>
    </main>`;
  if (forceRoute) renderPage();
}

function getRoute() {
  const raw = location.hash.replace(/^#\/?/, '') || 'dashboard';
  const parts = raw.split('/').filter(Boolean);
  if (parts[0] === 'doc') return { page: 'doc-editor', type: parts[1] || 'invoice', id: parts[2] || 'new' };
  return { page: PAGE_META[parts[0]] ? parts[0] : 'dashboard' };
}

function renderPage() {
  const route = getRoute();
  const navPage = route.page === 'doc-editor' ? (DOC_TYPES[route.type]?.page || 'invoices') : route.page;
  const meta = PAGE_META[navPage] || PAGE_META.dashboard;
  document.querySelectorAll('#navLinks a').forEach(a => a.classList.toggle('active', a.dataset.page === navPage));
  if ($('mobileRoute')) $('mobileRoute').value = navPage;
  $('pageIcon').textContent = meta.icon;
  $('pageTitle').textContent = meta.title;
  $('pageSub').textContent = meta.sub;
  const content = $('content');
  switch (route.page) {
    case 'dashboard': content.innerHTML = renderDashboard(); break;
    case 'chains': content.innerHTML = renderChains(); break;
    case 'customers': content.innerHTML = renderParties('customer'); break;
    case 'suppliers': content.innerHTML = renderParties('supplier'); break;
    case 'products': content.innerHTML = renderProducts(); break;
    case 'inventory': content.innerHTML = renderInventory(); break;
    case 'quotations': content.innerHTML = renderDocsPage('quotation'); break;
    case 'pi': content.innerHTML = renderDocsPage('pi'); break;
    case 'invoices': content.innerHTML = renderDocsPage('invoice'); break;
    case 'packing': content.innerHTML = renderDocsPage('packing'); break;
    case 'agreements': content.innerHTML = renderDocsPage('agreement'); break;
    case 'purchasing': content.innerHTML = renderDocsPage('purchase'); break;
    case 'accounting': content.innerHTML = renderAccounting(); break;
    case 'expenses': content.innerHTML = renderExpenses(); break;
    case 'reports': content.innerHTML = renderReports(); break;
    case 'assistant': content.innerHTML = renderAssistant(); break;
    case 'letterhead': content.innerHTML = renderLetterhead(); break;
    case 'serials': content.innerHTML = renderSerials(); break;
    case 'users': content.innerHTML = renderUsers(); loadUsersIfNeeded(); break;
    case 'settings': content.innerHTML = renderSettings(); break;
    case 'doc-editor': content.innerHTML = renderDocEditor(route.type, route.id); recalcEditorTotals(); break;
    default: content.innerHTML = renderDashboard();
  }
}

function renderDashboard() {
  const invoiceDocs = S.documents.filter(d => d.type === 'invoice');
  const sales = invoiceDocs.reduce((a, d) => a + docTotals(d).total, 0);
  const received = S.payments.filter(p => p.type === 'received').reduce((a, p) => a + num(p.amount), 0);
  const paid = S.payments.filter(p => p.type === 'paid').reduce((a, p) => a + num(p.amount), 0);
  const expenses = S.expenses.reduce((a, e) => a + num(e.amount), 0);
  const receivable = invoiceDocs.reduce((a, d) => a + Math.max(0, docTotals(d).total - paidForDoc(d.id)), 0);
  const profit = sales - paid - expenses;
  const pending = S.documents.filter(d => String(d.status).toLowerCase().includes('pending')).length;
  const recent = [...S.documents].sort((a, b) => String(b.updatedAt || b.createdAt || '').localeCompare(String(a.updatedAt || a.createdAt || ''))).slice(0, 8);
  return `
    <div class="kpi-grid">
      ${kpi('Total sales', money(sales, baseCurrency()), 'Commercial invoices')}
      ${kpi('Receivable', money(receivable, baseCurrency()), 'Unpaid invoice balance')}
      ${kpi('Cash received', money(received, baseCurrency()), 'Recorded receipts')}
      ${kpi('Profit estimate', money(profit, baseCurrency()), 'Sales - supplier payments - expenses')}
    </div>
    <div class="grid-2">
      <div class="card">
        <div class="section-title"><div><h3>Smart alerts</h3><p>Important things to check today.</p></div><button class="btn small" data-action="create-chain">New chain</button></div>
        ${renderAlerts(pending, receivable)}
      </div>
      <div class="card">
        <div class="section-title"><div><h3>Serial workflow</h3><p>Next chain and the connected documents.</p></div><a class="btn secondary small" href="#serials">Serial settings</a></div>
        <div class="serial-preview">${esc(peekBaseSerial())}</div>
        <div class="flow" style="margin-top:12px">${FLOW_TYPES.map(t => `<div class="flow-step"><strong>${DOC_TYPES[t].suffix}</strong><small>${DOC_TYPES[t].title}</small></div>`).join('')}</div>
      </div>
    </div>
    <div class="card">
      <div class="section-title"><div><h3>Recent documents</h3><p>Latest work across quotation, PI, invoice, packing list and agreements.</p></div><a class="btn secondary small" href="#chains">View chains</a></div>
      ${documentsTable(recent, true)}
    </div>`;
}

function kpi(label, value, sub) { return `<div class="kpi"><span>${esc(label)}</span><strong>${esc(value)}</strong><small>${esc(sub)}</small></div>`; }
function renderAlerts(pending, receivable) {
  const alerts = [];
  const expired = S.documents.filter(d => d.type === 'quotation' && d.validUntil && d.validUntil < today() && !['Accepted', 'Converted', 'Cancelled'].includes(d.status)).length;
  const noLogo = !S.company.logoData && !S.company.logoUrl;
  if (pending) alerts.push(`<div class="risk">${pending} document(s) are waiting for approval.</div>`);
  if (receivable > 0) alerts.push(`<div class="risk">Receivable balance is ${money(receivable, baseCurrency())}. Check unpaid invoices.</div>`);
  if (expired) alerts.push(`<div class="danger-note">${expired} quotation(s) are expired and not accepted.</div>`);
  if (noLogo) alerts.push(`<div class="risk">Upload your logo in Letterhead Designer.</div>`);
  if (!alerts.length) alerts.push(`<div class="success">No major alert. Your workflow is clean.</div>`);
  alerts.push(`<div class="guide-steps"><div><strong>Best flow:</strong> Quotation → accepted → PI → commercial invoice → packing list → payment → completed.</div></div>`);
  return alerts.join('');
}

function renderChains() {
  const groups = groupByBaseSerial();
  const cards = groups.length ? groups.map(g => chainCard(g)).join('') : `<div class="empty">No chains yet. Create a quotation to start the first serial chain.</div>`;
  return `<div class="card"><div class="section-title"><div><h3>One Serial Business Chain</h3><p>Each deal keeps the same base serial across all documents.</p></div><button class="btn" data-action="create-chain">Create first quotation</button></div></div>${cards}`;
}

function groupByBaseSerial() {
  const map = new Map();
  for (const d of S.documents) {
    const base = d.baseSerial || deriveBaseFromNumber(d.number) || d.chainId || 'NO-SERIAL';
    if (!map.has(base)) map.set(base, []);
    map.get(base).push(d);
  }
  return [...map.entries()].map(([base, docs]) => ({ base, docs: docs.sort((a, b) => String(a.date).localeCompare(String(b.date))) })).sort((a, b) => b.base.localeCompare(a.base));
}

function chainCard(g) {
  const customer = partyName('customer', g.docs.find(d => d.customerId)?.customerId || '');
  const total = g.docs.filter(d => d.type === 'invoice').reduce((a, d) => a + docTotals(d).total, 0);
  const steps = FLOW_TYPES.map(t => {
    const doc = g.docs.find(d => d.type === t);
    const cls = doc ? (['Accepted', 'Approved', 'Paid', 'Completed', 'Converted'].includes(doc.status) ? 'done' : 'active') : 'missing';
    const body = doc ? `<a href="#doc/${t}/${doc.id}"><strong>${DOC_TYPES[t].suffix}</strong><small>${esc(doc.number)} · ${esc(doc.status || 'Draft')}</small></a>` : `<strong>${DOC_TYPES[t].suffix}</strong><small>Not created</small>`;
    return `<div class="flow-step ${cls}">${body}</div>`;
  }).join('');
  const actions = FLOW_TYPES.filter(t => !g.docs.some(d => d.type === t)).slice(0, 2).map(t => `<button class="btn secondary small" data-action="create-doc" data-type="${t}" data-base="${esc(g.base)}">Add ${DOC_TYPES[t].suffix}</button>`).join('');
  return `<div class="chain-card"><div class="chain-head"><div><h3>${esc(g.base)}</h3><div class="muted tiny">${esc(customer || 'No customer')} · ${g.docs.length} document(s) · invoice total ${money(total, baseCurrency())}</div></div><div class="action-row">${actions}<button class="btn small" data-action="create-doc" data-type="quotation" data-base="${esc(g.base)}">New revision/doc</button></div></div><div class="flow">${steps}</div></div>`;
}

function renderParties(kind) {
  const list = kind === 'customer' ? S.customers : S.suppliers;
  const title = kind === 'customer' ? 'Customer' : 'Supplier';
  const rows = list.map(p => `<tr><td><strong>${esc(p.name)}</strong><div class="tiny muted">${esc(p.contact || '')}</div></td><td>${esc(p.phone || '')}<div class="tiny muted">${esc(p.email || '')}</div></td><td>${esc(p.city || '')}<div class="tiny muted">${esc(p.country || '')}</div></td><td>${esc(p.currency || baseCurrency())}</td><td class="right money">${kind === 'customer' ? money(customerBalance(p.id), p.currency || baseCurrency()) : money(supplierBalance(p.id), p.currency || baseCurrency())}</td><td><div class="action-row"><button class="btn secondary small" data-action="open-party" data-kind="${kind}" data-id="${p.id}">Edit</button><button class="btn danger small" data-action="delete-party" data-kind="${kind}" data-id="${p.id}">Delete</button></div></td></tr>`).join('');
  return `<div class="card"><div class="section-title"><div><h3>${title}s</h3><p>Manage ${kind} records and balances.</p></div><button class="btn" data-action="open-party" data-kind="${kind}" data-id="new">Add ${title}</button></div><div class="table-wrap"><table class="table"><thead><tr><th>Name</th><th>Contact</th><th>Location</th><th>Currency</th><th class="right">Balance</th><th>Actions</th></tr></thead><tbody>${rows || `<tr><td colspan="6" class="empty">No ${title.toLowerCase()}s yet.</td></tr>`}</tbody></table></div></div>`;
}

function openPartyModal(kind, id) {
  const list = kind === 'customer' ? S.customers : S.suppliers;
  const p = id === 'new' ? { id: '', type: kind, name: '', contact: '', email: '', phone: '', address: '', city: '', country: '', currency: baseCurrency(), taxId: '', balanceLimit: 0, notes: '' } : clone(list.find(x => x.id === id));
  if (!p) throw new Error('Record not found');
  openModal(`${id === 'new' ? 'Add' : 'Edit'} ${kind}`, `
    <form id="partyForm" data-kind="${kind}" data-id="${esc(id)}" class="form-grid two">
      <label class="span-2">Company / Name<input name="name" value="${esc(p.name)}" required></label>
      <label>Contact person<input name="contact" value="${esc(p.contact)}"></label>
      <label>Currency<select name="currency">${CURRENCIES.map(c => `<option ${c === (p.currency || baseCurrency()) ? 'selected' : ''}>${c}</option>`).join('')}</select></label>
      <label>Email<input name="email" type="email" value="${esc(p.email)}"></label>
      <label>Phone<input name="phone" value="${esc(p.phone)}"></label>
      <label class="span-2">Address<input name="address" value="${esc(p.address)}"></label>
      <label>City<input name="city" value="${esc(p.city)}"></label>
      <label>Country<input name="country" value="${esc(p.country)}"></label>
      <label>Tax/TRN/TIN<input name="taxId" value="${esc(p.taxId)}"></label>
      <label>Balance limit<input name="balanceLimit" type="number" step="0.01" value="${esc(p.balanceLimit || 0)}"></label>
      <label class="span-2">Notes<textarea name="notes">${esc(p.notes)}</textarea></label>
    </form>`, `<button class="btn secondary" data-action="close-modal">Cancel</button><button class="btn" data-action="save-party">Save</button>`);
}

async function savePartyFromModal() {
  const form = $('partyForm');
  const kind = form.dataset.kind;
  const id = form.dataset.id;
  const list = kind === 'customer' ? S.customers : S.suppliers;
  const fd = Object.fromEntries(new FormData(form));
  const rec = { ...fd, id: id === 'new' ? uid(kind === 'customer' ? 'cus' : 'sup') : id, type: kind, balanceLimit: num(fd.balanceLimit), updatedAt: nowISO() };
  if (id === 'new') { rec.createdAt = rec.updatedAt; list.push(rec); addAudit(`Create ${kind}`, kind, rec.name, rec.id); }
  else { const i = list.findIndex(x => x.id === id); rec.createdAt = list[i]?.createdAt || rec.updatedAt; list[i] = { ...list[i], ...rec }; addAudit(`Edit ${kind}`, kind, rec.name, rec.id); }
  closeModal();
  await saveData(`${titleCase(kind)} saved`);
}

async function deleteParty(kind, id) {
  if (!confirm('Delete this record?')) return;
  const list = kind === 'customer' ? S.customers : S.suppliers;
  const i = list.findIndex(x => x.id === id);
  if (i >= 0) { addAudit(`Delete ${kind}`, kind, list[i].name, id); list.splice(i, 1); await saveData('Deleted'); }
}

function renderProducts() {
  const productCount = S.products.filter(p => normalizeProductCategory(p.category) === 'product').length;
  const transportCount = S.products.filter(p => normalizeProductCategory(p.category) === 'transport').length;
  const serviceCount = S.products.filter(p => normalizeProductCategory(p.category) === 'service').length;
  const rows = S.products.map(p => `<tr><td>${categoryPill(p.category)}</td><td><strong>${esc(p.sku || '')}</strong><div class="tiny muted">HS: ${esc(p.hsCode || '—')}</div></td><td><strong>${esc(p.name)}</strong><div class="tiny muted">${esc(p.description || '')}</div></td><td>${esc(p.unit || '')}</td><td>${esc(p.warehouse || '')}</td><td class="right">${money(p.costPrice, p.currency || baseCurrency())}</td><td class="right">${money(p.salePrice, p.currency || baseCurrency())}</td><td class="right">${money(p.stockQty || 0)}</td><td><div class="action-row"><button class="btn secondary small" data-action="open-product" data-id="${p.id}">Edit</button><button class="btn danger small" data-action="delete-product" data-id="${p.id}">Delete</button></div></td></tr>`).join('');
  return `<div class="kpi-grid">${kpi('📦 Products', String(productCount), 'Goods, stock and HS codes')}${kpi('🚚 Transportation', String(transportCount), 'Freight, truck and container charges')}${kpi('🧾 Services', String(serviceCount), 'Customs, handling and documentation')}${kpi('🚀 Combined docs', String(S.documents.filter(d => normalizeDealMode(d.dealMode || inferDealMode(d.items)) === 'combined').length), 'Product + transportation documents')}</div><div class="grid-2"><div class="card premium-card"><div class="section-title"><div><h3>Choose product or transportation on every document</h3><p>One quotation can include product lines and transportation lines together. The same serial continues to PI, commercial invoice and packing list.</p></div><button class="btn" data-action="open-product" data-id="new">Add catalog item</button></div><div class="guide-steps"><div><strong>Use Product</strong> for goods and HS codes.</div><div><strong>Use Transportation</strong> for freight, road, sea, truck, container and route charges.</div><div><strong>Use Service</strong> for customs clearance, documentation and handling.</div></div></div><div class="card"><h3>Elegant catalog control</h3><p class="muted">The document editor now has quick buttons: <strong>+ Product</strong>, <strong>+ Transportation</strong>, and <strong>+ Service</strong>. Totals are split by category so you can see goods value and transport value separately.</p></div></div><div class="card"><div class="section-title"><div><h3>Products, Transportation & Services</h3><p>Catalog used in quotations, PI, invoices, purchase orders and packing lists.</p></div><button class="btn secondary small" data-action="export-csv" data-table="products">CSV</button></div><div class="table-wrap"><table class="table"><thead><tr><th>Category</th><th>SKU / HS</th><th>Name</th><th>Unit</th><th>Warehouse / Desk</th><th class="right">Cost</th><th class="right">Sale</th><th class="right">Stock</th><th>Actions</th></tr></thead><tbody>${rows || '<tr><td colspan="9" class="empty">No products yet.</td></tr>'}</tbody></table></div></div>`;
}

function openProductModal(id) {
  const p = id === 'new' ? { id: '', category: 'product', sku: '', name: '', description: '', hsCode: '', unit: 'Unit', costPrice: 0, salePrice: 0, currency: baseCurrency(), stockQty: 0, warehouse: '', notes: '' } : clone(S.products.find(x => x.id === id));
  if (!p) throw new Error('Product not found');
  p.category = normalizeProductCategory(p.category || inferProductCategory(p));
  openModal(`${id === 'new' ? 'Add' : 'Edit'} product / transportation / service`, `
    <form id="productForm" data-id="${esc(id)}" class="form-grid two">
      <label>Category<select name="category">${lineKindOptions(p.category)}</select></label>
      <label>SKU / Code<input name="sku" value="${esc(p.sku)}" placeholder="PRD-GEN / TRN-TRUCK"></label>
      <label class="span-2">Name<input name="name" value="${esc(p.name)}" required></label>
      <label class="span-2">Description<textarea name="description">${esc(p.description)}</textarea></label>
      <label>HS Code<input name="hsCode" value="${esc(p.hsCode)}" placeholder="For product items"></label>
      <label>Unit<input name="unit" value="${esc(p.unit)}" placeholder="Bag / MT / Truck / Container"></label>
      <label>Currency<select name="currency">${CURRENCIES.map(c => `<option ${c === (p.currency || baseCurrency()) ? 'selected' : ''}>${c}</option>`).join('')}</select></label>
      <label>Cost price<input name="costPrice" type="number" step="0.01" value="${esc(p.costPrice || 0)}"></label>
      <label>Sale price<input name="salePrice" type="number" step="0.01" value="${esc(p.salePrice || 0)}"></label>
      <label>Stock qty<input name="stockQty" type="number" step="0.001" value="${esc(p.stockQty || 0)}"></label>
      <label>Warehouse / Desk<input name="warehouse" value="${esc(p.warehouse)}" placeholder="Dubai Warehouse / Logistics Desk"></label>
      <label class="span-2">Notes<textarea name="notes">${esc(p.notes)}</textarea></label>
    </form>`, `<button class="btn secondary" data-action="close-modal">Cancel</button><button class="btn" data-action="save-product">Save</button>`);
}

async function saveProductFromModal() {
  const form = $('productForm');
  const id = form.dataset.id;
  const fd = Object.fromEntries(new FormData(form));
  const rec = { ...fd, id: id === 'new' ? uid('prd') : id, category: normalizeProductCategory(fd.category), costPrice: num(fd.costPrice), salePrice: num(fd.salePrice), stockQty: num(fd.stockQty), updatedAt: nowISO() };
  if (id === 'new') { rec.createdAt = rec.updatedAt; S.products.push(rec); addAudit('Create product', 'product', rec.name, rec.id); }
  else { const i = S.products.findIndex(x => x.id === id); rec.createdAt = S.products[i]?.createdAt || rec.updatedAt; S.products[i] = { ...S.products[i], ...rec }; addAudit('Edit product', 'product', rec.name, rec.id); }
  closeModal();
  await saveData('Product saved');
}

async function deleteProduct(id) {
  if (!confirm('Delete this product/service?')) return;
  const i = S.products.findIndex(x => x.id === id);
  if (i >= 0) { addAudit('Delete product', 'product', S.products[i].name, id); S.products.splice(i, 1); await saveData('Deleted'); }
}

function renderInventory() {
  const stockRows = S.products.map(p => `<tr><td>${categoryPill(p.category)}</td><td><strong>${esc(p.sku || '')}</strong></td><td>${esc(p.name)}</td><td>${esc(p.warehouse || '')}</td><td class="right">${money(p.stockQty || 0)}</td><td>${esc(p.unit || '')}</td><td class="right">${money((num(p.stockQty) * num(p.costPrice)), p.currency || baseCurrency())}</td></tr>`).join('');
  const shipDocs = S.documents.filter(d => ['invoice', 'packing', 'delivery'].includes(d.type) && (d.containerNo || d.blNo || d.pol || d.pod));
  const shipRows = shipDocs.map(d => `<tr><td><a href="#doc/${d.type}/${d.id}"><strong>${esc(d.number)}</strong></a><div class="tiny muted">${esc(d.baseSerial || '')}</div></td><td>${esc(partyName('customer', d.customerId))}</td><td>${esc(d.containerNo || '')}<div class="tiny muted">Seal: ${esc(d.sealNo || '')}</div></td><td>${esc(d.blNo || '')}</td><td>${esc(d.pol || '')} → ${esc(d.pod || '')}</td><td>${statusPill(d.status)}</td></tr>`).join('');
  return `<div class="grid-2"><div class="card"><div class="section-title"><div><h3>Warehouse Stock</h3><p>Stock quantity and estimated cost value.</p></div><button class="btn secondary small" data-action="open-product" data-id="new">Add item</button></div><div class="table-wrap"><table class="table"><thead><tr><th>Category</th><th>SKU</th><th>Name</th><th>Warehouse</th><th class="right">Qty</th><th>Unit</th><th class="right">Cost Value</th></tr></thead><tbody>${stockRows || '<tr><td colspan="7" class="empty">No stock records.</td></tr>'}</tbody></table></div></div><div class="card"><div class="section-title"><div><h3>Shipment Control</h3><p>Container, seal, BL, POL/POD and delivery status.</p></div><button class="btn secondary small" data-action="create-doc" data-type="delivery">Delivery note</button></div><div class="table-wrap"><table class="table"><thead><tr><th>Document</th><th>Customer</th><th>Container</th><th>BL</th><th>Route</th><th>Status</th></tr></thead><tbody>${shipRows || '<tr><td colspan="6" class="empty">No logistics records yet.</td></tr>'}</tbody></table></div></div></div>`;
}

function renderDocsPage(type) {
  const docs = S.documents.filter(d => d.type === type).sort((a, b) => String(b.date || '').localeCompare(String(a.date || '')));
  return `<div class="card"><div class="section-title"><div><h3>${DOC_TYPES[type].plural}</h3><p>Create, approve, convert and print ${DOC_TYPES[type].plural.toLowerCase()}.</p></div><button class="btn" data-action="create-doc" data-type="${type}">Create ${DOC_TYPES[type].title}</button></div>${documentsTable(docs)}</div>`;
}

function documentsTable(docs, compact = false) {
  if (!docs.length) return `<div class="empty">No documents yet.</div>`;
  const rows = docs.map(d => {
    const t = docTotals(d);
    const paid = paidForDoc(d.id);
    const balance = Math.max(0, t.total - paid);
    const type = DOC_TYPES[d.type] || { title: titleCase(d.type), suffix: d.type };
    return `<tr><td><a href="#doc/${d.type}/${d.id}"><strong>${esc(d.number || '(draft)')}</strong></a><div class="tiny muted">${esc(d.baseSerial || '')}</div></td><td>${type.icon || ''} ${esc(type.title)}<div class="tiny muted">${esc(dealModeLabel(d.dealMode || inferDealMode(d.items)))}</div></td><td>${esc(d.date || '')}</td><td>${esc(partyName('customer', d.customerId) || partyName('supplier', d.supplierId) || '')}</td><td class="right money">${money(t.total, d.currency || baseCurrency())}<div class="tiny muted">Due: ${money(balance, d.currency || baseCurrency())}</div></td><td>${statusPill(d.status)}</td><td>${compact ? `<a class="btn secondary small" href="#doc/${d.type}/${d.id}">Open</a>` : docRowActions(d)}</td></tr>`;
  }).join('');
  return `<div class="table-wrap"><table class="table"><thead><tr><th>Number</th><th>Type</th><th>Date</th><th>Party</th><th class="right">Total</th><th>Status</th><th>Actions</th></tr></thead><tbody>${rows}</tbody></table></div>`;
}

function docRowActions(d) {
  const next = nextDocType(d.type);
  return `<div class="action-row"><a class="btn secondary small" href="#doc/${d.type}/${d.id}">Edit</a><button class="btn secondary small" data-action="print-doc" data-id="${d.id}">Print</button>${next ? `<button class="btn good small" data-action="convert-doc" data-id="${d.id}" data-target="${next}">To ${DOC_TYPES[next].suffix}</button>` : ''}<button class="btn danger small" data-action="delete-doc" data-id="${d.id}">Delete</button></div>`;
}

function createDoc(type, base = '') {
  if (base) sessionStorage.setItem('zenith-new-doc-base', base);
  setHash(`doc/${type}/new`);
  if (getRoute().page === 'doc-editor') renderPage();
}

function renderDocEditor(type, id) {
  if (!DOC_TYPES[type]) type = 'invoice';
  let doc = id === 'new' ? newDoc(type) : clone(S.documents.find(d => d.id === id));
  if (!doc) return `<div class="card"><div class="empty">Document not found.</div></div>`;
  doc.items = (doc.items?.length ? doc.items : [blankItem('product')]).map(normalizeLineItem);
  doc.dealMode = normalizeDealMode(doc.dealMode || inferDealMode(doc.items));
  const isNew = id === 'new';
  const partyIsSupplier = type === 'purchase';
  const partyList = partyIsSupplier ? S.suppliers : S.customers;
  const partyField = partyIsSupplier ? 'supplierId' : 'customerId';
  const locked = isLockedDoc(doc);
  const itemRows = doc.items.map((it, idx) => itemRow(it, idx)).join('');
  const conversions = FLOW_TYPES.filter(t => t !== type).map(t => `<button class="btn secondary small" data-action="convert-doc" data-id="${esc(doc.id || 'new')}" data-target="${t}">Make ${DOC_TYPES[t].suffix}</button>`).join('');
  const modeCards = DEAL_MODES.map(m => `<label class="mode-card ${doc.dealMode === m.value ? 'active' : ''}"><input type="radio" name="dealModeQuick" value="${m.value}" ${doc.dealMode === m.value ? 'checked' : ''} onchange="document.querySelector('[name=dealMode]').value=this.value"><b>${m.icon} ${m.label}</b><span>${m.hint}</span></label>`).join('');
  return `
    <div class="doc-editor">
      <div class="card doc-header-card">
        <div class="section-title"><div><h3>${isNew ? 'New' : 'Edit'} ${DOC_TYPES[type].title}</h3><p>Choose Product, Transportation, or Product + Transportation. The same base serial stays through quotation, PI, commercial invoice, packing list and agreement.</p></div><div class="action-row"><button class="btn" data-action="save-doc">Save</button>${!isNew ? `<button class="btn secondary" data-action="print-doc" data-id="${doc.id}">Print</button>` : ''}</div></div>
        ${locked ? `<div class="risk">This document is locked by status/date. Admin can still revise by creating a new revision.</div>` : ''}
        <div class="mode-grid">${modeCards}</div>
        <form id="docForm" data-id="${esc(id)}" data-type="${esc(type)}" class="form-grid">
          <label>Work Type<select name="dealMode">${dealModeOptions(doc.dealMode)}</select></label>
          <label>Document Number<input name="number" value="${esc(doc.number || '')}" placeholder="Auto after save"></label>
          <label>Base Serial<input name="baseSerial" value="${esc(doc.baseSerial || '')}" placeholder="Auto chain serial"></label>
          <label>Date<input name="date" type="date" value="${esc(doc.date || today())}"></label>
          <label>Status<select name="status">${STATUS_LIST.map(s => `<option ${s === (doc.status || 'Draft') ? 'selected' : ''}>${s}</option>`).join('')}</select></label>
          <label class="span-2">${partyIsSupplier ? 'Supplier' : 'Customer'}<select name="${partyField}"><option value="">Select...</option>${partyList.map(p => `<option value="${p.id}" ${p.id === doc[partyField] ? 'selected' : ''}>${esc(p.name)}</option>`).join('')}</select></label>
          <label>Currency<select name="currency">${CURRENCIES.map(c => `<option ${c === (doc.currency || baseCurrency()) ? 'selected' : ''}>${c}</option>`).join('')}</select></label>
          <label>Exchange Rate<input class="recalc" name="exchangeRate" type="number" step="0.000001" value="${esc(doc.exchangeRate || 1)}"></label>
          <label>Valid Until<input name="validUntil" type="date" value="${esc(doc.validUntil || (type === 'quotation' ? plusDays(14) : ''))}"></label>
          <label>Due Date<input name="dueDate" type="date" value="${esc(doc.dueDate || (type === 'invoice' ? plusDays(7) : ''))}"></label>
          <label>Incoterm<input name="incoterm" value="${esc(doc.incoterm || '')}" placeholder="FOB / CIF / EXW"></label>
          <label>Discount<input class="recalc" name="discount" type="number" step="0.01" value="${esc(doc.discount || 0)}"></label>
          <label>Shipping / Other<input class="recalc" name="shipping" type="number" step="0.01" value="${esc(doc.shipping || 0)}"></label>
          <label>Tax Rate %<input class="recalc" name="taxRate" type="number" step="0.01" value="${esc(doc.taxRate || 0)}"></label>
          <label>POL<input name="pol" value="${esc(doc.pol || '')}" placeholder="Port of loading"></label>
          <label>POD<input name="pod" value="${esc(doc.pod || '')}" placeholder="Port of discharge"></label>
          <label>Container No<input name="containerNo" value="${esc(doc.containerNo || '')}"></label>
          <label>Seal No<input name="sealNo" value="${esc(doc.sealNo || '')}"></label>
          <label>BL No<input name="blNo" value="${esc(doc.blNo || '')}"></label>
          <label>Vessel<input name="vessel" value="${esc(doc.vessel || '')}"></label>
          <label>Voyage<input name="voyage" value="${esc(doc.voyage || '')}"></label>
          <label>Truck / Driver<input name="truckDriver" value="${esc(doc.truckDriver || '')}"></label>
          <label>Customs Declaration<input name="customsNo" value="${esc(doc.customsNo || '')}"></label>
          <label>Revision<input name="revision" type="number" step="1" value="${esc(doc.revision || 0)}"></label>
          <label class="span-2">Customer Approval Token<input name="customerToken" value="${esc(doc.customerToken || '')}" placeholder="Auto after save"></label>
          <label class="full">Notes<textarea name="notes">${esc(doc.notes || S.company.defaultNotes || '')}</textarea></label>
          <label class="full">Terms<textarea name="terms">${esc(doc.terms || S.company.defaultTerms || '')}</textarea></label>
          <input type="hidden" name="id" value="${esc(doc.id || '')}"><input type="hidden" name="type" value="${esc(type)}"><input type="hidden" name="chainId" value="${esc(doc.chainId || '')}"><input type="hidden" name="sourceDocId" value="${esc(doc.sourceDocId || '')}">
        </form>
        <div class="items-toolbar"><div><h3>Product & Transportation Lines</h3><p class="tiny muted">Mix physical products, freight/trucking/container charges, customs services and other charges in one document.</p></div><div class="action-row"><button class="btn secondary small" data-action="add-line-kind" data-kind="product">+ Product</button><button class="btn secondary small" data-action="add-line-kind" data-kind="transport">+ Transportation</button><button class="btn secondary small" data-action="add-line-kind" data-kind="service">+ Service</button><button class="btn secondary small" data-action="add-line">+ Custom</button></div></div>
        <div class="table-wrap items-wrap"><table class="table" id="itemsTable"><thead><tr><th>Line Type</th><th>Catalog</th><th>Description</th><th>HS</th><th>Unit</th><th>Qty</th><th>Unit Price</th><th>Net Wt</th><th>Gross Wt</th><th>Packages</th><th></th></tr></thead><tbody>${itemRows}</tbody></table></div>
      </div>
      <aside class="side-stack">
        <div class="card compact"><h3>Totals</h3><div class="totals-box" id="editorTotals"></div></div>
        <div class="card compact"><h3>Workflow</h3><div class="action-row"><button class="btn secondary small" data-action="mark-doc-status" data-id="${esc(doc.id || '')}" data-status="Sent">Mark Sent</button><button class="btn good small" data-action="mark-doc-status" data-id="${esc(doc.id || '')}" data-status="Accepted">Accept</button><button class="btn good small" data-action="mark-doc-status" data-id="${esc(doc.id || '')}" data-status="Approved">Approve</button><button class="btn warn small" data-action="mark-doc-status" data-id="${esc(doc.id || '')}" data-status="Cancelled">Cancel</button></div><hr>${conversions}</div>
        <div class="card compact"><h3>Serial</h3><div class="serial-preview" style="font-size:16px">${esc(doc.baseSerial || 'Auto on save')}</div><p class="tiny muted">Current document suffix: ${DOC_TYPES[type].suffix}. Revision R${num(doc.revision || 0)}.</p>${doc.id ? `<button class="btn secondary small" data-action="copy-approval" data-id="${doc.id}">Copy approval text</button>` : ''}</div>
        <div class="card compact"><h3>Accounting</h3><p class="tiny muted">Paid: ${money(doc.id ? paidForDoc(doc.id) : 0, doc.currency || baseCurrency())}</p>${doc.id ? `<button class="btn secondary small" data-action="open-payment" data-id="new" data-doc="${doc.id}">Add payment</button>` : '<p class="tiny muted">Save first to add payment.</p>'}</div>
      </aside>
    </div>`;
}

function newDoc(type) {
  const base = sessionStorage.getItem('zenith-new-doc-base') || '';
  sessionStorage.removeItem('zenith-new-doc-base');
  const partyIsSupplier = type === 'purchase';
  const firstParty = partyIsSupplier ? S.suppliers[0]?.id : S.customers[0]?.id;
  const d = { id: '', type, number: '', baseSerial: base, chainId: base, revision: 0, date: today(), validUntil: type === 'quotation' ? plusDays(14) : '', dueDate: type === 'invoice' ? plusDays(7) : '', dealMode: 'combined', status: 'Draft', customerId: partyIsSupplier ? '' : (firstParty || ''), supplierId: partyIsSupplier ? (firstParty || '') : '', currency: baseCurrency(), exchangeRate: 1, taxRate: 0, discount: 0, shipping: 0, incoterm: '', pol: '', pod: '', containerNo: '', sealNo: '', blNo: '', vessel: '', voyage: '', truckDriver: '', customsNo: '', notes: S.company.defaultNotes || '', terms: S.company.defaultTerms || '', customerToken: '', sourceDocId: '', items: [blankItem()] };
  if (base) d.number = makeDocNumber(base, type, 0);
  return d;
}
function blankItem(kind = 'product') {
  kind = normalizeLineKind(kind);
  return { itemKind: kind, category: kind, productId: '', description: kind === 'transport' ? 'Transportation / freight charge' : kind === 'service' ? 'Service charge' : '', hsCode: '', unit: kind === 'transport' ? 'Trip' : kind === 'service' ? 'Service' : 'Unit', qty: 1, unitPrice: 0, netWeight: 0, grossWeight: 0, packages: 0 };
}
function itemRow(it, idx) {
  it = normalizeLineItem(it);
  return `<tr class="item-row">
    <td><select class="item-input recalc line-kind" data-field="itemKind">${lineKindOptions(it.itemKind)}</select></td>
    <td><select class="product-select item-input" data-field="productId">${productOptions(it.productId)}</select></td>
    <td><input class="item-input line-desc" data-field="description" value="${esc(it.description || '')}"></td>
    <td><input class="item-input" data-field="hsCode" value="${esc(it.hsCode || '')}"></td>
    <td><input class="item-input" data-field="unit" value="${esc(it.unit || '')}"></td>
    <td><input class="item-input recalc" data-field="qty" type="number" step="0.001" value="${esc(it.qty || 0)}"></td>
    <td><input class="item-input recalc" data-field="unitPrice" type="number" step="0.01" value="${esc(it.unitPrice || 0)}"></td>
    <td><input class="item-input" data-field="netWeight" type="number" step="0.001" value="${esc(it.netWeight || 0)}"></td>
    <td><input class="item-input" data-field="grossWeight" type="number" step="0.001" value="${esc(it.grossWeight || 0)}"></td>
    <td><input class="item-input" data-field="packages" type="number" step="0.001" value="${esc(it.packages || 0)}"></td>
    <td><button class="btn danger small" type="button" data-action="remove-line">×</button></td>
  </tr>`;
}
function addItemLine(kind = 'product') { $('itemsTable').querySelector('tbody').insertAdjacentHTML('beforeend', itemRow(blankItem(kind), document.querySelectorAll('.item-row').length)); recalcEditorTotals(); }
function fillLineFromProduct(sel) {
  const p = S.products.find(x => x.id === sel.value);
  if (!p) return;
  const row = sel.closest('tr');
  row.querySelector('[data-field="description"]').value = p.description || p.name || '';
  row.querySelector('[data-field="hsCode"]').value = p.hsCode || '';
  row.querySelector('[data-field="unit"]').value = p.unit || '';
  row.querySelector('[data-field="unitPrice"]').value = p.salePrice || 0;
  const kindField = row.querySelector('[data-field="itemKind"]');
  if (kindField) kindField.value = normalizeProductCategory(p.category || inferProductCategory(p));
  recalcEditorTotals();
}
function readDocForm() {
  const form = $('docForm');
  if (!form) throw new Error('Document form not found');
  const fd = Object.fromEntries(new FormData(form));
  const existing = fd.id ? S.documents.find(d => d.id === fd.id) : null;
  let base = String(fd.baseSerial || '').trim();
  if (!base) base = existing?.baseSerial || allocateBaseSerial();
  const type = fd.type;
  const revision = num(fd.revision || 0);
  const id = fd.id || uid('doc');
  const doc = {
    ...(existing || {}), ...fd, id, type, dealMode: normalizeDealMode(fd.dealMode || inferDealMode([])), baseSerial: base, chainId: fd.chainId || base, revision, exchangeRate: num(fd.exchangeRate) || 1, taxRate: num(fd.taxRate), discount: num(fd.discount), shipping: num(fd.shipping), updatedAt: nowISO(),
    items: [...document.querySelectorAll('.item-row')].map(row => {
      const get = f => row.querySelector(`[data-field="${f}"]`)?.value || '';
      const itemKind = normalizeLineKind(get('itemKind') || inferItemKind({ description: get('description'), unit: get('unit') }));
      return { itemKind, category: itemKind, productId: get('productId'), description: get('description'), hsCode: get('hsCode'), unit: get('unit'), qty: num(get('qty')), unitPrice: num(get('unitPrice')), netWeight: num(get('netWeight')), grossWeight: num(get('grossWeight')), packages: num(get('packages')) };
    }).filter(it => it.description || it.productId || it.qty || it.unitPrice)
  };
  doc.dealMode = normalizeDealMode(doc.dealMode || inferDealMode(doc.items));
  if (!doc.number) doc.number = makeDocNumber(base, type, revision);
  if (!doc.customerToken) doc.customerToken = randomTokenShort();
  if (!doc.createdAt) doc.createdAt = doc.updatedAt;
  if (!doc.createdBy) doc.createdBy = USER?.username || 'local';
  doc.updatedBy = USER?.username || 'local';
  doc.verificationCode = generateVerification(doc);
  return doc;
}
async function saveDocFromEditor() {
  const doc = readDocForm();
  if (S.settings.lockBeforeDate && doc.date && doc.date < S.settings.lockBeforeDate && !isAdmin()) throw new Error('This date is locked by accounting settings. Ask admin to unlock.');
  const i = S.documents.findIndex(d => d.id === doc.id);
  if (i >= 0) S.documents[i] = doc; else S.documents.push(doc);
  addAudit(i >= 0 ? 'Edit document' : 'Create document', 'document', `${doc.number} ${DOC_TYPES[doc.type]?.title || doc.type}`, doc.id);
  await saveData('Document saved');
  setHash(`doc/${doc.type}/${doc.id}`);
}
async function deleteDoc(id) {
  if (!confirm('Delete this document? Its serial remains visible in the audit trail.')) return;
  const i = S.documents.findIndex(d => d.id === id);
  if (i >= 0) { addAudit('Delete document', 'document', S.documents[i].number, id); S.documents.splice(i, 1); await saveData('Document deleted'); setHash('chains'); }
}
function isLockedDoc(doc) { return ['Accepted', 'Approved', 'Converted', 'Paid', 'Completed', 'Cancelled'].includes(doc.status) || (S.settings.lockBeforeDate && doc.date && doc.date < S.settings.lockBeforeDate); }
async function markDocStatus(id, status) {
  if (!id) { toast('Save document first, then change workflow status.', 'error'); return; }
  const doc = S.documents.find(d => d.id === id);
  if (!doc) throw new Error('Document not found');
  doc.status = status;
  doc.updatedAt = nowISO();
  if (status === 'Accepted') doc.acceptedAt = nowISO();
  if (status === 'Approved') doc.approvedAt = nowISO();
  addAudit(`Status: ${status}`, 'document', doc.number, id);
  await saveData('Status updated');
}
function nextDocType(type) {
  const idx = FLOW_TYPES.indexOf(type);
  return idx >= 0 && idx < FLOW_TYPES.length - 1 ? FLOW_TYPES[idx + 1] : '';
}
async function convertDoc(id, target) {
  if (id === 'new' || !id) { toast('Save the document first, then convert.', 'error'); return; }
  const src = S.documents.find(d => d.id === id);
  if (!src) throw new Error('Source document not found');
  const existing = S.documents.filter(d => d.baseSerial === src.baseSerial && d.type === target);
  let revision = 0;
  if (existing.length) {
    if (!confirm(`${DOC_TYPES[target].title} already exists in this chain. Create a new revision?`)) return;
    revision = Math.max(...existing.map(d => num(d.revision))) + 1;
  }
  const d = clone(src);
  d.id = uid('doc'); d.type = target; d.revision = revision; d.number = makeDocNumber(src.baseSerial, target, revision); d.date = today(); d.status = 'Draft'; d.sourceDocId = src.id; d.createdAt = nowISO(); d.updatedAt = d.createdAt; d.createdBy = USER?.username || 'local'; d.updatedBy = d.createdBy; d.customerToken = randomTokenShort(); d.verificationCode = generateVerification(d);
  if (target === 'packing') { d.taxRate = 0; d.discount = 0; d.shipping = 0; }
  S.documents.push(d);
  if (src.status !== 'Accepted' && src.type === 'quotation') src.status = 'Converted';
  addAudit('Convert document', 'document', `${src.number} → ${d.number}`, d.id);
  await saveData(`Converted to ${DOC_TYPES[target].title}`);
  setHash(`doc/${target}/${d.id}`);
}
function recalcEditorTotals() {
  const box = $('editorTotals');
  if (!box) return;
  const lines = [...document.querySelectorAll('.item-row')].map(row => ({ kind: normalizeLineKind(row.querySelector('[data-field="itemKind"]')?.value), qty: num(row.querySelector('[data-field="qty"]')?.value), unitPrice: num(row.querySelector('[data-field="unitPrice"]')?.value) }));
  const subtotal = lines.reduce((a, it) => a + it.qty * it.unitPrice, 0);
  const by = Object.fromEntries(LINE_KINDS.map(k => [k.value, lines.filter(it => it.kind === k.value).reduce((a, it) => a + it.qty * it.unitPrice, 0)]));
  const taxRate = num(document.querySelector('[name="taxRate"]')?.value);
  const discount = num(document.querySelector('[name="discount"]')?.value);
  const shipping = num(document.querySelector('[name="shipping"]')?.value);
  const taxable = Math.max(0, subtotal - discount + shipping);
  const tax = taxable * taxRate / 100;
  const total = taxable + tax;
  const cur = document.querySelector('[name="currency"]')?.value || baseCurrency();
  box.innerHTML = `<div class="totals-row"><span>Products</span><strong>${money(by.product, cur)}</strong></div><div class="totals-row"><span>Transportation</span><strong>${money(by.transport, cur)}</strong></div><div class="totals-row"><span>Services/Charges</span><strong>${money(by.service + by.charge, cur)}</strong></div><div class="totals-row"><span>Subtotal</span><strong>${money(subtotal, cur)}</strong></div><div class="totals-row"><span>Tax</span><strong>${money(tax, cur)}</strong></div><div class="totals-row grand"><span>Total</span><strong>${money(total, cur)}</strong></div>`;
}
function docTotals(d) {
  const lines = (d.items || []).map(normalizeLineItem);
  const subtotal = lines.reduce((a, it) => a + num(it.qty) * num(it.unitPrice), 0);
  const product = lines.filter(it => it.itemKind === 'product').reduce((a, it) => a + num(it.qty) * num(it.unitPrice), 0);
  const transport = lines.filter(it => it.itemKind === 'transport').reduce((a, it) => a + num(it.qty) * num(it.unitPrice), 0);
  const service = lines.filter(it => ['service','charge'].includes(it.itemKind)).reduce((a, it) => a + num(it.qty) * num(it.unitPrice), 0);
  const taxable = Math.max(0, subtotal - num(d.discount) + num(d.shipping));
  const tax = taxable * num(d.taxRate) / 100;
  return { subtotal: cleanMoney(subtotal), product: cleanMoney(product), transport: cleanMoney(transport), service: cleanMoney(service), tax: cleanMoney(tax), total: cleanMoney(taxable + tax) };
}

function renderAccounting() {
  const rows = S.payments.slice().sort((a, b) => String(b.date).localeCompare(String(a.date))).map(p => `<tr><td>${esc(p.date || '')}</td><td>${statusPill(p.type || '')}</td><td>${esc(partyName(p.partyType, p.partyId))}</td><td>${esc(docById(p.documentId)?.number || '')}</td><td class="right money">${money(p.amount, p.currency || baseCurrency())}</td><td>${esc(p.account || '')}</td><td>${esc(p.method || '')}</td><td><div class="action-row"><button class="btn secondary small" data-action="open-payment" data-id="${p.id}">Edit</button><button class="btn danger small" data-action="delete-payment" data-id="${p.id}">Delete</button></div></td></tr>`).join('');
  const received = S.payments.filter(p => p.type === 'received').reduce((a, p) => a + num(p.amount), 0);
  const paid = S.payments.filter(p => p.type === 'paid').reduce((a, p) => a + num(p.amount), 0);
  return `<div class="kpi-grid">${kpi('Received', money(received, baseCurrency()), 'Customer receipts')}${kpi('Paid', money(paid, baseCurrency()), 'Supplier/payment vouchers')}${kpi('Cash balance', money(received - paid - S.expenses.reduce((a,e)=>a+num(e.amount),0), baseCurrency()), 'Simple cash estimate')}${kpi('Unpaid invoices', String(S.documents.filter(d => d.type === 'invoice' && docTotals(d).total > paidForDoc(d.id)).length), 'Need collection')}</div><div class="card"><div class="section-title"><div><h3>Receipts & Payments</h3><p>Record customer receipts and supplier payments.</p></div><button class="btn" data-action="open-payment" data-id="new">Add payment</button></div><div class="table-wrap"><table class="table"><thead><tr><th>Date</th><th>Type</th><th>Party</th><th>Document</th><th class="right">Amount</th><th>Account</th><th>Method</th><th>Actions</th></tr></thead><tbody>${rows || '<tr><td colspan="8" class="empty">No payments yet.</td></tr>'}</tbody></table></div></div>`;
}
function openPaymentModal(id, docId = '') {
  const p = id === 'new' ? { id: '', type: 'received', date: today(), partyType: 'customer', partyId: '', documentId: docId, currency: baseCurrency(), amount: 0, exchangeRate: 1, account: 'Cash', method: 'Cash', reference: '', notes: '' } : clone(S.payments.find(x => x.id === id));
  if (!p) throw new Error('Payment not found');
  const doc = docById(docId || p.documentId);
  if (doc) { p.partyId = doc.customerId || doc.supplierId || p.partyId; p.partyType = doc.supplierId ? 'supplier' : 'customer'; p.currency = doc.currency || p.currency; p.amount = p.amount || Math.max(0, docTotals(doc).total - paidForDoc(doc.id)); }
  const partyOptions = [...S.customers.map(x => ({ ...x, kind: 'customer' })), ...S.suppliers.map(x => ({ ...x, kind: 'supplier' }))].map(x => `<option value="${x.kind}:${x.id}" ${(p.partyType + ':' + p.partyId) === (x.kind + ':' + x.id) ? 'selected' : ''}>${x.kind === 'customer' ? 'Customer' : 'Supplier'} · ${esc(x.name)}</option>`).join('');
  openModal(`${id === 'new' ? 'Add' : 'Edit'} payment`, `<form id="paymentForm" data-id="${esc(id)}" class="form-grid two"><label>Type<select name="type"><option value="received" ${p.type === 'received' ? 'selected' : ''}>Received</option><option value="paid" ${p.type === 'paid' ? 'selected' : ''}>Paid</option></select></label><label>Date<input name="date" type="date" value="${esc(p.date)}"></label><label class="span-2">Party<select name="partyCombo"><option value=":">Select...</option>${partyOptions}</select></label><label class="span-2">Document<select name="documentId"><option value="">No document</option>${S.documents.map(d => `<option value="${d.id}" ${d.id === p.documentId ? 'selected' : ''}>${esc(d.number)} · ${esc(partyName('customer', d.customerId) || partyName('supplier', d.supplierId))}</option>`).join('')}</select></label><label>Currency<select name="currency">${CURRENCIES.map(c => `<option ${c === (p.currency || baseCurrency()) ? 'selected' : ''}>${c}</option>`).join('')}</select></label><label>Amount<input name="amount" type="number" step="0.01" value="${esc(p.amount || 0)}"></label><label>Exchange Rate<input name="exchangeRate" type="number" step="0.000001" value="${esc(p.exchangeRate || 1)}"></label><label>Account<input name="account" value="${esc(p.account || '')}"></label><label>Method<input name="method" value="${esc(p.method || '')}"></label><label>Reference<input name="reference" value="${esc(p.reference || '')}"></label><label class="span-2">Notes<textarea name="notes">${esc(p.notes || '')}</textarea></label></form>`, `<button class="btn secondary" data-action="close-modal">Cancel</button><button class="btn" data-action="save-payment">Save</button>`);
}
async function savePaymentFromModal() {
  const form = $('paymentForm');
  const id = form.dataset.id;
  const fd = Object.fromEntries(new FormData(form));
  const [partyType, partyId] = String(fd.partyCombo || ':').split(':');
  const rec = { ...fd, id: id === 'new' ? uid('pay') : id, partyType, partyId, amount: num(fd.amount), exchangeRate: num(fd.exchangeRate) || 1, updatedAt: nowISO() };
  delete rec.partyCombo;
  if (id === 'new') { rec.createdAt = rec.updatedAt; S.payments.push(rec); addAudit('Create payment', 'payment', `${rec.type} ${rec.amount}`, rec.id); }
  else { const i = S.payments.findIndex(x => x.id === id); rec.createdAt = S.payments[i]?.createdAt || rec.updatedAt; S.payments[i] = { ...S.payments[i], ...rec }; addAudit('Edit payment', 'payment', `${rec.type} ${rec.amount}`, rec.id); }
  updatePaidStatuses(); closeModal(); await saveData('Payment saved');
}
async function deletePayment(id) { if (!confirm('Delete this payment?')) return; const i = S.payments.findIndex(x => x.id === id); if (i >= 0) { addAudit('Delete payment', 'payment', S.payments[i].reference || id, id); S.payments.splice(i, 1); updatePaidStatuses(); await saveData('Payment deleted'); } }

function renderExpenses() {
  const rows = S.expenses.slice().sort((a, b) => String(b.date).localeCompare(String(a.date))).map(e => `<tr><td>${esc(e.date)}</td><td>${esc(e.category)}</td><td>${esc(e.vendor || '')}</td><td class="right money">${money(e.amount, e.currency || baseCurrency())}</td><td>${esc(e.account || '')}</td><td>${esc(e.notes || '')}</td><td><div class="action-row"><button class="btn secondary small" data-action="open-expense" data-id="${e.id}">Edit</button><button class="btn danger small" data-action="delete-expense" data-id="${e.id}">Delete</button></div></td></tr>`).join('');
  return `<div class="card"><div class="section-title"><div><h3>Expenses</h3><p>Track office, logistics, customs, bank and operational costs.</p></div><button class="btn" data-action="open-expense" data-id="new">Add expense</button></div><div class="table-wrap"><table class="table"><thead><tr><th>Date</th><th>Category</th><th>Vendor</th><th class="right">Amount</th><th>Account</th><th>Notes</th><th>Actions</th></tr></thead><tbody>${rows || '<tr><td colspan="7" class="empty">No expenses yet.</td></tr>'}</tbody></table></div></div>`;
}
function openExpenseModal(id) {
  const e = id === 'new' ? { id: '', date: today(), category: 'General', vendor: '', currency: baseCurrency(), amount: 0, exchangeRate: 1, account: 'Cash', notes: '' } : clone(S.expenses.find(x => x.id === id));
  if (!e) throw new Error('Expense not found');
  openModal(`${id === 'new' ? 'Add' : 'Edit'} expense`, `<form id="expenseForm" data-id="${esc(id)}" class="form-grid two"><label>Date<input name="date" type="date" value="${esc(e.date)}"></label><label>Category<input name="category" value="${esc(e.category)}"></label><label>Vendor<input name="vendor" value="${esc(e.vendor || '')}"></label><label>Currency<select name="currency">${CURRENCIES.map(c => `<option ${c === (e.currency || baseCurrency()) ? 'selected' : ''}>${c}</option>`).join('')}</select></label><label>Amount<input name="amount" type="number" step="0.01" value="${esc(e.amount || 0)}"></label><label>Exchange Rate<input name="exchangeRate" type="number" step="0.000001" value="${esc(e.exchangeRate || 1)}"></label><label>Account<input name="account" value="${esc(e.account || '')}"></label><label class="span-2">Notes<textarea name="notes">${esc(e.notes || '')}</textarea></label></form>`, `<button class="btn secondary" data-action="close-modal">Cancel</button><button class="btn" data-action="save-expense">Save</button>`);
}
async function saveExpenseFromModal() {
  const form = $('expenseForm'); const id = form.dataset.id; const fd = Object.fromEntries(new FormData(form));
  const rec = { ...fd, id: id === 'new' ? uid('exp') : id, amount: num(fd.amount), exchangeRate: num(fd.exchangeRate) || 1, updatedAt: nowISO() };
  if (id === 'new') { rec.createdAt = rec.updatedAt; S.expenses.push(rec); addAudit('Create expense', 'expense', `${rec.category} ${rec.amount}`, rec.id); }
  else { const i = S.expenses.findIndex(x => x.id === id); rec.createdAt = S.expenses[i]?.createdAt || rec.updatedAt; S.expenses[i] = { ...S.expenses[i], ...rec }; addAudit('Edit expense', 'expense', `${rec.category} ${rec.amount}`, rec.id); }
  closeModal(); await saveData('Expense saved');
}
async function deleteExpense(id) { if (!confirm('Delete this expense?')) return; const i = S.expenses.findIndex(x => x.id === id); if (i >= 0) { addAudit('Delete expense', 'expense', S.expenses[i].category, id); S.expenses.splice(i, 1); await saveData('Expense deleted'); } }

function renderReports() {
  const unpaid = S.documents.filter(d => d.type === 'invoice' && docTotals(d).total > paidForDoc(d.id));
  const unpaidRows = unpaid.map(d => `<tr><td><a href="#doc/${d.type}/${d.id}">${esc(d.number)}</a></td><td>${esc(partyName('customer', d.customerId))}</td><td>${esc(d.dueDate || '')}</td><td class="right">${money(docTotals(d).total, d.currency)}</td><td class="right">${money(paidForDoc(d.id), d.currency)}</td><td class="right money">${money(docTotals(d).total - paidForDoc(d.id), d.currency)}</td></tr>`).join('');
  const auditRows = S.auditLogs.slice(-80).reverse().map(a => `<tr><td>${esc(formatDateTime(a.time))}</td><td>${esc(a.user || '')}</td><td>${esc(a.action || '')}</td><td>${esc(a.entity || '')}</td><td>${esc(a.details || '')}</td></tr>`).join('');
  return `<div class="grid-2"><div class="card"><div class="section-title"><div><h3>Unpaid invoices</h3><p>Collection list and aging control.</p></div><button class="btn secondary small" data-action="export-csv" data-table="documents">CSV</button></div><div class="table-wrap"><table class="table"><thead><tr><th>Invoice</th><th>Customer</th><th>Due</th><th class="right">Total</th><th class="right">Paid</th><th class="right">Balance</th></tr></thead><tbody>${unpaidRows || '<tr><td colspan="6" class="empty">No unpaid invoices.</td></tr>'}</tbody></table></div></div><div class="card"><div class="section-title"><div><h3>VAT / Tax Summary</h3><p>Draft summary based on document tax rates.</p></div></div>${renderTaxSummary()}</div></div><div class="card"><div class="section-title"><div><h3>Audit trail</h3><p>Changes, logins, approvals and important actions.</p></div><button class="btn secondary small" data-action="export-csv" data-table="audit">Audit CSV</button></div><div class="table-wrap"><table class="table"><thead><tr><th>Time</th><th>User</th><th>Action</th><th>Entity</th><th>Details</th></tr></thead><tbody>${auditRows || '<tr><td colspan="5" class="empty">No audit log yet.</td></tr>'}</tbody></table></div></div>`;
}
function renderTaxSummary() {
  const inv = S.documents.filter(d => d.type === 'invoice');
  const taxable = inv.reduce((a, d) => a + docTotals(d).subtotal, 0);
  const tax = inv.reduce((a, d) => a + docTotals(d).tax, 0);
  const total = inv.reduce((a, d) => a + docTotals(d).total, 0);
  return `<div class="kpi-grid" style="grid-template-columns:1fr"><div class="kpi"><span>Taxable sales</span><strong>${money(taxable, baseCurrency())}</strong></div><div class="kpi"><span>Output tax</span><strong>${money(tax, baseCurrency())}</strong></div><div class="kpi"><span>Total invoices</span><strong>${money(total, baseCurrency())}</strong></div></div><p class="tiny muted">Use accountant review before filing official tax/VAT returns.</p>`;
}

function renderAssistant() {
  return `<div class="card"><div class="section-title"><div><h3>Smart Command Center</h3><p>Type simple commands like “unpaid invoices”, “profit”, “next serial”, “customer balance”, or “server setup”.</p></div></div><div class="command-box"><input id="commandInput" placeholder="Example: show unpaid invoices"><button class="btn" data-action="run-command">Run</button></div><br><div id="assistantAnswer" class="assistant-answer">Ready. Try: unpaid invoices, profit, next serial, pending approvals, server setup.</div></div>`;
}
function runCommand() {
  const q = $('commandInput').value.toLowerCase();
  const ans = $('assistantAnswer');
  if (!q.trim()) return;
  if (q.includes('unpaid')) {
    const unpaid = S.documents.filter(d => d.type === 'invoice' && docTotals(d).total > paidForDoc(d.id));
    ans.textContent = unpaid.length ? unpaid.map(d => `${d.number} · ${partyName('customer', d.customerId)} · balance ${money(docTotals(d).total - paidForDoc(d.id), d.currency)}`).join('\n') : 'No unpaid invoices.';
  } else if (q.includes('profit')) {
    const sales = S.documents.filter(d => d.type === 'invoice').reduce((a,d)=>a+docTotals(d).total,0);
    const paid = S.payments.filter(p => p.type === 'paid').reduce((a,p)=>a+num(p.amount),0);
    const exp = S.expenses.reduce((a,e)=>a+num(e.amount),0);
    ans.textContent = `Estimated profit: ${money(sales - paid - exp, baseCurrency())}\nSales: ${money(sales, baseCurrency())}\nSupplier paid: ${money(paid, baseCurrency())}\nExpenses: ${money(exp, baseCurrency())}`;
  } else if (q.includes('serial')) {
    ans.textContent = `Next base serial: ${peekBaseSerial()}\nQuotation will become ${makeDocNumber(peekBaseSerial(), 'quotation', 0)}.`;
  } else if (q.includes('pending')) {
    const p = S.documents.filter(d => String(d.status).toLowerCase().includes('pending'));
    ans.textContent = p.length ? p.map(d => `${d.number} · ${d.status}`).join('\n') : 'No pending approvals.';
  } else if (q.includes('server')) {
    ans.textContent = 'Server setup: run with ZENITH_ERP_ADDR=0.0.0.0:8080 and ZENITH_ERP_BROWSER=0. Employees open the server URL, click Create employee account, then admin approves them in Employees & Access.';
  } else if (q.includes('customer')) {
    ans.textContent = S.customers.map(c => `${c.name}: ${money(customerBalance(c.id), c.currency || baseCurrency())}`).join('\n') || 'No customers.';
  } else {
    ans.textContent = 'I can check unpaid invoices, profit, next serial, pending approvals, customer balances, and server setup. More commands can be added later.';
  }
}

function renderLetterhead() {
  return `<div class="grid-2"><div class="card"><div class="section-title"><div><h3>Letterhead Settings</h3><p>This matches your uploaded Zenith Eclipse sample: logo top-left, document number top-right, To section, and footer.</p></div><button class="btn" data-action="save-company">Save design</button></div><form id="companyForm" class="form-grid two"><label>Company Name<input name="name" value="${esc(S.company.name)}"></label><label>Legal Name<input name="legalName" value="${esc(S.company.legalName || '')}"></label><label class="span-2">Slogan<input name="slogan" value="${esc(S.company.slogan || '')}"></label><label>Email<input name="email" value="${esc(S.company.email || '')}"></label><label>Phone<input name="phone" value="${esc(S.company.phone || '')}"></label><label class="span-2">Address<input name="address" value="${esc(S.company.address || '')}"></label><label>Website<input name="website" value="${esc(S.company.website || '')}"></label><label>Tax/TRN<input name="taxId" value="${esc(S.company.taxId || '')}"></label><label>Base Currency<select name="baseCurrency">${CURRENCIES.map(c => `<option ${c === baseCurrency() ? 'selected' : ''}>${c}</option>`).join('')}</select></label><label>Stamp Text<input name="stampText" value="${esc(S.company.stampText || '')}"></label><label class="span-2">Default Notes<textarea name="defaultNotes">${esc(S.company.defaultNotes || '')}</textarea></label><label class="span-2">Default Terms<textarea name="defaultTerms">${esc(S.company.defaultTerms || '')}</textarea></label><label>Logo upload<input class="file-input" type="file" accept="image/*" data-image-upload="logoData"></label><label>Stamp upload<input class="file-input" type="file" accept="image/*" data-image-upload="stampData"></label><label>Bank Name<input name="bankName" value="${esc(S.company.bankName || '')}"></label><label>Bank Account<input name="bankAccount" value="${esc(S.company.bankAccount || '')}"></label><label>IBAN<input name="bankIban" value="${esc(S.company.bankIban || '')}"></label><label>SWIFT<input name="bankSwift" value="${esc(S.company.bankSwift || '')}"></label></form></div><div class="card"><h3>Live Letterhead Preview</h3>${letterPreview()}</div></div>`;
}
function letterPreview() {
  return `<div class="sample-letter"><div class="sample-letter-header"><div><div class="sample-brand"><img src="${esc(logoSrc())}"><div><h2>${esc(S.company.name)}</h2><small>${esc(S.company.slogan || '')}</small></div></div><div class="sample-to"><b>To:</b><div><strong>Haroon Rezwan and Bradaran Amar khil Trade Co</strong><br><strong>MR. Abdul Qasum</strong><br>Shop# 19, Faisal Sharif Market, Mandawi, Kabul AFG</div></div></div><div class="sample-meta">Quotation# ${esc(makeDocNumber(peekBaseSerial(), 'quotation', 0))}<br><span class="tiny">Date: ${today()}</span></div></div><div class="sample-body">Document body area</div><div class="sample-footer"><img class="leaf" src="${esc(leafSrc())}"><div>✉ ${esc(S.company.email)}</div><div>📍 ${esc(S.company.address)}</div><div>☎ ${esc(S.company.phone)}</div><div><strong>Find More at</strong><br>${esc(S.company.website)}</div></div></div>`;
}
async function saveCompanySettings() {
  const fd = Object.fromEntries(new FormData($('companyForm')));
  S.company = { ...S.company, ...fd };
  addAudit('Update letterhead', 'settings', 'Company letterhead changed');
  await saveData('Letterhead saved');
}
function readImageUpload(input) {
  const field = input.dataset.imageUpload;
  const file = input.files?.[0];
  if (!file) return;
  const reader = new FileReader();
  reader.onload = () => { S.company[field] = reader.result; toast('Image loaded. Click Save design.', 'ok'); renderPage(); };
  reader.readAsDataURL(file);
}

function renderSerials() {
  const chains = groupByBaseSerial().slice(0, 20).map(g => `<tr><td><strong>${esc(g.base)}</strong></td><td>${g.docs.length}</td><td>${g.docs.map(d => DOC_TYPES[d.type]?.suffix || d.type).join(' → ')}</td><td>${esc(partyName('customer', g.docs[0]?.customerId || ''))}</td></tr>`).join('');
  return `<div class="grid-2"><div class="card"><div class="section-title"><div><h3>Serial Number Manager</h3><p>Control the number used for all related documents.</p></div><button class="btn" data-action="save-serials">Save serial settings</button></div><form id="serialForm" class="form-grid two"><label>Prefix<input name="serialPrefix" value="${esc(S.settings.serialPrefix)}"></label><label>Code / Project<input name="serialCode" value="${esc(S.settings.serialCode)}"></label><label>Year<input name="serialYear" value="${esc(S.settings.serialYear)}"></label><label>Next Number<input name="nextSerial" type="number" step="1" value="${esc(S.settings.nextSerial)}"></label><label>Padding<input name="serialPadding" type="number" step="1" value="${esc(S.settings.serialPadding)}"></label><label>Lock before date<input name="lockBeforeDate" type="date" value="${esc(S.settings.lockBeforeDate || '')}"></label><label>Revision mode<select name="revisionMode"><option value="true" ${S.settings.revisionMode ? 'selected' : ''}>Enabled</option><option value="false" ${!S.settings.revisionMode ? 'selected' : ''}>Disabled</option></select></label><label>Approval required<select name="approvalRequired"><option value="true" ${S.settings.approvalRequired ? 'selected' : ''}>Yes</option><option value="false" ${!S.settings.approvalRequired ? 'selected' : ''}>No</option></select></label></form><br><div class="serial-preview">${esc(peekBaseSerial())}</div><p class="tiny muted">Example quotation: ${esc(makeDocNumber(peekBaseSerial(), 'quotation', 0))}</p></div><div class="card"><div class="section-title"><div><h3>Recent serial chains</h3><p>Duplicate protection: never reuse old base serials.</p></div></div><div class="table-wrap"><table class="table"><thead><tr><th>Base Serial</th><th>Docs</th><th>Flow</th><th>Customer</th></tr></thead><tbody>${chains || '<tr><td colspan="4" class="empty">No chains yet.</td></tr>'}</tbody></table></div></div></div>`;
}
async function saveSerialSettings() {
  const fd = Object.fromEntries(new FormData($('serialForm')));
  S.settings = { ...S.settings, ...fd, nextSerial: num(fd.nextSerial), serialPadding: num(fd.serialPadding), revisionMode: fd.revisionMode === 'true', approvalRequired: fd.approvalRequired === 'true' };
  addAudit('Update serial settings', 'settings', 'Serial format/settings changed');
  await saveData('Serial settings saved');
}

function renderUsers() {
  if (!API_MODE) return `<div class="card"><div class="empty">Employee self-signup works in server mode. Run the app on your server, then employees can create accounts from the login page.</div></div>`;
  if (!isAdmin()) return `<div class="card"><div class="empty">Only admin can manage employee accounts.</div></div>`;
  return `<div class="card"><div class="section-title"><div><h3>Employees & Access</h3><p>Employees create their own accounts. Admin approves, disables, or changes roles here.</p></div><button class="btn secondary small" onclick="loadUsersIfNeeded(true)">Refresh</button></div><div class="grid-2"><div class="card compact"><h3>Signup Settings</h3><form id="signupForm" class="form-grid two"><label>Self signup<select name="signupsEnabled"><option value="true" ${S.settings.signupsEnabled ? 'selected' : ''}>Enabled</option><option value="false" ${!S.settings.signupsEnabled ? 'selected' : ''}>Disabled</option></select></label><label>Default role<select name="defaultRole"><option value="staff" ${S.settings.defaultRole === 'staff' ? 'selected' : ''}>Staff</option><option value="manager" ${S.settings.defaultRole === 'manager' ? 'selected' : ''}>Manager</option><option value="viewer" ${S.settings.defaultRole === 'viewer' ? 'selected' : ''}>Viewer</option></select></label></form><button class="btn small" data-action="save-signup-settings">Save signup settings</button></div><div class="card compact"><h3>Roles</h3><p class="tiny muted"><strong>Admin:</strong> settings/users/restore. <strong>Manager:</strong> business work. <strong>Staff:</strong> normal work. <strong>Viewer:</strong> read-only.</p></div></div><br><div id="usersTable">Loading users...</div></div>`;
}
async function loadUsersIfNeeded(force = false) {
  if (!API_MODE || !isAdmin()) return;
  if (USERS_CACHE.length && !force) { renderUsersTable(); return; }
  const res = await fetch('/api/users', { credentials: 'include' });
  const data = await safeJSON(res);
  if (!res.ok) { $('usersTable').innerHTML = `<div class="danger-note">${esc(data?.error || 'Could not load users')}</div>`; return; }
  USERS_CACHE = Array.isArray(data) ? data : (data.users || []);
  renderUsersTable();
}
function renderUsersTable() {
  const rows = USERS_CACHE.map(u => `<tr><td><strong>${esc(u.username)}</strong><div class="tiny muted">${esc(u.fullName || '')} · ${esc(u.email || '')}</div></td><td>${statusPill(u.status)}</td><td>${esc(u.role)}</td><td>${esc(formatDateTime(u.createdAt))}</td><td>${esc(formatDateTime(u.lastLoginAt))}</td><td><div class="action-row"><button class="btn good small" data-action="user-action" data-username="${esc(u.username)}" data-user-action="approve">Approve</button><button class="btn warn small" data-action="user-action" data-username="${esc(u.username)}" data-user-action="role" data-role="manager">Manager</button><button class="btn secondary small" data-action="user-action" data-username="${esc(u.username)}" data-user-action="role" data-role="staff">Staff</button><button class="btn secondary small" data-action="user-action" data-username="${esc(u.username)}" data-user-action="role" data-role="viewer">Viewer</button><button class="btn danger small" data-action="user-action" data-username="${esc(u.username)}" data-user-action="disable">Disable</button></div></td></tr>`).join('');
  const el = $('usersTable'); if (el) el.innerHTML = `<div class="table-wrap"><table class="table"><thead><tr><th>User</th><th>Status</th><th>Role</th><th>Created</th><th>Last login</th><th>Actions</th></tr></thead><tbody>${rows || '<tr><td colspan="6" class="empty">No users.</td></tr>'}</tbody></table></div>`;
}
async function userAction(username, action, role = '') {
  const res = await fetch('/api/users', { method: 'POST', credentials: 'include', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ username, action, role }) });
  const data = await safeJSON(res);
  if (!res.ok) throw new Error(data?.error || 'User update failed');
  toast('User updated', 'ok'); USERS_CACHE = []; await loadUsersIfNeeded(true);
}
async function saveSignupSettings() {
  const fd = Object.fromEntries(new FormData($('signupForm')));
  S.settings.signupsEnabled = fd.signupsEnabled === 'true'; S.settings.defaultRole = fd.defaultRole || 'staff';
  addAudit('Update signup settings', 'settings', `Self signup ${S.settings.signupsEnabled ? 'enabled' : 'disabled'}`);
  await saveData('Signup settings saved');
}

function renderSettings() {
  return `<div class="grid-2"><div class="card"><div class="section-title"><div><h3>Backup & Restore</h3><p>Protect your database before server changes.</p></div></div><div class="action-row"><button class="btn" data-action="backup">Download JSON Backup</button><button class="btn secondary" data-action="export-xlsx">Download Excel</button><button class="btn secondary" data-action="export-csv" data-table="documents">Documents CSV</button></div><br><label>Restore backup JSON<input id="restoreFile" class="file-input" type="file" accept="application/json"></label><br><button class="btn warn" data-action="restore-backup">Restore selected backup</button><p class="risk">Restore overwrites current business data. Use only after downloading a backup.</p></div><div class="card"><div class="section-title"><div><h3>Password</h3><p>Change your current login password.</p></div></div>${API_MODE ? `<form id="passwordForm" class="form-grid two"><label>Old password<input name="oldPassword" type="password"></label><label>New password<input name="newPassword" type="password" minlength="6"></label></form><br><button class="btn" data-action="change-password">Change password</button>` : '<p class="muted">Password is available in server/EXE mode.</p>'}</div></div><div class="card"><div class="section-title"><div><h3>Server Deployment Guide</h3><p>Use one central server so all employees work on the same database.</p></div></div><div class="guide-steps"><div><strong>1.</strong> Copy the server package to your Windows or Linux server.</div><div><strong>2.</strong> Run with <code>ZENITH_ERP_ADDR=0.0.0.0:8080</code> and <code>ZENITH_ERP_BROWSER=0</code>.</div><div><strong>3.</strong> Open the server URL from employee computers or phones.</div><div><strong>4.</strong> Employees click “Create employee account”.</div><div><strong>5.</strong> Admin logs in and approves employees under Employees & Access.</div></div></div>`;
}
async function changePassword() {
  const fd = Object.fromEntries(new FormData($('passwordForm')));
  const res = await fetch('/api/change-password', { method: 'POST', credentials: 'include', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(fd) });
  const data = await safeJSON(res); if (!res.ok) throw new Error(data?.error || 'Password change failed'); toast('Password changed', 'ok'); $('passwordForm').reset();
}
async function downloadBackup() { if (API_MODE) { location.href = '/api/backup'; } else { downloadText('zenith-eclipse-erp-backup-' + today() + '.json', JSON.stringify(S, null, 2), 'application/json'); } }
async function exportXLSX() { if (API_MODE) location.href = '/api/export/xlsx'; else toast('Excel export is available in EXE/server mode. Use JSON backup offline.', 'error'); }
async function exportCSV(table) { if (API_MODE) location.href = '/api/export/csv?table=' + encodeURIComponent(table); else downloadText(`zenith-${table}-${today()}.csv`, makeCSV(table), 'text/csv'); }
async function restoreBackup() {
  const file = $('restoreFile')?.files?.[0]; if (!file) throw new Error('Choose a backup JSON file first.');
  if (!confirm('Restore this backup and overwrite current data?')) return;
  const text = await file.text();
  if (API_MODE) { const res = await fetch('/api/restore', { method: 'POST', credentials: 'include', body: text }); const data = await safeJSON(res); if (!res.ok) throw new Error(data?.error || 'Restore failed'); await loadAPIData(); renderShell(); toast('Backup restored', 'ok'); }
  else { S = JSON.parse(text); normalizeState(); localStorage.setItem(STORE_KEY, JSON.stringify(S)); renderShell(); toast('Backup restored', 'ok'); }
}

function openModal(title, body, foot = '') { closeModal(); document.body.insertAdjacentHTML('beforeend', `<div class="modal-backdrop" id="modalBackdrop"><div class="modal"><div class="modal-head"><h3>${esc(title)}</h3><button class="close-x" data-action="close-modal">×</button></div><div class="modal-body">${body}</div><div class="modal-foot">${foot}</div></div></div>`); }
function closeModal() { $('modalBackdrop')?.remove(); }
function toast(text, type = '') { document.querySelectorAll('.toast').forEach(x => x.remove()); const el = document.createElement('div'); el.className = `toast ${type}`; el.textContent = text; document.body.appendChild(el); setTimeout(() => el.remove(), 2600); }

function statusPill(status = '') { const s = String(status || 'Draft'); const cls = s.toLowerCase().replace(/\s+/g, '-'); return `<span class="status ${esc(cls)}">${esc(s)}</span>`; }
function partyName(kind, id) { const list = kind === 'supplier' ? S.suppliers : S.customers; return list.find(p => p.id === id)?.name || ''; }
function docById(id) { return S.documents.find(d => d.id === id); }
function paidForDoc(id) { return S.payments.filter(p => p.documentId === id).reduce((a, p) => a + (p.type === 'received' || p.type === 'paid' ? num(p.amount) : 0), 0); }
function updatePaidStatuses() { for (const d of S.documents.filter(x => ['invoice', 'purchase'].includes(x.type))) { const total = docTotals(d).total; const paid = paidForDoc(d.id); if (total > 0 && paid >= total) d.status = 'Paid'; else if (paid > 0) d.status = 'Partial'; } }
function customerBalance(id) { const invoices = S.documents.filter(d => d.customerId === id && d.type === 'invoice').reduce((a, d) => a + docTotals(d).total, 0); const received = S.payments.filter(p => p.partyType === 'customer' && p.partyId === id && p.type === 'received').reduce((a, p) => a + num(p.amount), 0); return cleanMoney(invoices - received); }
function supplierBalance(id) { const purchases = S.documents.filter(d => d.supplierId === id && d.type === 'purchase').reduce((a, d) => a + docTotals(d).total, 0); const paid = S.payments.filter(p => p.partyType === 'supplier' && p.partyId === id && p.type === 'paid').reduce((a, p) => a + num(p.amount), 0); return cleanMoney(purchases - paid); }
function pad(n, width) { return String(Math.trunc(num(n))).padStart(width || 4, '0'); }
function peekBaseSerial() { return `${S.settings.serialPrefix}-${S.settings.serialCode}-${S.settings.serialYear}-${pad(S.settings.nextSerial, num(S.settings.serialPadding) || 4)}`; }
function allocateBaseSerial() { const base = peekBaseSerial(); S.settings.nextSerial = num(S.settings.nextSerial) + 1; return base; }
function makeDocNumber(base, type, revision = 0) { return `${base}-${DOC_TYPES[type]?.suffix || String(type).toUpperCase()}-R${Math.trunc(num(revision))}`; }
function deriveBaseFromNumber(number = '') { const m = String(number).match(/^(.*?)-(QTN|PI|CI|PL|AGR|PO|DN|RCPT)-R\d+$/); return m ? m[1] : ''; }
function generateVerification(doc) { return simpleHash([doc.number, doc.date, doc.baseSerial, doc.type, doc.customerId, JSON.stringify(doc.items || [])].join('|')).toUpperCase().slice(0, 12); }
function simpleHash(str) { let h1 = 0x811c9dc5, h2 = 0x45d9f3b; for (let i = 0; i < str.length; i++) { h1 ^= str.charCodeAt(i); h1 = Math.imul(h1, 16777619); h2 ^= h1; h2 = Math.imul(h2, 1597334677); } return ((h1 >>> 0).toString(16).padStart(8, '0') + (h2 >>> 0).toString(16).padStart(8, '0')); }
function randomTokenShort() { return Math.random().toString(36).slice(2, 8).toUpperCase() + '-' + Date.now().toString(36).slice(-4).toUpperCase(); }
function formatDateTime(s) { if (!s) return ''; try { return new Date(s).toLocaleString(); } catch { return s; } }
function allocateSerialPreview() { toast('Next serial is ' + peekBaseSerial(), 'ok'); }
function copyApprovalText(id) { const d = docById(id); if (!d) return; const text = `Please approve ${DOC_TYPES[d.type]?.title || 'document'} ${d.number}\nCustomer token: ${d.customerToken}\nVerification: ${d.verificationCode}\nTotal: ${money(docTotals(d).total, d.currency || baseCurrency())}`; navigator.clipboard?.writeText(text); toast('Approval text copied', 'ok'); }
function downloadText(filename, text, type) { const blob = new Blob([text], { type }); const a = document.createElement('a'); a.href = URL.createObjectURL(blob); a.download = filename; a.click(); URL.revokeObjectURL(a.href); }
function makeCSV(table) { const rows = table === 'customers' ? S.customers : table === 'suppliers' ? S.suppliers : table === 'products' ? S.products : table === 'expenses' ? S.expenses : table === 'payments' ? S.payments : table === 'audit' ? S.auditLogs : S.documents; const keys = [...new Set(rows.flatMap(r => Object.keys(r).filter(k => typeof r[k] !== 'object')))] ; return [keys.join(','), ...rows.map(r => keys.map(k => '"' + String(r[k] ?? '').replace(/"/g, '""') + '"').join(','))].join('\n'); }
function printDoc(id) { const d = docById(id); if (!d) throw new Error('Document not found'); if (API_MODE) { window.open('/document/' + encodeURIComponent(id), '_blank'); } else { const w = window.open('', '_blank'); w.document.write(clientPrintHTML(d)); w.document.close(); } }
function clientPrintHTML(d) {
  const party = S.customers.find(c => c.id === d.customerId) || S.suppliers.find(c => c.id === d.supplierId) || {};
  const t = docTotals(d);
  const rows = (d.items || []).map(normalizeLineItem).map((it, i) => `<tr><td>${i+1}</td><td>${lineKindLabel(it.itemKind)}</td><td>${esc(it.description)}</td><td>${esc(it.hsCode || '')}</td><td>${esc(it.unit || '')}</td><td class="right">${money(it.qty)}</td><td class="right">${money(it.unitPrice)}</td><td class="right">${money(num(it.qty)*num(it.unitPrice))}</td><td class="right">${money(it.netWeight)}</td><td class="right">${money(it.grossWeight)}</td><td class="right">${money(it.packages)}</td></tr>`).join('');
  return `<!doctype html><html><head><meta charset="utf-8"><title>${esc(d.number)}</title><style>
:root{font-family:Arial,Helvetica,sans-serif;color:#0f172a}body{margin:0;background:#eef5fb}.no-print{position:fixed;right:24px;bottom:24px;background:#083b75;color:white;border:0;border-radius:12px;padding:12px 16px;font-weight:800;cursor:pointer}.sheet{width:210mm;min-height:297mm;margin:18px auto;background:white;border:10px solid transparent;background-image:linear-gradient(#fff,#fff),linear-gradient(135deg,#5eead4,#0f5ea8);background-origin:border-box;background-clip:padding-box,border-box;border-radius:14px;box-shadow:0 24px 70px rgba(15,23,42,.18);display:flex;flex-direction:column}.inner{padding:17px 19px 12px;min-height:273mm;display:flex;flex-direction:column}.head{display:grid;grid-template-columns:1fr 235px;gap:20px;border-bottom:1px solid #dbeafe;padding-bottom:8px}.brand{display:flex;gap:12px;align-items:flex-start}.brand img{width:60px;height:60px;border-radius:50%;object-fit:cover}.brand h1{font-family:Georgia,serif;margin:0;font-size:22px;letter-spacing:.03em}.slogan{font-size:7px;font-weight:900;letter-spacing:.08em;text-transform:uppercase}.to{display:grid;grid-template-columns:auto 1fr;gap:8px;margin-top:7px}.to b.label{font-family:Georgia,serif;font-size:27px}.to p{margin:1px 0;font-size:12px}.meta{text-align:right;padding-top:11px}.meta h2{font-size:14px;margin:0 0 7px}.title{text-align:center;margin:14px 0}.title h2{font-size:20px;text-transform:uppercase;letter-spacing:.16em;margin:0}.chips{display:flex;gap:8px;justify-content:center;margin-top:6px}.chip{border:1px solid #bfdbfe;border-radius:999px;padding:4px 9px;font-size:10px;font-weight:800;background:#eff6ff}.info{display:grid;grid-template-columns:1fr 1fr;gap:12px}.box{border:1px solid #dbeafe;border-radius:12px;padding:10px}.box h3{font-size:10px;color:#475569;text-transform:uppercase;letter-spacing:.08em;margin:0 0 6px}.box p{font-size:11px;margin:3px 0}.tbl{width:100%;border-collapse:collapse;margin-top:12px}.tbl th{background:#0f5ea8;color:white;text-align:left;font-size:9px;padding:7px}.tbl td{font-size:9.6px;padding:7px;border-bottom:1px solid #e7eef5;vertical-align:top}.right{text-align:right}.totals{margin-left:auto;width:310px;margin-top:12px}.row{display:flex;justify-content:space-between;border-bottom:1px solid #e7eef5;padding:6px 0;font-size:11px}.grand{font-size:15px;font-weight:900;border-bottom:3px solid #0f5ea8}.terms{font-size:11px;line-height:1.45;white-space:pre-wrap;margin-top:12px}.grow{flex:1}.sign{display:flex;justify-content:space-between;margin-top:24px}.sig{width:220px;border-top:1px solid #111827;text-align:center;padding-top:7px;font-size:11px;color:#0f5ea8;font-weight:900}.foot{display:grid;grid-template-columns:50px 1fr 1.5fr 1fr 1.35fr;gap:8px;border-top:1px solid #dbeafe;padding-top:8px;align-items:center;font-size:10.5px}.leaf{width:42px}@media print{body{background:#fff}.sheet{margin:0;border-radius:0;box-shadow:none}.no-print{display:none}@page{size:A4;margin:0}}</style></head><body><button class="no-print" onclick="print()">Print / Save PDF</button><main class="sheet"><div class="inner"><section class="head"><div><div class="brand"><img src="${esc(logoSrc())}"><div><h1>${esc(S.company.name)}</h1><div class="slogan">${esc(S.company.slogan || '')}</div></div></div><div class="to"><b class="label">To:</b><div><p><strong>${esc(party.name || '')}</strong></p><p><strong>${esc(party.contact || '')}</strong></p><p>${esc(party.address || '')}</p></div></div></div><div class="meta"><h2>${esc(DOC_TYPES[d.type]?.title || d.type)}# ${esc(d.number)}</h2><p>Date: ${esc(d.date)}</p><p>Status: ${esc(d.status || 'Draft')}</p></div></section><section class="title"><h2>${esc(DOC_TYPES[d.type]?.title || d.type)}</h2><div class="chips"><span class="chip">${esc(dealModeLabel(d.dealMode || inferDealMode(d.items)))}</span><span class="chip">Base: ${esc(d.baseSerial || '')}</span></div></section><section class="info"><div class="box"><h3>Buyer / Customer</h3><p><strong>${esc(party.name || '')}</strong></p><p>${esc(party.contact || '')}</p><p>${esc(party.city || '')} ${esc(party.country || '')}</p></div><div class="box"><h3>Product & Transportation Details</h3><p>Currency: <strong>${esc(d.currency || baseCurrency())}</strong></p><p>Route: ${esc(d.pol || '')} → ${esc(d.pod || '')}</p><p>Container: ${esc(d.containerNo || '')} Seal: ${esc(d.sealNo || '')}</p><p>BL: ${esc(d.blNo || '')} Vessel: ${esc(d.vessel || '')} ${esc(d.voyage || '')}</p></div></section><table class="tbl"><thead><tr><th>#</th><th>Type</th><th>Description</th><th>HS</th><th>Unit</th><th class="right">Qty</th><th class="right">Price</th><th class="right">Total</th><th class="right">Net</th><th class="right">Gross</th><th class="right">Packages</th></tr></thead><tbody>${rows}</tbody></table><section class="totals"><div class="row"><span>Products</span><strong>${money(t.product,d.currency)}</strong></div><div class="row"><span>Transportation</span><strong>${money(t.transport,d.currency)}</strong></div><div class="row"><span>Services/Charges</span><strong>${money(t.service,d.currency)}</strong></div><div class="row"><span>Subtotal</span><strong>${money(t.subtotal,d.currency)}</strong></div><div class="row"><span>Tax</span><strong>${money(t.tax,d.currency)}</strong></div><div class="row grand"><span>Total</span><strong>${money(t.total,d.currency)}</strong></div></section><section class="terms"><strong>Notes</strong><br>${esc(d.notes || '')}</section><section class="terms"><strong>Terms & Conditions</strong><br>${esc(d.terms || '')}</section><div class="grow"></div><div class="sign"><div class="terms"><strong>Verification:</strong> ${esc(d.verificationCode || generateVerification(d))}</div><div class="sig">${esc(S.company.stampText || 'Authorized Signature')}</div></div><footer class="foot"><img class="leaf" src="${esc(leafSrc())}"><div>✉ ${esc(S.company.email)}</div><div>📍 ${esc(S.company.address)}</div><div>☎ ${esc(S.company.phone)}</div><div><strong>Find More at</strong><br>${esc(S.company.website)}</div></footer></div></main></body></html>`;
}
async function logout() { await fetch('/api/logout', { method: 'POST', credentials: 'include' }).catch(() => {}); USER = null; S = null; showLogin('Logged out'); }
