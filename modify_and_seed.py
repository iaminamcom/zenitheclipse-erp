import base64, hashlib, json, os, re, datetime, shutil, textwrap
from pathlib import Path

BASE = Path('/mnt/data/zenith_erp_custom')
MAIN = BASE/'main.go'
BACKUP = Path('/mnt/data/zenith-eclipse-erp-backup-2026-06-13-161412.json')
LOGO = Path('/mnt/data/pdf_img_0_0.png')
LEAF = Path('/mnt/data/pdf_img_0_1.png')

def data_url(path):
    if not path.exists():
        return ''
    return 'data:image/png;base64,' + base64.b64encode(path.read_bytes()).decode('ascii')

def hpw(username, password):
    s = (username.lower() + ':zenith-eclipse-local-mvp:' + password).encode()
    return hashlib.sha256(s).hexdigest()

def s(v):
    if v is None:
        return ''
    if isinstance(v, bool):
        return 'true' if v else 'false'
    if isinstance(v, (int,float)):
        # keep clean integer-looking values
        if isinstance(v,float) and v.is_integer():
            return str(int(v))
        return str(v)
    return str(v)

def make_record(module, number, job_ref='', status='Draft', fields=None, links=None, id_=None, created_at=None, created_by='admin'):
    now = datetime.datetime.now(datetime.timezone.utc).isoformat()
    return {
        'id': id_ or hashlib.sha1((module+number+now).encode()).hexdigest()[:24],
        'module': module,
        'number': number,
        'jobRef': job_ref or '',
        'status': status or 'Draft',
        'createdAt': created_at or now,
        'createdBy': created_by or 'admin',
        'updatedAt': created_at or now,
        'updatedBy': created_by or 'admin',
        'version': 1,
        'fields': {k:s(v) for k,v in (fields or {}).items() if v is not None and s(v)!=''},
        'links': links or {},
        'history': []
    }

def pick(d, *keys):
    for k in keys:
        if isinstance(d, dict) and k in d and d[k] not in (None, ''):
            return d[k]
    return ''

def totals(items, shipping=0, discount=0, tax_rate=0):
    products=transport=services=0.0
    for it in items or []:
        qty=float(it.get('qty') or it.get('quantity') or 0)
        price=float(it.get('unitPrice') or it.get('price') or it.get('amount') or 0)
        total=float(it.get('total') or (qty*price))
        kind=str(it.get('itemKind') or it.get('category') or it.get('type') or 'product').lower()
        if 'transport' in kind or 'freight' in kind:
            transport+=total
        elif 'service' in kind or 'charge' in kind:
            services+=total
        else:
            products+=total
    subtotal=products+transport+services+float(shipping or 0)
    discount=float(discount or 0)
    tax=(subtotal-discount)*float(tax_rate or 0)/100.0
    total=subtotal-discount+tax
    return products, transport, services, subtotal, tax, total

old = json.loads(BACKUP.read_text())
company_old = old.get('company', {})
logo_data = data_url(LOGO)
leaf_data = data_url(LEAF)
company = {
    'name': pick(company_old,'name') or 'ZENITH ECLIPSE CO',
    'legalName': pick(company_old,'legalName') or pick(company_old,'name') or 'ZENITH ECLIPSE CO',
    'address': pick(company_old,'address'),
    'city': pick(company_old,'city'),
    'country': pick(company_old,'country'),
    'phone': pick(company_old,'phone'),
    'whatsApp': pick(company_old,'whatsApp'),
    'email': pick(company_old,'email'),
    'website': pick(company_old,'website') or 'http://www.zenitheclipse.com',
    'taxNumber': pick(company_old,'taxId','trn'),
    'logoText': pick(company_old,'name') or 'ZENITH ECLIPSE CO',
    'stampText': pick(company_old,'stampText') or 'Authorized Signature',
    'baseCurrency': pick(company_old,'baseCurrency') or 'USD',
    'slogan': pick(company_old,'slogan'),
    'bankName': pick(company_old,'bankName'),
    'bankAccount': pick(company_old,'bankAccount'),
    'bankIban': pick(company_old,'bankIban'),
    'bankSwift': pick(company_old,'bankSwift'),
    'defaultTerms': pick(company_old,'defaultTerms'),
    'defaultNotes': pick(company_old,'defaultNotes'),
    'currencyList': pick(company_old,'currencyList'),
    'prefix': pick(company_old,'prefix') or 'ZE',
    'logoData': logo_data,
    'leafData': leaf_data,
    'stampData': pick(company_old,'stampData'),
}
now = datetime.datetime.now(datetime.timezone.utc).isoformat()
records=[]
serial={}

def inc(mod):
    serial[mod]=serial.get(mod,0)+1

def add(rec):
    records.append(rec); inc(rec['module']); return rec

# Users
users=[{
    'id':'admin', 'username':'admin','displayName':'Administrator','role':'Owner/Admin','department':'Management',
    'passwordHash':hpw('admin','ChangeMe123!'), 'active':True, 'createdAt': pick(old.get('users',[{}])[0] if old.get('users') else {}, 'createdAt') or now
}]
# Customers
cust_lookup={}
for c in old.get('customers',[]):
    fields={
        'name':pick(c,'name'), 'contactPerson':pick(c,'contact'), 'email':pick(c,'email'), 'mobile':pick(c,'phone','mobile'),
        'country':pick(c,'country'), 'city':pick(c,'city'), 'address':pick(c,'address'), 'code':pick(c,'code'),
        'creditLimit':pick(c,'creditLimit','balanceLimit'), 'outstandingBalance':pick(c,'outstandingBalance','balance'), 'currency':pick(c,'currency'),
        'kycStatus':pick(c,'kycStatus') or 'Imported', 'riskRating':pick(c,'riskRating') or '', 'notes':pick(c,'notes')
    }
    cust_lookup[c.get('id','')]=fields.get('name') or fields.get('code') or c.get('id','')
    add(make_record('customer', fields.get('code') or ('CUS-2026-%04d'%(serial.get('customer',0)+1)), '', pick(c,'status') or 'Approved', fields, id_=pick(c,'id'), created_at=pick(c,'createdAt')))
# Suppliers
sup_lookup={}
for u in old.get('suppliers',[]):
    fields={
        'name':pick(u,'name'), 'contactPerson':pick(u,'contact'), 'email':pick(u,'email'), 'mobile':pick(u,'phone','mobile'),
        'country':pick(u,'country'), 'city':pick(u,'city'), 'address':pick(u,'address'), 'code':pick(u,'code'),
        'outstandingBalance':pick(u,'outstandingBalance','balance'), 'currency':pick(u,'currency'), 'kybStatus':'Imported', 'riskRating':pick(u,'riskRating'), 'notes':pick(u,'notes')
    }
    sup_lookup[u.get('id','')]=fields.get('name') or fields.get('code') or u.get('id','')
    add(make_record('supplier', fields.get('code') or ('SUP-2026-%04d'%(serial.get('supplier',0)+1)), '', pick(u,'status') or 'Approved', fields, id_=pick(u,'id'), created_at=pick(u,'createdAt')))
# Products / services
for p in old.get('products',[]):
    fields={
        'name':pick(p,'name'), 'sku':pick(p,'sku','code'), 'category':pick(p,'category'), 'description':pick(p,'description'),
        'hsCode':pick(p,'hsCode'), 'unit':pick(p,'unit'), 'costPrice':pick(p,'costPrice'), 'salePrice':pick(p,'salePrice','unitPrice'),
        'currency':pick(p,'currency'), 'supplier': sup_lookup.get(p.get('supplierId',''), pick(p,'supplier')), 'minStock':pick(p,'minStock'), 'notes':pick(p,'notes')
    }
    add(make_record('product', pick(p,'code','sku') or ('PRD-2026-%04d'%(serial.get('product',0)+1)), '', pick(p,'status') or 'Active', fields, id_=pick(p,'id'), created_at=pick(p,'createdAt')))
# Cases
for case in old.get('cases',[]):
    fields={
        'title':pick(case,'title'), 'customer':cust_lookup.get(case.get('customerId',''),''), 'supplier':sup_lookup.get(case.get('supplierId',''),''),
        'priority':pick(case,'priority'), 'owner':pick(case,'owner'), 'notes':pick(case,'notes'), 'baseSerial':pick(case,'baseSerial','baseNumber')
    }
    add(make_record('business_case', pick(case,'baseSerial','baseNumber') or ('CASE-2026-%04d'%(serial.get('business_case',0)+1)), pick(case,'baseSerial','baseNumber'), pick(case,'status') or 'Draft', fields, id_=pick(case,'id'), created_at=pick(case,'createdAt')))
# Accounts
for a in old.get('accounts',[]):
    fields={'bankName':pick(a,'bankName') or pick(a,'name'), 'accountName':pick(a,'accountName') or pick(a,'name'), 'accountNumber':pick(a,'accountNumber'), 'iban':pick(a,'iban'), 'currency':pick(a,'currency'), 'openingBalance':pick(a,'balance'), 'currentBalance':pick(a,'balance'), 'type':pick(a,'type'), 'notes':pick(a,'notes')}
    add(make_record('bank_account', pick(a,'name') or ('BANK-2026-%04d'%(serial.get('bank_account',0)+1)), '', 'Active', fields, id_=pick(a,'id')))

# Documents
module_map={'quotation':'quotation','qtn':'quotation','pi':'proforma_invoice','proforma':'proforma_invoice','proforma_invoice':'proforma_invoice','commercial_invoice':'commercial_invoice','ci':'commercial_invoice','invoice':'sales_invoice','sales_invoice':'sales_invoice'}
first_doc_record=None
for d in old.get('documents',[]):
    typ=str(pick(d,'type') or '').lower()
    module=module_map.get(typ, typ or 'document_upload')
    if module not in {'quotation','proforma_invoice','commercial_invoice','sales_invoice','packing_list','bill_of_lading','delivery_note'}:
        module='document_upload'
    items=[]
    for it in d.get('items') or []:
        qty=float(it.get('qty') or it.get('quantity') or 0)
        price=float(it.get('unitPrice') or it.get('price') or 0)
        total=float(it.get('total') or qty*price)
        items.append({
            'type': pick(it,'itemKind','category','type') or 'product', 'description': pick(it,'description','name'), 'hsCode': pick(it,'hsCode'),
            'unit': pick(it,'unit'), 'qty': qty, 'unitPrice': price, 'total': total, 'netWeight': it.get('netWeight',0),
            'grossWeight': it.get('grossWeight',0), 'packages': it.get('packages',0)
        })
    products, transport, services, subtotal, tax, total = totals(items, pick(d,'shipping') or 0, pick(d,'discount') or 0, pick(d,'taxRate') or 0)
    route = ''
    pol, pod = pick(d,'pol'), pick(d,'pod')
    if pol or pod: route = (pol or '') + (' → ' if pol and pod else '') + (pod or '')
    fields={
        'customer': cust_lookup.get(d.get('customerId',''), pick(d,'customerName','customer')), 'supplier': sup_lookup.get(d.get('supplierId',''), pick(d,'supplierName','supplier')),
        'date': pick(d,'date'), 'dueDate':pick(d,'dueDate'), 'validUntil':pick(d,'validUntil'), 'currency':pick(d,'currency') or company['baseCurrency'],
        'incoterm':pick(d,'incoterm'), 'route':route, 'pol':pol, 'pod':pod, 'fpod':pick(d,'fpod'), 'blNo':pick(d,'blNo'),
        'containerNumber':pick(d,'containerNo','containerNumber'), 'sealNumber':pick(d,'sealNo','sealNumber'), 'truckDriver':pick(d,'truckDriver'),
        'vesselVoyage':(' '.join([s(pick(d,'vessel')),s(pick(d,'voyage'))])).strip(), 'itemsJSON':json.dumps(items,ensure_ascii=False),
        'productsTotal':products, 'transportTotal':transport, 'servicesTotal':services, 'shipping':pick(d,'shipping') or 0,
        'subtotal':subtotal, 'discount':pick(d,'discount') or 0, 'taxRate':pick(d,'taxRate') or 0, 'tax':tax, 'amount':total, 'total':total,
        'notes':pick(d,'notes'), 'terms':pick(d,'terms') or company['defaultTerms'], 'verificationCode':pick(d,'verificationCode'), 'baseSerial':pick(d,'baseSerial','baseNumber'),
        'dealMode':pick(d,'dealMode'), 'revision':pick(d,'revision'), 'customerToken':pick(d,'customerToken')
    }
    status=pick(d,'approvalStatus') or pick(d,'status') or 'Draft'
    # Normalize draft lowercase to Draft
    status = 'Draft' if str(status).lower()=='draft' else s(status)
    rec=add(make_record(module, pick(d,'number') or ('DOC-2026-%04d'%(serial.get(module,0)+1)), pick(d,'chainId','baseSerial','baseNumber'), status, fields, id_=pick(d,'id'), created_at=pick(d,'createdAt'), created_by=pick(d,'createdBy') or 'admin'))
    if first_doc_record is None and module in ('quotation','proforma_invoice'):
        first_doc_record=rec

# Add a sales invoice based on the first quotation so the requested second invoice is present.
source = first_doc_record or (records[-1] if records else None)
if source:
    fields=dict(source['fields'])
    fields['sourceDocument']=source['number']
    fields['invoiceDate']=datetime.date.today().isoformat()
    fields['dueDate']=datetime.date.today().isoformat()
    fields['paymentTerms']=fields.get('terms') or company['defaultTerms']
    fields['remarks']='Sales invoice template added to software from the requested quotation/proforma design.'
    job=source.get('jobRef') or 'ZE-HRBTC-2026-0001'
    add(make_record('sales_invoice','ZE-HRBTC-2026-0001-SINV-R0',job,'Pending Approval',fields,links={source['module']:source['id']},id_='seed-sales-invoice-0001'))

# Add a quotation letter for the design proposal.
design_items=[{'type':'service','description':'ERP document design proposal: company letterhead, quotation, proforma invoice and sales invoice templates inside Zenith Eclipse ERP','unit':'Project','qty':1,'unitPrice':0,'total':0,'netWeight':0,'grossWeight':0,'packages':1}]
fields={
    'customer':'ZENITH ECLIPSE CO','date':datetime.date.today().isoformat(),'validUntil':(datetime.date.today()+datetime.timedelta(days=15)).isoformat(),'currency':company['baseCurrency'],
    'productDescription':'Quotation letter for ERP document design proposal','quantity':'1','amount':'0','itemsJSON':json.dumps(design_items,ensure_ascii=False),
    'servicesTotal':'0','subtotal':'0','total':'0','notes':'Design proposal quotation template created inside the software. Edit prices and scope as needed.',
    'terms':company['defaultTerms'],'verificationCode':'DESIGN-PROPOSAL','dealMode':'Design Proposal'
}
add(make_record('quotation','ZE-DESIGN-2026-0001-QTN-R0','ZE-DESIGN-2026-0001','Draft',fields,id_='seed-design-quotation'))

# Add official letterhead record.
letter_fields={
    'title':'Official Company Letterhead','subject':'Zenith Eclipse ERP Document Design','date':datetime.date.today().isoformat(),
    'body':'This letterhead template is part of the Zenith Eclipse ERP document engine. Use it for official company letters, design proposals, authorization letters and general correspondence.\n\nPrepared for: Zenith Eclipse Co\nPurpose: Professional letterhead, proforma invoice, sales invoice and quotation design inside the ERP software.',
    'remarks':'Use Print / Save PDF from the browser to export official letters.'
}
add(make_record('letterhead','ZE-LHD-2026-0001','', 'Draft', letter_fields, id_='seed-letterhead-0001'))

# Audit logs
for a in old.get('auditLogs',[])[:200]:
    pass
settings={
    'firstRun':'true',
    'passwordPolicy':'Minimum 8 characters recommended for real use',
    'storage':'Local JSON database with imported Zenith Eclipse backup seed',
    'seededFrom':'Uploaded old backup JSON and uploaded quotation PDF design',
    'legacyVersion':old.get('version',''),
    'documentDesign':'Quotation, letterhead, proforma invoice and sales invoice print templates are enabled'
}
state={
    'company':company,
    'users':users,
    'records':records,
    'serial':serial,
    'audit':[{'id':'seed-audit-0001','time':now,'user':'system','action':'IMPORT_BACKUP','module':'system','recordId':'','number':'','details':'Imported uploaded old backup and added letterhead, proforma invoice, sales invoice and quotation design proposal templates'}],
    'settings':settings
}
(BASE/'default_data.json').write_text(json.dumps(state,ensure_ascii=False,indent=2))
print('Wrote default_data.json with',len(records),'records, modules:', {m:sum(1 for r in records if r['module']==m) for m in sorted(set(r['module'] for r in records))})

# Modify Go source
src=MAIN.read_text()
# import embed
if '"crypto/sha256"\n' in src and '_ "embed"' not in src:
    src=src.replace('"crypto/sha256"\n', '"crypto/sha256"\n\t_ "embed"\n')
# consts
src=src.replace('const appName = "Zenith Eclipse ERP A-to-Z MVP"\nconst appVersion = "1.0.0-mvp"', 'const appName = "Zenith Eclipse ERP A-to-Z Custom"\nconst appVersion = "1.1.0-letterhead-invoices"')
# Company struct
src=re.sub(r'type Company struct \{.*?\n\}', '''type Company struct {
\tName         string `json:"name"`
\tLegalName    string `json:"legalName"`
\tAddress      string `json:"address"`
\tCity         string `json:"city"`
\tCountry      string `json:"country"`
\tPhone        string `json:"phone"`
\tWhatsApp     string `json:"whatsApp"`
\tEmail        string `json:"email"`
\tWebsite      string `json:"website"`
\tTaxNumber    string `json:"taxNumber"`
\tLogoText     string `json:"logoText"`
\tStampText    string `json:"stampText"`
\tBaseCurrency string `json:"baseCurrency"`
\tSlogan       string `json:"slogan"`
\tBankName     string `json:"bankName"`
\tBankAccount  string `json:"bankAccount"`
\tBankIban     string `json:"bankIban"`
\tBankSwift    string `json:"bankSwift"`
\tDefaultTerms string `json:"defaultTerms"`
\tDefaultNotes string `json:"defaultNotes"`
\tCurrencyList string `json:"currencyList"`
\tPrefix       string `json:"prefix"`
\tLogoData     string `json:"logoData"`
\tLeafData     string `json:"leafData"`
\tStampData    string `json:"stampData"`
}''', src, count=1, flags=re.S)
# embed variable after App struct
if 'var defaultDataJSON []byte' not in src:
    src=src.replace('type App struct {\n\tmu        sync.Mutex\n\tstate     State\n\tsessions  map[string]string\n\tdataDir   string\n\tdataPath  string\n\tuploadDir string\n}\n', 'type App struct {\n\tmu        sync.Mutex\n\tstate     State\n\tsessions  map[string]string\n\tdataDir   string\n\tdataPath  string\n\tuploadDir string\n}\n\n//go:embed default_data.json\nvar defaultDataJSON []byte\n')
# prefixes
new_prefixes='''var prefixes = map[string]string{
\t"customer": "CUS", "supplier": "SUP", "product": "PRD", "lead": "LEAD",
\t"rfq": "RFQ", "quotation": "QUO", "proforma_invoice": "PI", "sales_invoice": "SI", "sales_order": "SO",
\t"purchase_order": "PO", "commercial_invoice": "CI", "packing_list": "PL", "shipment": "SHP",
\t"bill_of_lading": "BL", "delivery_note": "DN", "receipt_voucher": "RV", "payment_voucher": "PV",
\t"expense": "EXP", "contract": "CTR", "employee": "EMP", "driver": "DRV", "truck": "TRK",
\t"task": "TSK", "compliance": "COM", "bank_account": "BANK", "approval": "APR", "document_upload": "DOC",
\t"letterhead": "LHD", "business_case": "CASE",
}

func main()'''
src=re.sub(r'var prefixes = map\[string\]string\{.*?\n\}\n\nfunc main\(\)', new_prefixes, src, count=1, flags=re.S)
# route letterhead
if 'handleLetterhead' not in src.split('func main()',1)[1].split('func (app *App) init',1)[0]:
    src=src.replace('mux.HandleFunc("/doc/", app.requireAuth(app.handleDocument))\n\tmux.HandleFunc("/verify/", app.handleVerify)', 'mux.HandleFunc("/doc/", app.requireAuth(app.handleDocument))\n\tmux.HandleFunc("/letterhead", app.requireAuth(app.handleLetterhead))\n\tmux.HandleFunc("/verify/", app.handleVerify)')
# data dir
src=src.replace('ZenithEclipseERP_AtoZ_MVP', 'ZenithEclipseERP_AtoZ_Custom_Letterhead_Invoices')
src=src.replace('.zenith_eclipse_erp_atoz_mvp', '.zenith_eclipse_erp_atoz_custom_letterhead_invoices')
# defaultState embed insertion
if 'if len(defaultDataJSON) > 0 {' not in src:
    src=src.replace('func defaultState() State {\n\tnow := time.Now().Format(time.RFC3339)', 'func defaultState() State {\n\tif len(defaultDataJSON) > 0 {\n\t\tvar s State\n\t\tif err := json.Unmarshal(defaultDataJSON, &s); err == nil {\n\t\t\tif s.Settings == nil {\n\t\t\t\ts.Settings = map[string]string{}\n\t\t\t}\n\t\t\treturn s\n\t\t}\n\t}\n\tnow := time.Now().Format(time.RFC3339)')
# business flow and status
src=src.replace('case "rfq", "quotation", "proforma_invoice", "sales_order", "purchase_order", "commercial_invoice", "packing_list", "shipment", "bill_of_lading", "delivery_note":', 'case "rfq", "quotation", "proforma_invoice", "sales_invoice", "sales_order", "purchase_order", "commercial_invoice", "packing_list", "shipment", "bill_of_lading", "delivery_note":')
src=src.replace('case "customer", "supplier", "contract", "quotation", "commercial_invoice", "payment_voucher", "receipt_voucher", "bill_of_lading", "delivery_note":', 'case "customer", "supplier", "contract", "quotation", "proforma_invoice", "sales_invoice", "commercial_invoice", "payment_voucher", "receipt_voucher", "bill_of_lading", "delivery_note":')
# afterRecordChange sales invoice
src=src.replace('existing.Module == "commercial_invoice" && existing.JobRef == rec.JobRef', 'existing.Module == "sales_invoice" && existing.JobRef == rec.JobRef')
src=src.replace('invoice := app.createRecordLocked(user, "commercial_invoice", fields, "Pending Approval", rec.JobRef, links)', 'invoice := app.createRecordLocked(user, "sales_invoice", fields, "Pending Approval", rec.JobRef, links)')
src=src.replace('app.addAuditLocked(user, "AUTO_INVOICE_TRIGGER", "commercial_invoice", invoice.ID, invoice.Number, "Delivery note marked Delivered; invoice draft created")', 'app.addAuditLocked(user, "AUTO_INVOICE_TRIGGER", "sales_invoice", invoice.ID, invoice.Number, "Delivery note marked Delivered; sales invoice draft created")')
src=src.replace('Delivery Note marked Delivered auto-creates a Commercial Invoice draft.', 'Delivery Note marked Delivered auto-creates a Sales Invoice draft.')
# copy common fields
src=src.replace('common := []string{"customer", "supplier", "amount", "cost", "saleAmount", "estimatedCost", "currency", "productDescription", "quantity", "weight", "route", "loadingLocation", "deliveryLocation", "containerNumber", "sealNumber", "truckNumber", "driverName", "driverMobile", "vesselVoyage", "pol", "pod", "fpod"}', 'common := []string{"customer", "supplier", "amount", "total", "subtotal", "discount", "tax", "taxRate", "cost", "saleAmount", "estimatedCost", "currency", "productDescription", "quantity", "weight", "route", "loadingLocation", "deliveryLocation", "containerNumber", "sealNumber", "truckNumber", "driverName", "driverMobile", "vesselVoyage", "pol", "pod", "fpod", "incoterm", "itemsJSON", "notes", "terms", "paymentTerms", "date", "invoiceDate", "dueDate", "validUntil", "verificationCode"}')
# handleRestore replacement
restore_block='''func (app *App) handleRestore(w http.ResponseWriter, r *http.Request) {
\tif r.Method != http.MethodPost {
\t\terrorJSON(w, 405, "POST required")
\t\treturn
\t}
\tuser := app.sessionUser(r)
\tb, err := io.ReadAll(io.LimitReader(r.Body, 100<<20))
\tif err != nil {
\t\terrorJSON(w, 400, "invalid backup json")
\t\treturn
\t}
\tvar restored State
\tif err := json.Unmarshal(b, &restored); err != nil || len(restored.Users) == 0 || restored.Records == nil {
\t\tif looksLikeLegacyBackup(b) {
\t\t\trestored = defaultState()
\t\t\tif restored.Settings == nil {
\t\t\t\trestored.Settings = map[string]string{}
\t\t\t}
\t\t\trestored.Settings["restoredFromLegacyBackup"] = time.Now().Format(time.RFC3339)
\t\t} else {
\t\t\terrorJSON(w, 400, "invalid backup json")
\t\t\treturn
\t\t}
\t}
\tapp.mu.Lock()
\tdefer app.mu.Unlock()
\told := app.state
\tapp.state = restored
\tapp.ensureStateDefaultsLocked()
\tapp.addAuditLocked(user, "RESTORE_BACKUP", "system", "", "", "Database restored from uploaded backup JSON")
\tif err := app.saveLocked(); err != nil {
\t\tapp.state = old
\t\terrorJSON(w, 500, err.Error())
\t\treturn
\t}
\twriteJSON(w, 200, map[string]any{"ok": true})
}

func looksLikeLegacyBackup(b []byte) bool {
\tvar raw map[string]json.RawMessage
\tif err := json.Unmarshal(b, &raw); err != nil {
\t\treturn false
\t}
\t_, hasCompany := raw["company"]
\t_, hasCustomers := raw["customers"]
\t_, hasDocuments := raw["documents"]
\treturn hasCompany && (hasCustomers || hasDocuments)
}

func (app *App) handleUpload'''
src=re.sub(r'func \(app \*App\) handleRestore\(w http.ResponseWriter, r \*http.Request\) \{.*?\n\}\n\nfunc \(app \*App\) handleUpload', restore_block, src, count=1, flags=re.S)
# handleLetterhead function insert before handleVerify
if 'func (app *App) handleLetterhead' not in src:
    insert='''func (app *App) handleLetterhead(w http.ResponseWriter, r *http.Request) {
\tapp.mu.Lock()
\tcompany := app.state.Company
\tvar rec Record
\tfor _, x := range app.state.Records {
\t\tif x.Module == "letterhead" {
\t\t\trec = x
\t\t\tbreak
\t\t}
\t}
\tapp.mu.Unlock()
\tif rec.ID == "" {
\t\tnow := time.Now().Format(time.RFC3339)
\t\trec = Record{ID: "letterhead", Module: "letterhead", Number: "ZE-LHD-" + time.Now().Format("2006") + "-0001", Status: "Draft", CreatedAt: now, CreatedBy: "system", UpdatedAt: now, UpdatedBy: "system", Version: 1, Fields: map[string]string{"title": "Official Company Letterhead", "body": "Use this page for official company letters."}, Links: map[string]string{}, History: []Change{}}
\t}
\thtml := renderDocHTML(company, rec, nil)
\tw.Header().Set("Content-Type", "text/html; charset=utf-8")
\t_, _ = w.Write([]byte(html))
}

'''
    src=src.replace('func (app *App) handleVerify', insert+'func (app *App) handleVerify')
# renderDocHTML replacement
render_new = r'''func renderDocHTML(c Company, rec Record, all []Record) string {
	if rec.Module == "letterhead" {
		return renderLetterheadHTML(c, rec)
	}
	return renderBusinessDocHTML(c, rec, all)
}

type docLine struct {
	Kind        string
	Description string
	HSCode      string
	Unit        string
	Qty         float64
	UnitPrice   float64
	Total       float64
	NetWeight   float64
	GrossWeight float64
	Packages    float64
}

func renderBusinessDocHTML(c Company, rec Record, all []Record) string {
	esc := template.HTMLEscapeString
	title := docTitle(rec.Module)
	currency := firstNonEmpty(rec.Fields["currency"], c.BaseCurrency, "USD")
	verification := firstNonEmpty(rec.Fields["verificationCode"], rec.Fields["verification"], strings.ToUpper(rec.ID[:min(12, len(rec.ID))]))
	date := firstNonEmpty(rec.Fields["invoiceDate"], rec.Fields["date"], rec.CreatedAt)
	buyer := firstNonEmpty(rec.Fields["customer"], rec.Fields["buyer"], rec.Fields["customerName"], "Customer / Buyer not set")
	contact := firstNonEmpty(rec.Fields["contactPerson"], rec.Fields["contact"], rec.Fields["receiverName"])
	buyerAddress := firstNonEmpty(rec.Fields["customerAddress"], rec.Fields["address"], rec.Fields["deliveryLocation"])
	route := firstNonEmpty(rec.Fields["route"], strings.TrimSpace(rec.Fields["pol"]+" → "+rec.Fields["pod"]))
	lines := lineItemsFromRecord(rec)
	products, transport, services, subtotal, discount, tax, total := totalsByKind(rec, lines)
	linked := linkedDocsHTML(rec, all)

	var details strings.Builder
	for _, row := range [][2]string{{"Currency", currency}, {"Incoterm", rec.Fields["incoterm"]}, {"Route", route}, {"Container", rec.Fields["containerNumber"]}, {"Seal", rec.Fields["sealNumber"]}, {"B/L No.", rec.Fields["blNo"]}, {"Due Date", rec.Fields["dueDate"]}} {
		if strings.TrimSpace(row[1]) != "" && strings.TrimSpace(row[1]) != "→" {
			details.WriteString("<div><b>" + esc(row[0]) + ":</b> " + esc(row[1]) + "</div>")
		}
	}
	if details.Len() == 0 {
		details.WriteString("<div><b>Document details:</b> update fields in ERP record.</div>")
	}

	bankBlock := ""
	if rec.Module == "proforma_invoice" || rec.Module == "sales_invoice" || rec.Module == "commercial_invoice" || rec.Module == "receipt_voucher" || rec.Module == "payment_voucher" {
		bankBlock = `<div class="bank"><b>Bank Details</b><br>` + esc(firstNonEmpty(c.BankName, "Bank name not set")) + `<br>Account: ` + esc(c.BankAccount) + `<br>IBAN: ` + esc(c.BankIban) + `<br>SWIFT: ` + esc(c.BankSwift) + `</div>`
	}
	stamp := esc(c.StampText)
	if strings.TrimSpace(c.StampData) != "" {
		stamp = `<img class="stamp" src="` + esc(c.StampData) + `" alt="stamp">`
	}

	return `<!doctype html><html><head><meta charset="utf-8"><title>` + esc(title+" "+rec.Number) + `</title>` + printCSS() + `</head><body><div class="actions"><button onclick="window.print()">Print / Save PDF</button></div><main class="doc-page"><section class="doc-shell"><header class="doc-header"><div class="brand-block">` + companyLogoHTML(c) + `<div><h1>` + esc(firstNonEmpty(c.Name, c.LogoText, "ZENITH ECLIPSE CO")) + `</h1><p>` + esc(c.Slogan) + `</p></div></div><div class="doc-meta"><b>` + esc(title) + `# ` + esc(rec.Number) + `</b><br>Date: ` + esc(formatDocDate(date)) + `<br>Status: ` + esc(rec.Status) + `<br>Job Ref: ` + esc(rec.JobRef) + `</div></header><div class="to-line"><b>To:</b> ` + esc(buyer) + `<br><span>` + esc(contact) + `</span><br><span>` + esc(buyerAddress) + `</span></div><h2>` + esc(title) + `</h2><div class="pills"><span>` + esc(firstNonEmpty(rec.Fields["dealMode"], moduleLabel(rec.Module))) + `</span><span>Verification ` + esc(verification) + `</span></div><section class="info-grid"><div class="info-card"><b>Buyer / Customer</b><p>` + esc(buyer) + `<br>` + esc(contact) + `<br>` + esc(buyerAddress) + `</p></div><div class="info-card"><b>Product & Transportation Details</b><p>` + details.String() + `</p></div></section>` + renderItemsTable(lines) + `<section class="lower"><div class="notes"><b>Notes</b><br>` + strings.ReplaceAll(esc(firstNonEmpty(rec.Fields["notes"], c.DefaultNotes)), "\n", "<br>") + `<br><br><b>Terms & Conditions</b><br>` + strings.ReplaceAll(esc(firstNonEmpty(rec.Fields["terms"], rec.Fields["paymentTerms"], c.DefaultTerms)), "\n", "<br>") + bankBlock + `</div>` + renderTotalsTable(currency, products, transport, services, subtotal, discount, tax, total) + `</section><section class="verify-row"><div class="qrbox"><pre>` + esc(fakeQR(firstNonEmpty(verification, rec.ID))) + `</pre><b>Verification</b><br>` + esc(verification) + `<br>Base serial: ` + esc(firstNonEmpty(rec.Fields["baseSerial"], rec.JobRef)) + `</div><div class="signature"><div class="sigline"></div><b>Authorized Signature</b><br>` + stamp + `</div></section>` + linked + `<footer>` + companyLeafHTML(c) + `<span>✉ ` + esc(c.Email) + `</span><span>📍 ` + esc(c.Address) + `</span><span>☎ ` + esc(c.Phone) + `</span><span>Find More at<br>` + esc(c.Website) + `</span></footer></section></main></body></html>`
}

func renderLetterheadHTML(c Company, rec Record) string {
	esc := template.HTMLEscapeString
	body := firstNonEmpty(rec.Fields["body"], rec.Fields["content"], "Write your official letter content here.")
	return `<!doctype html><html><head><meta charset="utf-8"><title>` + esc(firstNonEmpty(rec.Fields["title"], "Letterhead")) + `</title>` + printCSS() + `</head><body><div class="actions"><button onclick="window.print()">Print / Save PDF</button></div><main class="doc-page"><section class="doc-shell letter"><header class="doc-header"><div class="brand-block">` + companyLogoHTML(c) + `<div><h1>` + esc(firstNonEmpty(c.Name, "ZENITH ECLIPSE CO")) + `</h1><p>` + esc(c.Slogan) + `</p></div></div><div class="doc-meta"><b>LETTERHEAD</b><br>No: ` + esc(rec.Number) + `<br>Date: ` + esc(formatDocDate(firstNonEmpty(rec.Fields["date"], rec.CreatedAt))) + `</div></header><div class="letter-subject"><b>Subject:</b> ` + esc(firstNonEmpty(rec.Fields["subject"], rec.Fields["title"], "Official Letter")) + `</div><article class="letter-body">` + strings.ReplaceAll(esc(body), "\n", "<br>") + `</article><section class="verify-row"><div class="qrbox"><pre>` + esc(fakeQR(rec.ID)) + `</pre><b>Verification</b><br>` + esc(rec.ID[:min(12, len(rec.ID))]) + `</div><div class="signature"><div class="sigline"></div><b>Authorized Signature</b><br>` + esc(c.StampText) + `</div></section><footer>` + companyLeafHTML(c) + `<span>✉ ` + esc(c.Email) + `</span><span>📍 ` + esc(c.Address) + `</span><span>☎ ` + esc(c.Phone) + `</span><span>Find More at<br>` + esc(c.Website) + `</span></footer></section></main></body></html>`
}

func printCSS() string {
	return `<style>
	@page{size:A4;margin:0}*{box-sizing:border-box}body{margin:0;background:#e9eef5;font-family:Arial,Helvetica,sans-serif;color:#0f172a}.actions{position:fixed;right:18px;top:18px;z-index:10}.actions button{background:#075f9f;color:#fff;border:0;border-radius:10px;padding:10px 14px;font-weight:800}.doc-page{max-width:900px;margin:24px auto}.doc-shell{background:#fff;min-height:1180px;border:10px solid #2fa8d2;padding:26px 24px 18px;box-shadow:0 14px 45px #0002;position:relative}.doc-shell:before{content:"";position:absolute;inset:0;background:linear-gradient(90deg,rgba(47,168,210,.08),transparent 18%,transparent 82%,rgba(47,168,210,.08));pointer-events:none}.doc-header{position:relative;display:flex;justify-content:space-between;gap:18px;border-bottom:1px solid #dbeafe;padding-bottom:16px}.brand-block{display:flex;align-items:center;gap:12px}.brand-logo{width:72px;height:72px;border-radius:50%;object-fit:cover}.brand-fallback{width:72px;height:72px;border-radius:50%;background:#075f9f;color:#fff;display:flex;align-items:center;justify-content:center;font-weight:900}.brand-block h1{font-size:25px;line-height:1;margin:0;letter-spacing:1px}.brand-block p{margin:4px 0 0;font-size:9px;font-weight:800;letter-spacing:.04em;max-width:420px}.doc-meta{text-align:right;font-size:13px;line-height:1.65}.to-line{position:relative;margin:16px 0 10px;font-size:14px;line-height:1.45}.to-line b{font-family:Georgia,serif;font-size:32px}.to-line span{font-weight:700}.doc-shell h2{text-align:center;letter-spacing:8px;font-size:22px;margin:18px 0 8px}.pills{text-align:center;margin-bottom:18px}.pills span{display:inline-block;background:#eef9ff;border:1px solid #c7edff;color:#075f9f;border-radius:999px;padding:5px 18px;margin:3px;font-size:12px;font-weight:900}.info-grid{display:grid;grid-template-columns:1fr 1fr;gap:16px;margin:14px 0 18px}.info-card{border:1px solid #dbeafe;border-radius:16px;padding:12px;background:#fbfdff;min-height:98px}.info-card b{text-transform:uppercase;letter-spacing:2px;font-size:11px;color:#64748b}.info-card p{font-size:13px;line-height:1.55;margin:8px 0 0}.doc-table{width:100%;border-collapse:separate;border-spacing:0;font-size:12px;overflow:hidden;border-radius:8px}.doc-table th{background:#075f9f;color:white;font-size:10px;text-transform:uppercase;letter-spacing:.04em;padding:9px 7px}.doc-table td{border-bottom:1px solid #e2e8f0;padding:8px 7px;vertical-align:top}.tag{background:#eaf7ff;color:#075f9f;padding:3px 8px;border-radius:999px;font-size:10px;font-weight:800}.num{text-align:right}.lower{display:grid;grid-template-columns:1.2fr .9fr;gap:22px;margin-top:24px}.notes{font-size:12px;line-height:1.45;min-height:180px}.bank{border:1px solid #dbeafe;background:#f8fcff;border-radius:12px;padding:10px;margin-top:12px}.totals{width:100%;font-size:13px;border-collapse:collapse}.totals td{border-bottom:1px solid #e2e8f0;padding:8px}.totals td:last-child{text-align:right;font-weight:800}.totals .grand td{font-size:18px;border-bottom:4px solid #075f9f}.verify-row{display:flex;align-items:flex-end;justify-content:space-between;margin-top:70px}.qrbox{font-size:11px}.qrbox pre{font-family:monospace;font-size:6px;line-height:6px;border:1px solid #111;display:inline-block;padding:5px;margin:0 0 6px}.signature{text-align:center;width:300px}.sigline{border-top:1px solid #111;margin-bottom:8px}.stamp{max-width:120px;max-height:80px;object-fit:contain;margin-top:8px}footer{position:absolute;left:24px;right:24px;bottom:20px;border-top:1px solid #e2e8f0;padding-top:12px;display:grid;grid-template-columns:70px 1fr 1.5fr 1fr 1fr;gap:10px;align-items:center;font-size:11px;color:#334155}.leaf{width:50px;height:auto}.linked{margin-top:18px;font-size:11px;color:#64748b}.letter .letter-subject{margin:42px 0 20px;font-size:15px}.letter-body{font-size:15px;line-height:1.75;min-height:620px;white-space:normal}@media print{body{background:#fff}.actions{display:none}.doc-page{margin:0;max-width:none}.doc-shell{box-shadow:none;border-width:8px;min-height:1120px}}
	</style>`
}

func companyLogoHTML(c Company) string {
	esc := template.HTMLEscapeString
	if strings.TrimSpace(c.LogoData) != "" {
		return `<img class="brand-logo" src="` + esc(c.LogoData) + `" alt="logo">`
	}
	return `<div class="brand-fallback">ZE</div>`
}

func companyLeafHTML(c Company) string {
	esc := template.HTMLEscapeString
	if strings.TrimSpace(c.LeafData) != "" {
		return `<img class="leaf" src="` + esc(c.LeafData) + `" alt="leaf">`
	}
	return `<span></span>`
}

func docTitle(module string) string {
	switch module {
	case "quotation":
		return "QUOTATION"
	case "proforma_invoice":
		return "PROFORMA INVOICE"
	case "sales_invoice":
		return "SALES INVOICE"
	case "commercial_invoice":
		return "COMMERCIAL INVOICE"
	case "packing_list":
		return "PACKING LIST"
	case "bill_of_lading":
		return "BILL OF LADING"
	case "delivery_note":
		return "DELIVERY NOTE"
	case "receipt_voucher":
		return "RECEIPT VOUCHER"
	case "payment_voucher":
		return "PAYMENT VOUCHER"
	default:
		return strings.ToUpper(moduleLabel(module))
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func numField(fields map[string]string, keys ...string) float64 {
	for _, k := range keys {
		if v := parseNumber(fields[k]); v != 0 {
			return v
		}
	}
	return 0
}

func parseNumber(s string) float64 {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", ""))
	if s == "" {
		return 0
	}
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

func lineItemsFromRecord(rec Record) []docLine {
	rawText := firstNonEmpty(rec.Fields["itemsJSON"], rec.Fields["items"], rec.Fields["linesJSON"], rec.Fields["lines"])
	lines := []docLine{}
	if rawText != "" {
		var raw []map[string]any
		if err := json.Unmarshal([]byte(rawText), &raw); err == nil {
			for _, it := range raw {
				qty := floatAny(it["qty"], it["quantity"])
				price := floatAny(it["unitPrice"], it["price"], it["rate"])
				total := floatAny(it["total"], it["amount"])
				if total == 0 {
					total = qty * price
				}
				lines = append(lines, docLine{Kind: firstStringAny(it["type"], it["itemKind"], it["category"]), Description: firstStringAny(it["description"], it["name"], it["productDescription"]), HSCode: firstStringAny(it["hsCode"], it["HSCode"]), Unit: firstStringAny(it["unit"], it["uom"]), Qty: qty, UnitPrice: price, Total: total, NetWeight: floatAny(it["netWeight"]), GrossWeight: floatAny(it["grossWeight"]), Packages: floatAny(it["packages"], it["cartons"])})
			}
		}
	}
	if len(lines) == 0 {
		qty := numField(rec.Fields, "quantity", "qty")
		price := numField(rec.Fields, "unitPrice", "price")
		total := numField(rec.Fields, "amount", "total", "saleAmount")
		if qty == 0 {
			qty = 1
		}
		if price == 0 && total != 0 {
			price = total / qty
		}
		if total == 0 {
			total = qty * price
		}
		lines = append(lines, docLine{Kind: firstNonEmpty(rec.Fields["type"], rec.Fields["category"], moduleLabel(rec.Module)), Description: firstNonEmpty(rec.Fields["productDescription"], rec.Fields["cargoDescription"], rec.Fields["description"], moduleLabel(rec.Module)), HSCode: rec.Fields["hsCode"], Unit: firstNonEmpty(rec.Fields["unit"], "Unit"), Qty: qty, UnitPrice: price, Total: total, NetWeight: numField(rec.Fields, "netWeight"), GrossWeight: numField(rec.Fields, "grossWeight"), Packages: numField(rec.Fields, "packages")})
	}
	return lines
}

func floatAny(values ...any) float64 {
	for _, v := range values {
		switch x := v.(type) {
		case float64:
			return x
		case float32:
			return float64(x)
		case int:
			return float64(x)
		case int64:
			return float64(x)
		case json.Number:
			f, _ := x.Float64()
			return f
		case string:
			if n := parseNumber(x); n != 0 {
				return n
			}
		}
	}
	return 0
}

func firstStringAny(values ...any) string {
	for _, v := range values {
		if s := strings.TrimSpace(fmt.Sprint(v)); s != "" && s != "<nil>" {
			return s
		}
	}
	return ""
}

func totalsByKind(rec Record, lines []docLine) (products, transport, services, subtotal, discount, tax, total float64) {
	for _, l := range lines {
		kind := strings.ToLower(l.Kind + " " + l.Description)
		switch {
		case strings.Contains(kind, "transport") || strings.Contains(kind, "freight") || strings.Contains(kind, "logistics"):
			transport += l.Total
		case strings.Contains(kind, "service") || strings.Contains(kind, "charge"):
			services += l.Total
		default:
			products += l.Total
		}
	}
	if v := numField(rec.Fields, "productsTotal"); v != 0 { products = v }
	if v := numField(rec.Fields, "transportTotal"); v != 0 { transport = v }
	if v := numField(rec.Fields, "servicesTotal"); v != 0 { services = v }
	subtotal = products + transport + services + numField(rec.Fields, "shipping")
	if v := numField(rec.Fields, "subtotal"); v != 0 { subtotal = v }
	discount = numField(rec.Fields, "discount")
	tax = numField(rec.Fields, "tax")
	if tax == 0 && numField(rec.Fields, "taxRate") != 0 {
		tax = (subtotal - discount) * numField(rec.Fields, "taxRate") / 100
	}
	total = subtotal - discount + tax
	if v := numField(rec.Fields, "total", "amount", "saleAmount"); v != 0 { total = v }
	return
}

func renderItemsTable(lines []docLine) string {
	esc := template.HTMLEscapeString
	var b strings.Builder
	b.WriteString(`<table class="doc-table"><thead><tr><th>#</th><th>Type</th><th>Description</th><th>HS Code</th><th>Unit</th><th>Qty</th><th>Unit Price</th><th>Total</th><th>Net Wt</th><th>Gross Wt</th><th>Packages</th></tr></thead><tbody>`)
	for i, l := range lines {
		b.WriteString(`<tr><td>` + fmt.Sprintf("%d", i+1) + `</td><td><span class="tag">` + esc(firstNonEmpty(l.Kind, "Item")) + `</span></td><td>` + esc(l.Description) + `</td><td>` + esc(l.HSCode) + `</td><td>` + esc(l.Unit) + `</td><td class="num">` + formatQty(l.Qty) + `</td><td class="num">` + formatMoney(l.UnitPrice) + `</td><td class="num">` + formatMoney(l.Total) + `</td><td class="num">` + formatQty(l.NetWeight) + `</td><td class="num">` + formatQty(l.GrossWeight) + `</td><td class="num">` + formatQty(l.Packages) + `</td></tr>`)
	}
	b.WriteString(`</tbody></table>`)
	return b.String()
}

func renderTotalsTable(currency string, products, transport, services, subtotal, discount, tax, total float64) string {
	esc := template.HTMLEscapeString
	rows := [][2]string{{"Products", formatMoney(products) + " " + currency}, {"Transportation", formatMoney(transport) + " " + currency}, {"Services/Charges", formatMoney(services) + " " + currency}, {"Subtotal", formatMoney(subtotal) + " " + currency}, {"Discount", formatMoney(discount) + " " + currency}, {"Tax", formatMoney(tax) + " " + currency}}
	var b strings.Builder
	b.WriteString(`<table class="totals">`)
	for _, r := range rows {
		b.WriteString(`<tr><td>` + esc(r[0]) + `</td><td>` + esc(r[1]) + `</td></tr>`)
	}
	b.WriteString(`<tr class="grand"><td>Total</td><td>` + esc(formatMoney(total)+" "+currency) + `</td></tr></table>`)
	return b.String()
}

func linkedDocsHTML(rec Record, all []Record) string {
	if rec.JobRef == "" || len(all) == 0 {
		return ""
	}
	esc := template.HTMLEscapeString
	var b strings.Builder
	for _, other := range all {
		if other.JobRef == rec.JobRef && other.ID != rec.ID {
			b.WriteString(`<span class="tag">` + esc(moduleLabel(other.Module)+": "+other.Number+" - "+other.Status) + `</span> `)
		}
	}
	if b.Len() == 0 {
		return ""
	}
	return `<div class="linked"><b>Linked Job Documents:</b><br>` + b.String() + `</div>`
}

func formatDocDate(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.Format("2006-01-02")
	}
	if len(s) >= 10 {
		return s[:10]
	}
	return s
}

func formatQty(v float64) string {
	if v == 0 {
		return "0.00"
	}
	return fmt.Sprintf("%.2f", v)
}

func formatMoney(v float64) string {
	return fmt.Sprintf("%.2f", v)
}

'''
src=re.sub(r'func renderDocHTML\(c Company, rec Record, all \[\]Record\) string \{.*?\n\}\n\nfunc fakeQR', render_new+'func fakeQR', src, count=1, flags=re.S)
# module labels
labels='''func moduleLabel(module string) string {
\tlabels := map[string]string{"customer": "Customer", "supplier": "Supplier", "product": "Product", "lead": "Lead", "rfq": "RFQ", "quotation": "Quotation", "proforma_invoice": "Proforma Invoice", "sales_invoice": "Sales Invoice", "sales_order": "Sales Order", "purchase_order": "Purchase Order", "commercial_invoice": "Commercial Invoice", "packing_list": "Packing List", "shipment": "Shipment", "bill_of_lading": "Bill of Lading", "delivery_note": "Delivery Note", "receipt_voucher": "Receipt Voucher", "payment_voucher": "Payment Voucher", "expense": "Expense", "contract": "Contract", "employee": "Employee", "driver": "Driver", "truck": "Truck", "task": "Task", "compliance": "Compliance File", "bank_account": "Bank Account", "approval": "Approval", "document_upload": "Uploaded Document", "letterhead": "Letterhead", "business_case": "Business Case"}
\tif v := labels[module]; v != "" {
\t\treturn v
\t}
\treturn human(module)
}'''
src=re.sub(r'func moduleLabel\(module string\) string \{.*?\n\}', labels, src, count=1, flags=re.S)
# JS replacements
src=src.replace("{name:'Sales',items:['lead','customer','supplier','product','rfq','quotation','proforma_invoice','sales_order','purchase_order']},", "{name:'Sales',items:['lead','customer','supplier','product','rfq','quotation','proforma_invoice','sales_invoice','sales_order','purchase_order']},")
src=src.replace("{name:'Documents',items:['commercial_invoice','packing_list','bill_of_lading','delivery_note','receipt_voucher','payment_voucher','document_upload']},", "{name:'Documents',items:['letterhead','commercial_invoice','packing_list','bill_of_lading','delivery_note','receipt_voucher','payment_voucher','document_upload']},")
src=src.replace("{name:'Admin, Legal & Compliance',items:['employee','contract','task','compliance','bank_account']}", "{name:'Admin, Legal & Compliance',items:['business_case','employee','contract','task','compliance','bank_account']}")
src=src.replace("proforma_invoice:{label:'Proforma Invoices',single:'Proforma Invoice',fields:['customer','productDescription','quantity','amount','currency','paymentTerms','validUntil','remarks']},", "proforma_invoice:{label:'Proforma Invoices',single:'Proforma Invoice',fields:['customer','productDescription','quantity','amount','currency','paymentTerms','validUntil','remarks']},\n sales_invoice:{label:'Sales Invoices',single:'Sales Invoice',fields:['customer','productDescription','quantity','amount','currency','invoiceDate','dueDate','paymentTerms','remarks']},")
src=src.replace("document_upload:{label:'Uploaded Documents',single:'Uploaded Document',fields:['fileName','documentType','recordId','savedPath','sizeBytes','notes']}\n};", "document_upload:{label:'Uploaded Documents',single:'Uploaded Document',fields:['fileName','documentType','recordId','savedPath','sizeBytes','notes']},\n letterhead:{label:'Letterhead / Letters',single:'Letterhead Document',fields:['title','subject','date','body','remarks']},\n business_case:{label:'Business Cases',single:'Business Case',fields:['title','customer','supplier','priority','owner','notes']}\n};")
src=src.replace("var CONVERT={rfq:['quotation'],quotation:['proforma_invoice','sales_order'],proforma_invoice:['commercial_invoice','packing_list'],sales_order:['purchase_order','shipment'],purchase_order:['payment_voucher'],commercial_invoice:['receipt_voucher','packing_list'],shipment:['bill_of_lading','delivery_note'],bill_of_lading:['delivery_note'],delivery_note:['commercial_invoice']};", "var CONVERT={rfq:['quotation'],quotation:['proforma_invoice','sales_invoice','sales_order'],proforma_invoice:['sales_invoice','commercial_invoice','packing_list'],sales_invoice:['receipt_voucher','packing_list'],sales_order:['purchase_order','shipment'],purchase_order:['payment_voucher'],commercial_invoice:['receipt_voucher','packing_list'],shipment:['bill_of_lading','delivery_note'],bill_of_lading:['delivery_note'],delivery_note:['sales_invoice','commercial_invoice']};")
src=src.replace("function defaultStatusJS(m){if(['customer','supplier','contract','quotation','commercial_invoice','payment_voucher','receipt_voucher','bill_of_lading','delivery_note'].indexOf(m)>=0)return 'Pending Approval';", "function defaultStatusJS(m){if(['customer','supplier','contract','quotation','proforma_invoice','sales_invoice','commercial_invoice','payment_voucher','receipt_voucher','bill_of_lading','delivery_note'].indexOf(m)>=0)return 'Pending Approval';")
# Company settings
src=src.replace("<button class=\"btn\" onclick=\"openCompanyModal()\">Edit Company</button>", "<button class=\"btn\" onclick=\"openCompanyModal()\">Edit Company</button> <button class=\"btn secondary\" onclick=\"window.open(&quot;/letterhead&quot;,&quot;_blank&quot;)\">Open Letterhead</button>")
old_open="function openCompanyModal(){var c=state.company;var keys=['name','logoText','stampText','address','phone','email','website','taxNumber','baseCurrency'];"
new_open="function openCompanyModal(){var c=state.company;var keys=['name','legalName','logoText','stampText','slogan','address','city','country','phone','whatsApp','email','website','taxNumber','baseCurrency','bankName','bankAccount','bankIban','bankSwift','defaultNotes','defaultTerms','currencyList','prefix'];"
src=src.replace(old_open,new_open)
old_save="function saveCompany(){var c={};['name','logoText','stampText','address','phone','email','website','taxNumber','baseCurrency'].forEach(function(k){c[k]=$('c_'+k).value});"
new_save="function saveCompany(){var c=Object.assign({},state.company);['name','legalName','logoText','stampText','slogan','address','city','country','phone','whatsApp','email','website','taxNumber','baseCurrency','bankName','bankAccount','bankIban','bankSwift','defaultNotes','defaultTerms','currencyList','prefix'].forEach(function(k){c[k]=$('c_'+k).value});"
src=src.replace(old_save,new_save)
# Title
src=src.replace('<title>Zenith Eclipse ERP</title>','<title>Zenith Eclipse ERP - Letterhead & Invoices</title>')
MAIN.write_text(src)
print('Modified main.go')
