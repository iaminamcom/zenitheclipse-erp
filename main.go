package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"image"
	"image/color"
	"image/png"
	"io"
	"log"
	"mime/multipart"
	"net"
	"net/http"
	"net/smtp"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const appName = "Zenith Eclipse ERP A-to-Z Custom"
const appVersion = "3.3.0-docker-dokploy"

type Company struct {
	Name                string `json:"name"`
	LegalName           string `json:"legalName"`
	Address             string `json:"address"`
	City                string `json:"city"`
	Country             string `json:"country"`
	Phone               string `json:"phone"`
	WhatsApp            string `json:"whatsApp"`
	Email               string `json:"email"`
	Website             string `json:"website"`
	TaxNumber           string `json:"taxNumber"`
	LogoText            string `json:"logoText"`
	StampText           string `json:"stampText"`
	BaseCurrency        string `json:"baseCurrency"`
	Slogan              string `json:"slogan"`
	BankName            string `json:"bankName"`
	BankAccount         string `json:"bankAccount"`
	BankIban            string `json:"bankIban"`
	BankSwift           string `json:"bankSwift"`
	DefaultTerms        string `json:"defaultTerms"`
	DefaultNotes        string `json:"defaultNotes"`
	CurrencyList        string `json:"currencyList"`
	Prefix              string `json:"prefix"`
	LogoData            string `json:"logoData"`
	LeafData            string `json:"leafData"`
	StampData           string `json:"stampData"`
	LabelData           string `json:"labelData"`
	SignatureData       string `json:"signatureData"`
	VerificationBaseURL string `json:"verificationBaseURL"`
}

type User struct {
	ID           string `json:"id"`
	Username     string `json:"username"`
	DisplayName  string `json:"displayName"`
	Role         string `json:"role"`
	Department   string `json:"department"`
	PasswordHash string `json:"passwordHash,omitempty"`
	Active       bool   `json:"active"`
	CreatedAt    string `json:"createdAt"`
}

type PublicUser struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Role        string `json:"role"`
	Department  string `json:"department"`
	Active      bool   `json:"active"`
	CreatedAt   string `json:"createdAt"`
}

type Change struct {
	Version        int               `json:"version"`
	Time           string            `json:"time"`
	User           string            `json:"user"`
	Reason         string            `json:"reason"`
	PreviousStatus string            `json:"previousStatus"`
	PreviousFields map[string]string `json:"previousFields"`
}

type Record struct {
	ID        string            `json:"id"`
	Module    string            `json:"module"`
	Number    string            `json:"number"`
	JobRef    string            `json:"jobRef"`
	Status    string            `json:"status"`
	CreatedAt string            `json:"createdAt"`
	CreatedBy string            `json:"createdBy"`
	UpdatedAt string            `json:"updatedAt"`
	UpdatedBy string            `json:"updatedBy"`
	Version   int               `json:"version"`
	Fields    map[string]string `json:"fields"`
	Links     map[string]string `json:"links"`
	History   []Change          `json:"history"`
}

type AuditEntry struct {
	ID       string `json:"id"`
	Time     string `json:"time"`
	User     string `json:"user"`
	Action   string `json:"action"`
	Module   string `json:"module"`
	RecordID string `json:"recordId"`
	Number   string `json:"number"`
	Details  string `json:"details"`
}

type State struct {
	Company  Company           `json:"company"`
	Users    []User            `json:"users"`
	Records  []Record          `json:"records"`
	Serial   map[string]int    `json:"serial"`
	Audit    []AuditEntry      `json:"audit"`
	Settings map[string]string `json:"settings"`
}

type App struct {
	mu            sync.Mutex
	state         State
	sessions      map[string]string
	dataDir       string
	dataPath      string
	uploadDir     string
	backupDir     string
	loginFailures map[string][]time.Time
}

//go:embed default_data.json
var defaultDataJSON []byte

var prefixes = map[string]string{
	"customer": "CUS", "supplier": "SUP", "product": "PRD", "lead": "LEAD",
	"rfq": "RFQ", "quotation": "QTN", "proforma_invoice": "PI", "sales_invoice": "SI", "sales_order": "SO",
	"purchase_order": "PO", "commercial_invoice": "CI", "packing_list": "PL", "shipment": "SHP",
	"bill_of_lading": "BL", "delivery_note": "DN", "handover_sheet": "HND", "receipt_voucher": "RV", "payment_voucher": "PV",
	"expense": "EXP", "contract": "CTR", "employee": "EMP", "driver": "DRV", "truck": "TRK",
	"task": "TSK", "compliance": "COM", "bank_account": "BANK", "approval": "APR", "document_upload": "DOC", "email_log": "EML",
	"letterhead": "LHD", "terms_template": "TRM", "business_case": "CASE",
}

func main() {
	app := &App{sessions: map[string]string{}, loginFailures: map[string][]time.Time{}}
	if err := app.init(); err != nil {
		log.Fatal(err)
	}
	app.startBackupScheduler()

	mux := http.NewServeMux()
	mux.HandleFunc("/", app.handleIndex)
	mux.HandleFunc("/api/login", app.handleLogin)
	mux.HandleFunc("/api/logout", app.handleLogout)
	mux.HandleFunc("/api/verify-document/", app.handleVerifyDocumentAPI)
	mux.HandleFunc("/api/state", app.requireAuth(app.handleState))
	mux.HandleFunc("/api/company", app.requireAuth(app.handleCompany))
	mux.HandleFunc("/api/serial", app.requireAuth(app.handleSerialUpdate))
	mux.HandleFunc("/api/email/settings", app.requireAuth(app.handleEmailSettings))
	mux.HandleFunc("/api/email/send", app.requireAuth(app.handleEmailSend))
	mux.HandleFunc("/api/email/test-connection", app.requireAuth(app.handleEmailTestConnection))
	mux.HandleFunc("/api/email/send-test", app.requireAuth(app.handleEmailSendTest))
	mux.HandleFunc("/api/backup", app.requireAuth(app.handleFullBackup))
	mux.HandleFunc("/api/backup/full", app.requireAuth(app.handleFullBackup))
	mux.HandleFunc("/api/backup/run", app.requireAuth(app.handleBackupRun))
	mux.HandleFunc("/api/backup/settings", app.requireAuth(app.handleBackupSettings))
	mux.HandleFunc("/api/record", app.requireAuth(app.handleRecordCreate))
	mux.HandleFunc("/api/record/update", app.requireAuth(app.handleRecordUpdate))
	mux.HandleFunc("/api/record/status", app.requireAuth(app.handleRecordStatus))
	mux.HandleFunc("/api/record/convert", app.requireAuth(app.handleRecordConvert))
	mux.HandleFunc("/api/user", app.requireAuth(app.handleUserCreate))
	mux.HandleFunc("/api/user/password", app.requireAuth(app.handlePasswordChange))
	mux.HandleFunc("/api/export", app.requireAuth(app.handleExport))
	mux.HandleFunc("/api/restore/full", app.requireAuth(app.handleRestore))
	mux.HandleFunc("/api/restore", app.requireAuth(app.handleRestore))
	mux.HandleFunc("/api/upload", app.requireAuth(app.handleUpload))
	mux.HandleFunc("/api/ai/extract", app.requireAuth(app.handleAIExtract))
	mux.HandleFunc("/doc/", app.requireAuth(app.handleDocument))
	mux.HandleFunc("/export/", app.requireAuth(app.handleDocumentFileExport))
	mux.HandleFunc("/letterhead", app.requireAuth(app.handleLetterhead))
	mux.HandleFunc("/verify/", app.handleVerify)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })

	ln, url, err := openServerListener()
	if err != nil {
		log.Fatal(err)
	}
	if !isHeadlessRuntime() {
		go func() {
			time.Sleep(300 * time.Millisecond)
			openBrowser(url)
		}()
	}

	log.Println(appName, "running at", url)
	log.Println("Local ERP URL:", url)
	if err := http.Serve(ln, mux); err != nil {
		log.Fatal(err)
	}
}

func (app *App) init() error {
	base := os.Getenv("ZENITH_ERP_HOME")
	if base == "" {
		if runtime.GOOS == "windows" {
			base = os.Getenv("APPDATA")
			if base == "" {
				base, _ = os.UserHomeDir()
			}
			base = filepath.Join(base, "ZenithEclipseERP_AtoZ_Custom_Letterhead_Invoices")
		} else {
			home, _ := os.UserHomeDir()
			if home == "" {
				home = "."
			}
			base = filepath.Join(home, ".zenith_eclipse_erp_atoz_custom_letterhead_invoices")
		}
	}
	app.dataDir = base
	app.uploadDir = filepath.Join(base, "uploads")
	app.backupDir = filepath.Join(base, "backups")
	app.dataPath = filepath.Join(base, "erp_data.json")
	if err := os.MkdirAll(app.uploadDir, 0755); err != nil {
		return err
	}
	_ = os.MkdirAll(app.backupDir, 0755)
	if err := os.MkdirAll(app.backupDir, 0755); err != nil {
		return err
	}
	if _, err := os.Stat(app.dataPath); err != nil {
		if os.IsNotExist(err) {
			app.state = defaultState()
			app.ensureStateDefaultsLocked()
			return app.saveLocked()
		}
		return err
	}
	b, err := os.ReadFile(app.dataPath)
	if err != nil {
		return err
	}
	if len(strings.TrimSpace(string(b))) == 0 {
		app.state = defaultState()
		app.ensureStateDefaultsLocked()
		return app.saveLocked()
	}
	if err := json.Unmarshal(b, &app.state); err != nil {
		backup := app.dataPath + ".broken-" + time.Now().Format("20060102150405")
		_ = os.WriteFile(backup, b, 0644)
		app.state = defaultState()
		app.ensureStateDefaultsLocked()
		return app.saveLocked()
	}
	app.ensureStateDefaultsLocked()
	go app.maybeAutoBackup()
	return app.saveLocked()
}

func defaultState() State {
	if len(defaultDataJSON) > 0 {
		var s State
		if err := json.Unmarshal(defaultDataJSON, &s); err == nil {
			if s.Settings == nil {
				s.Settings = map[string]string{}
			}
			return s
		}
	}
	now := time.Now().Format(time.RFC3339)
	s := State{
		Company: Company{
			Name:                "ZENITH ECLIPSE CO",
			LegalName:           "ZENITH ECLIPSE CO",
			Address:             "Citadel Tower, office 204, Business Bay, Dubai, UAE",
			City:                "Dubai",
			Country:             "United Arab Emirates",
			Phone:               "+971 42 500 715",
			WhatsApp:            "",
			Email:               "info@zenitheclipse.com",
			Website:             "http://www.zenitheclipse.com",
			VerificationBaseURL: "https://www.zenitheclipse.com/verify",
			TaxNumber:           "TRN / VAT No.",
			LogoText:            "ZENITH ECLIPSE",
			StampText:           "AUTHORIZED STAMP",
			BaseCurrency:        "AED",
		},
		Users: []User{{
			ID: genID(), Username: "admin", DisplayName: "System Administrator", Role: "Owner/Admin", Department: "Management",
			PasswordHash: hashPassword("admin", "ChangeMe123!"), Active: true, CreatedAt: now,
		}},
		Records: []Record{},
		Serial:  map[string]int{},
		Audit:   []AuditEntry{},
		Settings: map[string]string{
			"firstRun":       "true",
			"passwordPolicy": "Minimum 8 characters recommended for real use",
			"storage":        "Local JSON MVP database",
		},
	}
	sample := []Record{
		makeSeedRecord("customer", "CUS-"+strconv.Itoa(time.Now().Year())+"-0001", "", "Approved", map[string]string{"name": "Demo Customer LLC", "email": "customer@example.com", "mobile": "+971000000", "country": "UAE", "creditLimit": "50000", "outstandingBalance": "12000", "currency": "AED", "kycStatus": "Pending Review", "riskRating": "Medium"}),
		makeSeedRecord("supplier", "SUP-"+strconv.Itoa(time.Now().Year())+"-0001", "", "Approved", map[string]string{"name": "Demo Supplier Co", "email": "supplier@example.com", "mobile": "+968000000", "country": "Oman", "outstandingBalance": "7500", "currency": "AED", "kybStatus": "Pending Review", "riskRating": "Low"}),
		makeSeedRecord("shipment", "SHP-"+strconv.Itoa(time.Now().Year())+"-0001", "JOB-"+strconv.Itoa(time.Now().Year())+"-0001", "In Transit", map[string]string{"customer": "Demo Customer LLC", "supplier": "Demo Supplier Co", "route": "Jebel Ali to Muscat", "containerNumber": "MSCU1234567", "sealNumber": "SEAL9081", "truckNumber": "TRK-100", "driverName": "Demo Driver", "saleAmount": "18000", "estimatedCost": "12000", "currency": "AED"}),
	}
	s.Records = append(s.Records, sample...)
	s.Serial["customer"] = 1
	s.Serial["supplier"] = 1
	s.Serial["shipment"] = 1
	s.Serial["job"] = 1
	s.Audit = append(s.Audit, AuditEntry{ID: genID(), Time: now, User: "system", Action: "INITIALIZE", Module: "system", Details: "Created local ERP MVP database"})
	return s
}

func makeSeedRecord(module, number, jobRef, status string, fields map[string]string) Record {
	now := time.Now().Format(time.RFC3339)
	return Record{ID: genID(), Module: module, Number: number, JobRef: jobRef, Status: status, CreatedAt: now, CreatedBy: "system", UpdatedAt: now, UpdatedBy: "system", Version: 1, Fields: fields, Links: map[string]string{}, History: []Change{}}
}

func (app *App) ensureStateDefaultsLocked() {
	if app.state.Serial == nil {
		app.state.Serial = map[string]int{}
	}
	if app.state.Settings == nil {
		app.state.Settings = map[string]string{}
	}
	if strings.TrimSpace(app.state.Company.Prefix) == "" {
		app.state.Company.Prefix = "ZE"
	}
	if strings.TrimSpace(app.state.Settings["serialFormat"]) == "" {
		app.state.Settings["serialFormat"] = "{PREFIX}-{MODULE}-{YEAR}-{JOBSEQ}"
	}
	if strings.TrimSpace(app.state.Settings["serialPadding"]) == "" {
		app.state.Settings["serialPadding"] = "4"
	}
	if strings.TrimSpace(app.state.Settings["serialYear"]) == "" {
		app.state.Settings["serialYear"] = strconv.Itoa(time.Now().Year())
	}
	if strings.TrimSpace(app.state.Settings["serialRevision"]) == "" {
		app.state.Settings["serialRevision"] = "R0"
	}
	if strings.TrimSpace(app.state.Settings["jobPrefix"]) == "" {
		app.state.Settings["jobPrefix"] = "ZEN"
	}
	if strings.TrimSpace(app.state.Settings["jobPadding"]) == "" {
		app.state.Settings["jobPadding"] = "5"
	}
	if app.state.Settings["v16WorkflowSerialMigration"] != "done" {
		// New workflow-friendly default. Existing document numbers are not changed.
		oldFormat := strings.TrimSpace(app.state.Settings["serialFormat"])
		if oldFormat == "" || strings.Contains(oldFormat, "{CUSTOMER}") || (strings.Contains(oldFormat, "{SEQ}") && strings.Contains(oldFormat, "{MODULE}")) {
			app.state.Settings["serialFormat"] = "{MODULE}-{YEAR}-{JOBSEQ}"
		}
		app.ensureFinsetaBankAccountLocked()
		app.state.Settings["v16WorkflowSerialMigration"] = "done"
	}
	if app.state.Settings["v15ContactBankMigration"] != "done" {
		app.state.Company.Phone = "+971 42 500 715"
		app.state.Company.WhatsApp = ""
		if strings.Contains(strings.ToLower(strings.TrimSpace(app.state.Company.BankName)), "finseta") {
			app.state.Company.BankName = ""
			app.state.Company.BankAccount = ""
			app.state.Company.BankIban = ""
			app.state.Company.BankSwift = ""
		}
		app.state.Settings["v15ContactBankMigration"] = "done"
	}
	if app.state.Settings["v17FooterWorkflowBankMigration"] != "done" {
		app.ensureFinsetaBankAccountLocked()
		app.state.Settings["documentBorderColor"] = "#003366"
		app.state.Settings["workflowFooterPolicy"] = "Workflow prints compactly in the footer-left area for quotation, proforma invoice, sales invoice, commercial invoice and packing list."
		app.state.Settings["documentBankPolicy"] = "Bank details are hidden unless a saved bank account is selected. Selected bank details print under Terms & Conditions only."
		app.state.Settings["v17FooterWorkflowBankMigration"] = "done"
	}
	if strings.TrimSpace(app.state.Company.VerificationBaseURL) == "" {
		app.state.Company.VerificationBaseURL = normalizeVerificationBase(firstNonEmpty(app.state.Settings["verificationBaseURL"], app.state.Company.Website))
	}
	if app.state.Settings["v18VerticalWorkflowVerifyMigration"] != "done" {
		app.ensureFinsetaBankAccountLocked()
		app.state.Settings["serialFormat"] = "{PREFIX}-{MODULE}-{YEAR}-{JOBSEQ}"
		app.state.Settings["workflowFooterPolicy"] = "Vertical footer workflow with one document per line, same job reference, and different serial number per document type."
		app.state.Settings["verificationPolicy"] = "QR code encodes the server verification URL based on the document serial number. Configure Company Verification Base URL for production."
		if strings.TrimSpace(app.state.Company.VerificationBaseURL) == "" {
			app.state.Company.VerificationBaseURL = "https://www.zenitheclipse.com/verify"
		}
		app.state.Settings["v18VerticalWorkflowVerifyMigration"] = "done"
	}
	if app.state.Settings["v19CompactWorkflowPageBreakMigration"] != "done" {
		app.state.Settings["workflowFooterPolicy"] = "Compact footer workflow list with no arrows and reduced footer height."
		app.state.Settings["letterPageBreakPolicy"] = "Letters/contracts use real A4 chunks with repeated header/footer. Optional forced first-page continuation is available per document."
		app.state.Settings["v19CompactWorkflowPageBreakMigration"] = "done"
	}
	if strings.TrimSpace(app.state.Settings["autoBackupFrequency"]) == "" {
		app.state.Settings["autoBackupFrequency"] = "Off"
	}
	if strings.TrimSpace(app.state.Settings["backupLocalPath"]) == "" {
		app.state.Settings["backupLocalPath"] = app.backupDir
	}
	if strings.TrimSpace(app.state.Settings["backupRetentionCount"]) == "" {
		app.state.Settings["backupRetentionCount"] = "10"
	}
	if app.state.Settings["v20LogisticsBackupVerifyMigration"] != "done" {
		app.state.Settings["logisticsPricePolicy"] = "BL, packing list, handover sheet, delivery note and warehouse/shipping documents hide unit price, amount, total, tax and payment tables unless Show Price Table is selected."
		app.state.Settings["backupPolicy"] = "Full backup ZIP includes database, settings, users, audit, uploaded files, QR records and a manifest. Auto backup can be Off, Daily, Weekly or Monthly."
		app.state.Settings["verificationAPIPolicy"] = "Public API endpoint /api/verify-document/{document_number} returns live verification data from ERP database."
		for i := range app.state.Records {
			if logisticsNoPriceModule(app.state.Records[i].Module) && strings.TrimSpace(app.state.Records[i].Fields["showPriceTable"]) == "" {
				app.state.Records[i].Fields["showPriceTable"] = "Hide"
			}
		}
		app.state.Settings["v20LogisticsBackupVerifyMigration"] = "done"
	}

	if app.state.Settings["v22DocCalcTermsExportMigration"] != "done" {
		app.state.Settings["termsDefaultTemplate"] = firstNonEmpty(app.state.Settings["termsDefaultTemplate"], app.state.Company.DefaultTerms, "Payment terms, delivery terms, validity, and other conditions are subject to final written confirmation.")
		app.state.Settings["documentSummaryPolicy"] = "Document totals print as compact subtotal, optional transportation/services/discount, VAT/tax and final total only."
		app.state.Settings["exportPolicy"] = "Documents can be downloaded as PDF or Word DOCX from each record action menu."
		for i := range app.state.Records {
			app.normalizeRecordFieldsLocked(app.state.Records[i].Module, app.state.Records[i].Fields)
		}
		app.state.Settings["v22DocCalcTermsExportMigration"] = "done"
	}

	if app.state.Settings["v23CustomerEmailLayoutSMTPMigration"] != "done" {
		app.state.Settings["customerAutofillPolicy"] = "Customer selection fetches name, address, email, phone, TRN/VAT, contact person, currency, terms, credit limit and balance from the customer master."
		app.state.Settings["footerLayoutPolicy"] = "Footer first row: Address | Email/Web | Phone | QR. Second row: compact horizontal workflow. No workflow boxes in document body."
		app.state.Settings["smtpPolicy"] = "SMTP settings include test connection and send test email with clear diagnostics for authentication, TLS, timeout and provider errors."
		app.sanitizeAllEmailFieldsLocked()
		for i := range app.state.Records {
			if logisticsNoPriceModule(app.state.Records[i].Module) && strings.TrimSpace(app.state.Records[i].Fields["termsOption"]) == "" {
				app.state.Records[i].Fields["termsOption"] = "Hide"
			}
		}
		app.state.Settings["v23CustomerEmailLayoutSMTPMigration"] = "done"
	}

	if app.state.Settings["v24FooterWorkflowPlacementMigration"] != "done" {
		app.state.Settings["footerLayoutPolicy"] = "Footer keeps the same height: contact/QR row on top and one-line compact workflow placed in the existing bottom footer space."
		app.state.Settings["v24FooterWorkflowPlacementMigration"] = "done"
	}

	if app.state.Users == nil || len(app.state.Users) == 0 {
		app.state.Users = []User{{ID: genID(), Username: "admin", DisplayName: "System Administrator", Role: "Owner/Admin", Department: "Management", PasswordHash: hashPassword("admin", "ChangeMe123!"), Active: true, CreatedAt: time.Now().Format(time.RFC3339)}}
	}
	for i := range app.state.Records {
		if app.state.Records[i].Fields == nil {
			app.state.Records[i].Fields = map[string]string{}
		}
		if app.state.Records[i].Links == nil {
			app.state.Records[i].Links = map[string]string{}
		}
		if app.state.Records[i].Version == 0 {
			app.state.Records[i].Version = 1
		}
	}
}

func (app *App) ensureFinsetaBankAccountLocked() {
	for _, r := range app.state.Records {
		if r.Module == "bank_account" && (strings.Contains(strings.ToLower(r.Fields["bankName"]), "finseta") || strings.Contains(strings.ToUpper(r.Fields["iban"]), "LU514080000028949442")) {
			return
		}
	}
	now := time.Now().Format(time.RFC3339)
	fields := map[string]string{"bankName": "Finseta", "accountName": "ZENITH ECLIPSE GENERAL TRADING L.L.C", "iban": "LU514080000028949442", "swift": "To be added", "currency": firstNonEmpty(app.state.Company.BaseCurrency, "USD"), "openingBalance": "0", "currentBalance": "0", "type": "bank", "notes": "Selectable bank account for document templates"}
	app.state.Records = append(app.state.Records, Record{ID: "bank-finseta-default", Module: "bank_account", Number: "BANK-FINSETA", JobRef: "", Status: "Approved", CreatedAt: now, CreatedBy: "system", UpdatedAt: now, UpdatedBy: "system", Version: 1, Fields: fields, Links: map[string]string{}, History: []Change{}})
}

func (app *App) saveLocked() error {
	app.ensureStateDefaultsLocked()
	tmp := app.dataPath + ".tmp"
	b, err := json.MarshalIndent(app.state, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, app.dataPath)
}

func genID() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err == nil {
		return hex.EncodeToString(b)
	}
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func hashPassword(username, password string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(username) + ":zenith-eclipse-local-mvp:" + password))
	return hex.EncodeToString(sum[:])
}

func openServerListener() (net.Listener, string, error) {
	bindHost := strings.TrimSpace(os.Getenv("ZENITH_ERP_BIND"))
	if bindHost == "" {
		if isDockerRuntime() {
			bindHost = "0.0.0.0"
		} else {
			bindHost = "127.0.0.1"
		}
	}
	port := strings.TrimSpace(os.Getenv("ZENITH_ERP_PORT"))
	if port == "" {
		port = "8080"
	}
	ln, err := net.Listen("tcp", net.JoinHostPort(bindHost, port))
	if err != nil && port == "8080" {
		// Keep the URL predictable but avoid failing if another app already uses 8080.
		port = "18080"
		ln, err = net.Listen("tcp", net.JoinHostPort(bindHost, port))
	}
	if err != nil {
		return nil, "", err
	}
	_, actualPort, splitErr := net.SplitHostPort(ln.Addr().String())
	if splitErr != nil || actualPort == "" {
		actualPort = port
	}
	displayHost := strings.TrimSpace(os.Getenv("ZENITH_ERP_URL_HOST"))
	if displayHost == "" {
		switch strings.ToLower(bindHost) {
		case "127.0.0.1", "localhost", "::1":
			displayHost = "localhost"
		case "0.0.0.0", "::", "[::]":
			displayHost = firstLANIPv4()
			if displayHost == "" {
				displayHost = "localhost"
			}
		default:
			displayHost = bindHost
		}
	}
	return ln, "http://" + net.JoinHostPort(displayHost, actualPort), nil
}

func firstLANIPv4() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil {
				continue
			}
			ip = ip.To4()
			if ip == nil || ip.IsLoopback() {
				continue
			}
			return ip.String()
		}
	}
	return ""
}

func isDockerRuntime() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("ZENITH_ERP_DOCKER")))
	return v == "1" || v == "true" || v == "yes"
}

func isHeadlessRuntime() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("ZENITH_ERP_HEADLESS")))
	return isDockerRuntime() || v == "1" || v == "true" || v == "yes"
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

func secureHeaders(w http.ResponseWriter) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	secureHeaders(w)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func errorJSON(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"ok": false, "error": message})
}

func (app *App) handleIndex(w http.ResponseWriter, r *http.Request) {
	secureHeaders(w)
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(indexHTML))
}

func (app *App) requireAuth(fn http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if app.sessionUser(r) == "" {
			errorJSON(w, http.StatusUnauthorized, "login required")
			return
		}
		fn(w, r)
	}
}

func (app *App) sessionUser(r *http.Request) string {
	c, err := r.Cookie("zenith_session")
	if err != nil || c.Value == "" {
		return ""
	}
	app.mu.Lock()
	defer app.mu.Unlock()
	return app.sessions[c.Value]
}

func (app *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorJSON(w, 405, "POST required")
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorJSON(w, 400, "invalid json")
		return
	}
	username := strings.TrimSpace(req.Username)
	client := clientIP(r)
	app.mu.Lock()
	defer app.mu.Unlock()
	if app.loginFailures == nil {
		app.loginFailures = map[string][]time.Time{}
	}
	if app.loginBlockedLocked(username, client) {
		app.addAuditLocked(username, "LOGIN_BLOCKED", "auth", "", "", "Too many failed login attempts from "+client)
		_ = app.saveLocked()
		errorJSON(w, http.StatusTooManyRequests, "too many failed login attempts; wait 15 minutes")
		return
	}
	for _, u := range app.state.Users {
		if strings.EqualFold(u.Username, username) && u.Active && u.PasswordHash == hashPassword(u.Username, req.Password) {
			token := genID() + genID()
			app.sessions[token] = u.Username
			delete(app.loginFailures, loginFailureKey(username, client))
			http.SetCookie(w, &http.Cookie{Name: "zenith_session", Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: 8 * 60 * 60})
			app.addAuditLocked(u.Username, "LOGIN", "auth", "", "", "User logged in from "+client)
			_ = app.saveLocked()
			writeJSON(w, 200, map[string]any{"ok": true, "user": publicUser(u)})
			return
		}
	}
	app.recordLoginFailureLocked(username, client)
	app.addAuditLocked(username, "LOGIN_FAILED", "auth", "", "", "Invalid login attempt from "+client)
	_ = app.saveLocked()
	errorJSON(w, http.StatusUnauthorized, "invalid username or password")
}

func clientIP(r *http.Request) string {
	if fwd := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); fwd != "" {
		parts := strings.Split(fwd, ",")
		return strings.TrimSpace(parts[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	return r.RemoteAddr
}

func loginFailureKey(username, ip string) string {
	return strings.ToLower(strings.TrimSpace(username)) + "|" + strings.TrimSpace(ip)
}

func (app *App) loginBlockedLocked(username, ip string) bool {
	key := loginFailureKey(username, ip)
	now := time.Now()
	cutoff := now.Add(-15 * time.Minute)
	attempts := app.loginFailures[key]
	kept := attempts[:0]
	for _, t := range attempts {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	app.loginFailures[key] = kept
	return len(kept) >= 5
}

func (app *App) recordLoginFailureLocked(username, ip string) {
	key := loginFailureKey(username, ip)
	app.loginFailures[key] = append(app.loginFailures[key], time.Now())
}

func (app *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	token := ""
	if c, err := r.Cookie("zenith_session"); err == nil {
		token = c.Value
	}
	app.mu.Lock()
	if token != "" {
		delete(app.sessions, token)
	}
	app.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: "zenith_session", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	writeJSON(w, 200, map[string]any{"ok": true})
}

func publicUser(u User) PublicUser {
	return PublicUser{ID: u.ID, Username: u.Username, DisplayName: u.DisplayName, Role: u.Role, Department: u.Department, Active: u.Active, CreatedAt: u.CreatedAt}
}

func (app *App) handleState(w http.ResponseWriter, r *http.Request) {
	user := app.sessionUser(r)
	app.mu.Lock()
	defer app.mu.Unlock()
	users := make([]PublicUser, 0, len(app.state.Users))
	var me PublicUser
	for _, u := range app.state.Users {
		pu := publicUser(u)
		users = append(users, pu)
		if strings.EqualFold(u.Username, user) {
			me = pu
		}
	}
	records := append([]Record(nil), app.state.Records...)
	sort.Slice(records, func(i, j int) bool { return records[i].UpdatedAt > records[j].UpdatedAt })
	audit := append([]AuditEntry(nil), app.state.Audit...)
	sort.Slice(audit, func(i, j int) bool { return audit[i].Time > audit[j].Time })
	if len(audit) > 500 {
		audit = audit[:500]
	}
	serial := map[string]int{}
	for k, v := range app.state.Serial {
		serial[k] = v
	}
	settingsPublic := copyMap(app.state.Settings)
	if strings.TrimSpace(settingsPublic["smtpPassword"]) != "" {
		settingsPublic["smtpPassword"] = ""
		settingsPublic["smtpPasswordSet"] = "yes"
	}
	writeJSON(w, 200, map[string]any{
		"ok": true, "appName": appName, "version": appVersion, "company": app.state.Company, "records": records,
		"audit": audit, "users": users, "me": me, "dataDir": app.dataDir, "settings": settingsPublic, "serial": serial, "prefixes": prefixes,
	})
}

func (app *App) handleCompany(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorJSON(w, 405, "POST required")
		return
	}
	user := app.sessionUser(r)
	var c Company
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		errorJSON(w, 400, "invalid json")
		return
	}
	if err := validateEmailValue("company email", c.Email); err != nil {
		errorJSON(w, 400, err.Error())
		return
	}
	c.Email = normalizeEmailValue(c.Email)
	app.mu.Lock()
	defer app.mu.Unlock()
	c.Phone = singlePhone(c)
	c.WhatsApp = ""
	if strings.Contains(strings.ToLower(strings.TrimSpace(c.BankName)), "finseta") {
		c.BankName = ""
		c.BankAccount = ""
		c.BankIban = ""
		c.BankSwift = ""
	}
	app.state.Company = c
	app.addAuditLocked(user, "UPDATE_COMPANY", "settings", "", "", "Company settings updated")
	if err := app.saveLocked(); err != nil {
		errorJSON(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "company": c})
}

func (app *App) handleSerialUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorJSON(w, 405, "POST required")
		return
	}
	user := app.sessionUser(r)
	var req struct {
		Serial   map[string]int    `json:"serial"`
		Settings map[string]string `json:"settings"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorJSON(w, 400, "invalid json")
		return
	}
	app.mu.Lock()
	defer app.mu.Unlock()
	if app.state.Serial == nil {
		app.state.Serial = map[string]int{}
	}
	for k, v := range req.Serial {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		if v < 0 {
			v = 0
		}
		app.state.Serial[key] = v
	}
	if app.state.Settings == nil {
		app.state.Settings = map[string]string{}
	}
	for _, k := range []string{"serialFormat", "serialPadding", "serialYear", "serialRevision", "jobPrefix", "jobPadding"} {
		if v, ok := req.Settings[k]; ok {
			app.state.Settings[k] = strings.TrimSpace(v)
		}
	}
	app.addAuditLocked(user, "UPDATE_SERIAL", "settings", "", "", "Serial number settings updated")
	if err := app.saveLocked(); err != nil {
		errorJSON(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "serial": app.state.Serial, "settings": app.state.Settings})
}

func (app *App) handleEmailSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorJSON(w, 405, "POST required")
		return
	}
	user := app.sessionUser(r)
	var req map[string]string
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorJSON(w, 400, "invalid json")
		return
	}
	for _, pair := range []struct {
		key   string
		label string
	}{{"smtpUser", "SMTP username/email"}, {"smtpFrom", "sender email"}} {
		if v := strings.TrimSpace(req[pair.key]); v != "" {
			if err := validateEmailValue(pair.label, v); err != nil {
				errorJSON(w, 400, err.Error())
				return
			}
			req[pair.key] = normalizeEmailValue(v)
		}
	}
	app.mu.Lock()
	defer app.mu.Unlock()
	if app.state.Settings == nil {
		app.state.Settings = map[string]string{}
	}
	for _, k := range []string{"smtpHost", "smtpPort", "smtpUser", "smtpFrom", "smtpFromName", "smtpSecurity", "emailSignature"} {
		if v, ok := req[k]; ok {
			if strings.Contains(strings.ToLower(k), "email") || k == "smtpUser" || k == "smtpFrom" {
				v = normalizeEmailValue(v)
			}
			app.state.Settings[k] = strings.TrimSpace(v)
		}
	}
	if pw, ok := req["smtpPassword"]; ok && strings.TrimSpace(pw) != "" && pw != "__KEEP__" {
		app.state.Settings["smtpPassword"] = pw
	}
	app.addAuditLocked(user, "UPDATE_EMAIL_SETTINGS", "settings", "", "", "SMTP/email settings updated")
	if err := app.saveLocked(); err != nil {
		errorJSON(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (app *App) handleEmailTestConnection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorJSON(w, 405, "POST required")
		return
	}
	app.mu.Lock()
	settings := copyMap(app.state.Settings)
	company := app.state.Company
	app.mu.Unlock()
	if err := testSMTPConnection(settings, company); err != nil {
		errorJSON(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "message": "SMTP connection and authentication successful"})
}

func (app *App) handleEmailSendTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorJSON(w, 405, "POST required")
		return
	}
	var req struct {
		To string `json:"to"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	app.mu.Lock()
	settings := copyMap(app.state.Settings)
	company := app.state.Company
	app.mu.Unlock()
	to := strings.TrimSpace(req.To)
	if to == "" {
		to = firstNonEmpty(settings["smtpFrom"], company.Email, settings["smtpUser"])
	}
	if err := validateEmailValue("test recipient", to); err != nil {
		errorJSON(w, 400, err.Error())
		return
	}
	body := "This is a test email from Zenith Eclipse ERP SMTP settings.\n\nIf you received this, your company email connection is working."
	if err := sendSMTPMail(settings, company, []string{normalizeEmailValue(to)}, nil, nil, "Zenith Eclipse ERP SMTP Test", body, nil); err != nil {
		errorJSON(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "message": "Test email sent to " + normalizeEmailValue(to)})
}

func (app *App) handleEmailSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorJSON(w, 405, "POST required")
		return
	}
	user := app.sessionUser(r)
	var req struct {
		RecordID string `json:"recordId"`
		To       string `json:"to"`
		Cc       string `json:"cc"`
		Bcc      string `json:"bcc"`
		Subject  string `json:"subject"`
		Body     string `json:"body"`
		Attach   bool   `json:"attach"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorJSON(w, 400, "invalid json")
		return
	}
	if err := validateEmailList("To", req.To); err != nil {
		errorJSON(w, 400, err.Error())
		return
	}
	if err := validateEmailList("CC", req.Cc); err != nil {
		errorJSON(w, 400, err.Error())
		return
	}
	if err := validateEmailList("BCC", req.Bcc); err != nil {
		errorJSON(w, 400, err.Error())
		return
	}
	toList := splitEmails(normalizeEmailValue(req.To))
	if len(toList) == 0 {
		errorJSON(w, 400, "recipient email required")
		return
	}
	app.mu.Lock()
	settings := copyMap(app.state.Settings)
	company := app.state.Company
	records := append([]Record(nil), app.state.Records...)
	var rec *Record
	for i := range records {
		if records[i].ID == req.RecordID {
			rec = &records[i]
			break
		}
	}
	app.mu.Unlock()
	if rec == nil {
		errorJSON(w, 404, "document not found")
		return
	}
	subject := firstNonEmpty(req.Subject, rec.Number+" - "+moduleLabel(rec.Module))
	body := firstNonEmpty(req.Body, "Dear Customer,\n\nPlease find the attached document.\n\nRegards,")
	if sig := strings.TrimSpace(settings["emailSignature"]); sig != "" {
		body += "\n\n" + sig
	}
	attachments := []EmailAttachment{}
	attachmentNote := "No attachment"
	if req.Attach {
		html := renderDocHTML(company, *rec, records)
		if pdf, err := renderPDFBytes(html); err == nil && len(pdf) > 0 {
			attachments = append(attachments, EmailAttachment{FileName: sanitizeFileName(rec.Number) + ".pdf", ContentType: "application/pdf", Data: pdf})
			attachmentNote = "PDF attachment generated from A4 print template"
		} else {
			attachments = append(attachments, EmailAttachment{FileName: sanitizeFileName(rec.Number) + ".html", ContentType: "text/html; charset=utf-8", Data: []byte(html)})
			attachmentNote = "Printable HTML attached because local browser PDF engine was not available"
		}
	}
	if err := sendSMTPMail(settings, company, toList, splitEmails(req.Cc), splitEmails(req.Bcc), subject, body, attachments); err != nil {
		errorJSON(w, 500, err.Error())
		return
	}
	app.mu.Lock()
	fields := map[string]string{"to": req.To, "cc": req.Cc, "bcc": req.Bcc, "subject": subject, "body": body, "documentNumber": rec.Number, "documentId": rec.ID, "customer": firstNonEmpty(rec.Fields["customer"], rec.Fields["partyName"], rec.Fields["to"]), "sentAt": time.Now().Format(time.RFC3339), "attachment": attachmentNote}
	logRec := app.createRecordLocked(user, "email_log", fields, "Sent", rec.JobRef, map[string]string{"document": rec.ID})
	app.addAuditLocked(user, "EMAIL_SENT", "email_log", logRec.ID, logRec.Number, "Email sent for "+rec.Number+" to "+req.To)
	_ = app.saveLocked()
	app.mu.Unlock()
	writeJSON(w, 200, map[string]any{"ok": true, "message": "email sent"})
}

type EmailAttachment struct {
	FileName    string
	ContentType string
	Data        []byte
}

var strictEmailRegex = regexp.MustCompile(`^[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}$`)

func normalizeEmailValue(s string) string {
	s = strings.TrimSpace(s)
	if strings.Contains(s, "@") {
		parts := strings.SplitN(s, "@", 2)
		parts[1] = strings.ReplaceAll(parts[1], ",", ".")
		s = parts[0] + "@" + parts[1]
	}
	return s
}

func validateEmailValue(label, email string) error {
	email = strings.TrimSpace(email)
	if email == "" {
		return nil
	}
	if strings.Contains(email, ",") {
		return fmt.Errorf("invalid %s %q: use a dot in the domain, for example info@zscm.ae, not info@zscm,ae", label, email)
	}
	if !strictEmailRegex.MatchString(email) {
		return fmt.Errorf("invalid %s %q: email must look like name@domain.com", label, email)
	}
	return nil
}

func validateEmailList(label, raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if strings.Contains(raw, ",") && strings.Count(raw, "@") == 1 && !strings.Contains(raw, " ") && !strings.Contains(raw, ";") {
		return fmt.Errorf("invalid %s email %q: use a dot in the domain, for example info@zscm.ae, not info@zscm,ae", label, raw)
	}
	parts := splitEmails(raw)
	if len(parts) == 0 && strings.Contains(raw, "@") {
		return fmt.Errorf("invalid %s email %q", label, raw)
	}
	for _, e := range parts {
		if err := validateEmailValue(label, e); err != nil {
			return err
		}
	}
	return nil
}

func validateEmailFields(fields map[string]string) error {
	for k, v := range fields {
		lk := strings.ToLower(k)
		if strings.Contains(lk, "email") && strings.TrimSpace(v) != "" {
			if err := validateEmailValue(k, v); err != nil {
				return err
			}
		}
	}
	return nil
}

func normalizeEmailFields(fields map[string]string) {
	for k, v := range fields {
		if strings.Contains(strings.ToLower(k), "email") {
			fields[k] = normalizeEmailValue(v)
		}
	}
}

func (app *App) sanitizeAllEmailFieldsLocked() {
	app.state.Company.Email = normalizeEmailValue(app.state.Company.Email)
	for i := range app.state.Records {
		if app.state.Records[i].Fields != nil {
			normalizeEmailFields(app.state.Records[i].Fields)
		}
	}
	if app.state.Settings != nil {
		for _, k := range []string{"smtpUser", "smtpFrom"} {
			app.state.Settings[k] = normalizeEmailValue(app.state.Settings[k])
		}
	}
}

func splitEmails(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ';' || r == '\n' || r == '\r' || r == ' ' || r == '\t' })
	out := []string{}
	seen := map[string]bool{}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || !strings.Contains(p, "@") {
			continue
		}
		lp := strings.ToLower(p)
		if !seen[lp] {
			out = append(out, p)
			seen[lp] = true
		}
	}
	return out
}

func sanitizeFileName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		s = "document"
	}
	s = regexp.MustCompile(`[^A-Za-z0-9_.-]+`).ReplaceAllString(s, "-")
	s = strings.Trim(s, "-_.")
	if s == "" {
		s = "document"
	}
	return s
}

func renderPDFBytes(html string) ([]byte, error) {
	tmpDir, err := os.MkdirTemp("", "zenith-pdf-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)
	htmlPath := filepath.Join(tmpDir, "document.html")
	pdfPath := filepath.Join(tmpDir, "document.pdf")
	if err := os.WriteFile(htmlPath, []byte(html), 0600); err != nil {
		return nil, err
	}

	// Linux/server fallback used for reliable testing and headless deployments.
	// Windows users normally use Microsoft Edge/Chrome below.
	if wp, err := exec.LookPath("weasyprint"); err == nil && runtime.GOOS != "windows" {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		cmd := exec.CommandContext(ctx, wp, htmlPath, pdfPath)
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
		runErr := cmd.Run()
		ctxErr := ctx.Err()
		cancel()
		if ctxErr == nil && runErr == nil {
			if b, readErr := os.ReadFile(pdfPath); readErr == nil && len(b) > 0 {
				return b, nil
			}
		}
		if ctxErr != nil {
			return nil, fmt.Errorf("PDF export timed out while using WeasyPrint")
		}
		// keep Chrome fallback available if WeasyPrint fails.
	}

	browser := findHeadlessBrowser()
	if browser == "" {
		return nil, fmt.Errorf("headless browser not found")
	}
	fileURL := "file:///" + strings.ReplaceAll(filepath.ToSlash(htmlPath), " ", "%20")
	args := []string{
		"--headless=new",
		"--disable-gpu",
		"--disable-dev-shm-usage",
		"--disable-crash-reporter",
		"--disable-background-networking",
		"--no-first-run",
		"--disable-extensions",
		"--run-all-compositor-stages-before-draw",
		"--virtual-time-budget=1000",
		"--print-to-pdf-no-header",
		"--no-pdf-header-footer",
		"--print-to-pdf=" + pdfPath,
		fileURL,
	}
	if runtime.GOOS != "windows" {
		args = append([]string{"--no-sandbox"}, args...)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, browser, args...)
	out, runErr := cmd.CombinedOutput()
	ctxErr := ctx.Err()
	if ctxErr != nil {
		return nil, fmt.Errorf("PDF export timed out")
	}
	if runErr != nil {
		return nil, fmt.Errorf("PDF export failed: %v %s", runErr, strings.TrimSpace(string(out)))
	}
	b, err := os.ReadFile(pdfPath)
	if err != nil || len(b) == 0 {
		return nil, fmt.Errorf("PDF export failed")
	}
	return b, nil
}

func findHeadlessBrowser() string {
	candidates := []string{}
	if runtime.GOOS == "windows" {
		for _, base := range []string{os.Getenv("PROGRAMFILES"), os.Getenv("PROGRAMFILES(X86)"), os.Getenv("LOCALAPPDATA")} {
			if base == "" {
				continue
			}
			candidates = append(candidates,
				filepath.Join(base, "Microsoft", "Edge", "Application", "msedge.exe"),
				filepath.Join(base, "Google", "Chrome", "Application", "chrome.exe"),
			)
		}
		candidates = append(candidates, "msedge", "chrome", "chromium")
	} else {
		candidates = append(candidates, "chromium", "chromium-browser", "google-chrome", "microsoft-edge", "chrome")
	}
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if filepath.IsAbs(c) {
			if _, err := os.Stat(c); err == nil {
				return c
			}
			continue
		}
		if p, err := exec.LookPath(c); err == nil {
			return p
		}
	}
	return ""
}

func encodeBase64Lines(data []byte) string {
	enc := base64.StdEncoding.EncodeToString(data)
	var sb strings.Builder
	for len(enc) > 76 {
		sb.WriteString(enc[:76] + "\r\n")
		enc = enc[76:]
	}
	sb.WriteString(enc + "\r\n")
	return sb.String()
}

func openSMTPClient(settings map[string]string, company Company) (*smtp.Client, error) {
	host := strings.TrimSpace(settings["smtpHost"])
	port := firstNonEmpty(settings["smtpPort"], "587")
	if host == "" {
		return nil, fmt.Errorf("SMTP host is not configured")
	}
	user := strings.TrimSpace(settings["smtpUser"])
	password := settings["smtpPassword"]
	security := strings.ToLower(firstNonEmpty(settings["smtpSecurity"], "starttls"))
	addr := net.JoinHostPort(host, port)
	dialer := &net.Dialer{Timeout: 20 * time.Second}
	var c *smtp.Client
	if security == "ssl" || security == "tls" || port == "465" {
		conn, err := tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
		if err != nil {
			return nil, fmt.Errorf("SSL/TLS connection error: check SMTP host, port, TLS setting, firewall, or provider block: %w", err)
		}
		c, err = smtp.NewClient(conn, host)
		if err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("SMTP greeting error after TLS connection: %w", err)
		}
	} else {
		conn, err := dialer.Dial("tcp", addr)
		if err != nil {
			return nil, fmt.Errorf("SMTP connection timeout or wrong host/port: %w", err)
		}
		c, err = smtp.NewClient(conn, host)
		if err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("SMTP server greeting error: %w", err)
		}
		if security == "starttls" || port == "587" {
			ok, _ := c.Extension("STARTTLS")
			if !ok {
				_ = c.Close()
				return nil, fmt.Errorf("STARTTLS is not supported by the SMTP server on this port; try SSL/TLS port 465 or None only if your server allows it")
			}
			if err := c.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}); err != nil {
				_ = c.Close()
				return nil, fmt.Errorf("STARTTLS/SSL error: check encryption type, port and certificate: %w", err)
			}
		}
	}
	if user != "" && password != "" {
		if err := c.Auth(smtp.PlainAuth("", user, password, host)); err != nil {
			_ = c.Close()
			return nil, fmt.Errorf("authentication failed: wrong password/app password, provider blocked SMTP, or account does not allow SMTP: %w", err)
		}
	} else if user != "" && password == "" {
		_ = c.Close()
		return nil, fmt.Errorf("SMTP password/app password is missing for username %s", user)
	}
	return c, nil
}

func testSMTPConnection(settings map[string]string, company Company) error {
	c, err := openSMTPClient(settings, company)
	if err != nil {
		return err
	}
	defer c.Close()
	return c.Quit()
}

func sendSMTPMail(settings map[string]string, company Company, to, cc, bcc []string, subject, body string, attachments []EmailAttachment) error {
	host := strings.TrimSpace(settings["smtpHost"])
	if host == "" {
		return fmt.Errorf("SMTP host is not configured")
	}
	from := normalizeEmailValue(firstNonEmpty(settings["smtpFrom"], company.Email, settings["smtpUser"]))
	if err := validateEmailValue("sender email", from); err != nil {
		return err
	}
	fromName := firstNonEmpty(settings["smtpFromName"], company.Name)
	boundary := "ZENITH-EMAIL-" + genID()
	allRecipients := append(append(append([]string{}, to...), cc...), bcc...)
	var msg strings.Builder
	msg.WriteString("From: " + formatAddress(fromName, from) + "\r\n")
	msg.WriteString("To: " + strings.Join(to, ", ") + "\r\n")
	if len(cc) > 0 {
		msg.WriteString("Cc: " + strings.Join(cc, ", ") + "\r\n")
	}
	msg.WriteString("Subject: " + strings.ReplaceAll(subject, "\r", " ") + "\r\n")
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: multipart/mixed; boundary=\"" + boundary + "\"\r\n\r\n")
	msg.WriteString("--" + boundary + "\r\n")
	msg.WriteString("Content-Type: text/plain; charset=utf-8\r\n\r\n")
	msg.WriteString(body + "\r\n\r\n")
	for _, a := range attachments {
		msg.WriteString("--" + boundary + "\r\n")
		ct := firstNonEmpty(a.ContentType, "application/octet-stream")
		fn := sanitizeFileName(a.FileName)
		msg.WriteString("Content-Type: " + ct + "; name=\"" + fn + "\"\r\n")
		msg.WriteString("Content-Transfer-Encoding: base64\r\n")
		msg.WriteString("Content-Disposition: attachment; filename=\"" + fn + "\"\r\n\r\n")
		msg.WriteString(encodeBase64Lines(a.Data))
		msg.WriteString("\r\n")
	}
	msg.WriteString("--" + boundary + "--\r\n")
	c, err := openSMTPClient(settings, company)
	if err != nil {
		return err
	}
	defer c.Close()
	if err := c.Mail(from); err != nil {
		return fmt.Errorf("sender rejected: %w", err)
	}
	for _, rcpt := range allRecipients {
		if err := c.Rcpt(rcpt); err != nil {
			return fmt.Errorf("recipient rejected for %s: %w", rcpt, err)
		}
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("SMTP data command failed: %w", err)
	}
	if _, err := w.Write([]byte(msg.String())); err != nil {
		_ = w.Close()
		return fmt.Errorf("failed to upload email content to SMTP server: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("SMTP server rejected email body or attachments: %w", err)
	}
	if err := c.Quit(); err != nil {
		return fmt.Errorf("SMTP quit/finish error: %w", err)
	}
	return nil
}

func formatAddress(name, email string) string {
	email = strings.TrimSpace(email)
	name = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(name, "\r", " "), "\n", " "))
	if name == "" {
		return email
	}
	name = strings.ReplaceAll(name, `"`, "'")
	return `"` + name + `" <` + email + `>`
}

func (app *App) handleRecordCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorJSON(w, 405, "POST required")
		return
	}
	user := app.sessionUser(r)
	var req struct {
		Module string            `json:"module"`
		Fields map[string]string `json:"fields"`
		Status string            `json:"status"`
		JobRef string            `json:"jobRef"`
		Links  map[string]string `json:"links"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorJSON(w, 400, "invalid json")
		return
	}
	module := cleanModule(req.Module)
	if module == "" {
		errorJSON(w, 400, "module required")
		return
	}
	if err := validateEmailFields(req.Fields); err != nil {
		errorJSON(w, 400, err.Error())
		return
	}
	app.mu.Lock()
	defer app.mu.Unlock()
	rec := app.createRecordLocked(user, module, req.Fields, req.Status, req.JobRef, req.Links)
	app.addAuditLocked(user, "CREATE", module, rec.ID, rec.Number, "Record created")
	app.afterRecordChangeLocked(user, &rec)
	if err := app.saveLocked(); err != nil {
		errorJSON(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "record": rec})
}

func cleanModule(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "-", "_")
	return s
}

func (app *App) createRecordLocked(user, module string, fields map[string]string, status, jobRef string, links map[string]string) Record {
	if fields == nil {
		fields = map[string]string{}
	}
	if links == nil {
		links = map[string]string{}
	}
	now := time.Now().Format(time.RFC3339)
	if status == "" {
		status = defaultStatus(module, fields)
	}
	if jobRef == "" {
		jobRef = fields["jobRef"]
	}
	if jobRef == "" && startsBusinessFlow(module) {
		jobRef = app.nextJobRefLocked()
	}
	if jobRef != "" {
		fields["jobRef"] = jobRef
		if fields["jobSequence"] == "" {
			fields["jobSequence"] = sequenceFromJobRef(jobRef)
		}
	}
	app.normalizeRecordFieldsLocked(module, fields)
	rec := Record{ID: genID(), Module: module, Number: app.nextNumberLocked(module, fields), JobRef: jobRef, Status: status, CreatedAt: now, CreatedBy: user, UpdatedAt: now, UpdatedBy: user, Version: 1, Fields: copyMap(fields), Links: copyMap(links), History: []Change{}}
	app.state.Records = append(app.state.Records, rec)
	return rec
}

func (app *App) normalizeRecordFieldsLocked(module string, fields map[string]string) {
	if fields == nil {
		return
	}
	normalizeEmailFields(fields)
	if module != "customer" && module != "supplier" && module != "bank_account" {
		app.autofillCustomerFieldsLocked(fields)
		app.autofillProductFieldsLocked(fields)
	}
	normalizeFinancialFields(module, fields)
}

func (app *App) autofillCustomerFieldsLocked(fields map[string]string) {
	customer := app.findCustomerForFieldsLocked(fields)
	if customer == nil || customer.Fields == nil {
		return
	}
	name := firstNonEmpty(customer.Fields["customerName"], customer.Fields["name"], customer.Fields["companyName"], customer.Fields["legalName"], customer.Number)
	if name == "" {
		return
	}
	setAlways := func(k, v string) {
		if strings.TrimSpace(v) != "" {
			fields[k] = strings.TrimSpace(v)
		}
	}
	setIfBlank := func(k, v string) {
		if strings.TrimSpace(fields[k]) == "" && strings.TrimSpace(v) != "" {
			fields[k] = strings.TrimSpace(v)
		}
	}
	setAlways("customerId", customer.ID)
	setAlways("customerNumber", customer.Number)
	setAlways("customer", name)
	setAlways("customerName", name)
	setAlways("buyer", name)
	setAlways("partyName", name)
	setAlways("to", name)
	setAlways("customerCode", firstNonEmpty(customer.Fields["customerCode"], customer.Fields["code"], codeFromFields(customer.Fields), codeFromFields(map[string]string{"name": name})))
	address := firstNonEmpty(customer.Fields["address"], customer.Fields["billingAddress"], customer.Fields["deliveryAddress"], customer.Fields["country"])
	setAlways("customerAddress", address)
	setIfBlank("toAddress", address)
	setIfBlank("partyAddress", address)
	email := normalizeEmailValue(firstNonEmpty(customer.Fields["email"], customer.Fields["contactEmail"]))
	setAlways("customerEmail", email)
	setIfBlank("email", email)
	phone := firstNonEmpty(customer.Fields["phone"], customer.Fields["mobile"], customer.Fields["contactPhone"])
	setAlways("customerPhone", phone)
	setIfBlank("phone", phone)
	contactPerson := firstNonEmpty(customer.Fields["contactPerson"], customer.Fields["contact"], customer.Fields["attention"], customer.Fields["mobile"])
	setIfBlank("contactPerson", contactPerson)
	setIfBlank("contact", contactPerson)
	taxNo := firstNonEmpty(customer.Fields["taxNumber"], customer.Fields["trn"], customer.Fields["vatNumber"], customer.Fields["TRN"], customer.Fields["VAT"])
	setAlways("customerTaxNumber", taxNo)
	setIfBlank("taxNumber", taxNo)
	setIfBlank("paymentTerms", customer.Fields["paymentTerms"])
	setIfBlank("currency", customer.Fields["currency"])
	setAlways("creditLimit", customer.Fields["creditLimit"])
	setAlways("outstandingBalance", firstNonEmpty(customer.Fields["outstandingBalance"], customer.Fields["balance"], customer.Fields["currentBalance"]))
}

func (app *App) findCustomerForFieldsLocked(fields map[string]string) *Record {
	candidates := []string{fields["customerCode"], fields["customer"], fields["customerName"], fields["buyer"], fields["partyName"], fields["to"]}
	norm := func(s string) string {
		s = strings.ToLower(strings.TrimSpace(s))
		s = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(s, "")
		return s
	}
	for pass := 0; pass < 2; pass++ {
		for i := range app.state.Records {
			r := &app.state.Records[i]
			if r.Module != "customer" || r.Fields == nil {
				continue
			}
			keys := []string{r.ID, r.Number, r.Fields["customerName"], r.Fields["name"], r.Fields["companyName"], r.Fields["legalName"], r.Fields["customerCode"], r.Fields["code"], r.Fields["email"], r.Fields["phone"], r.Fields["mobile"]}
			for _, cand := range candidates {
				cn := norm(cand)
				if cn == "" {
					continue
				}
				for _, key := range keys {
					kn := norm(key)
					if kn == "" {
						continue
					}
					if pass == 0 && cn == kn {
						return r
					}
					if pass == 1 && (strings.Contains(kn, cn) || strings.Contains(cn, kn)) && len(cn) >= 3 {
						return r
					}
				}
			}
		}
	}
	return nil
}

func (app *App) autofillProductFieldsLocked(fields map[string]string) {
	if fields == nil {
		return
	}
	product := app.findProductForFieldsLocked(fields)
	if product != nil && product.Fields != nil {
		setIfBlank := func(k, v string) {
			if strings.TrimSpace(fields[k]) == "" && strings.TrimSpace(v) != "" {
				fields[k] = strings.TrimSpace(v)
			}
		}
		setIfBlank("productId", product.ID)
		setIfBlank("productCode", firstNonEmpty(product.Fields["sku"], product.Fields["code"], product.Number))
		setIfBlank("productName", firstNonEmpty(product.Fields["name"], product.Number))
		setIfBlank("productDescription", firstNonEmpty(product.Fields["description"], product.Fields["name"], product.Number))
		setIfBlank("description", firstNonEmpty(product.Fields["description"], product.Fields["name"]))
		setIfBlank("hsCode", firstNonEmpty(product.Fields["hsCode"], product.Fields["HSCode"]))
		setIfBlank("unit", product.Fields["unit"])
		setIfBlank("currency", product.Fields["currency"])
		setIfBlank("unitPrice", firstNonEmpty(product.Fields["salePrice"], product.Fields["price"], product.Fields["rate"]))
		setIfBlank("type", product.Fields["category"])
	}
	app.autofillProductRowsLocked(fields)
}

func (app *App) autofillProductRowsLocked(fields map[string]string) {
	rawText := firstNonEmpty(fields["itemsJSON"], fields["items"], fields["linesJSON"], fields["lines"])
	if strings.TrimSpace(rawText) == "" {
		return
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(rawText), &rows); err != nil || len(rows) == 0 {
		return
	}
	changed := false
	for _, row := range rows {
		probe := map[string]string{}
		for _, k := range []string{"productCode", "sku", "code", "product", "productName", "name", "description", "productDescription"} {
			if v, ok := row[k]; ok && strings.TrimSpace(fmt.Sprint(v)) != "" {
				probe[k] = strings.TrimSpace(fmt.Sprint(v))
			}
		}
		product := app.findProductForFieldsLocked(probe)
		if product == nil || product.Fields == nil {
			continue
		}
		setIfBlankAny := func(k string, v string) {
			cur, ok := row[k]
			if (!ok || strings.TrimSpace(fmt.Sprint(cur)) == "" || fmt.Sprint(cur) == "<nil>") && strings.TrimSpace(v) != "" {
				row[k] = strings.TrimSpace(v)
				changed = true
			}
		}
		setIfZeroAny := func(k string, v string) {
			if parseNumber(fmt.Sprint(row[k])) == 0 && strings.TrimSpace(v) != "" && parseNumber(v) != 0 {
				row[k] = strings.TrimSpace(v)
				changed = true
			}
		}
		setIfBlankAny("productCode", firstNonEmpty(product.Fields["sku"], product.Fields["code"], product.Number))
		setIfBlankAny("description", firstNonEmpty(product.Fields["description"], product.Fields["name"], product.Number))
		setIfBlankAny("hsCode", firstNonEmpty(product.Fields["hsCode"], product.Fields["HSCode"]))
		setIfBlankAny("unit", product.Fields["unit"])
		setIfBlankAny("type", product.Fields["category"])
		setIfZeroAny("unitPrice", firstNonEmpty(product.Fields["salePrice"], product.Fields["price"], product.Fields["rate"]))
	}
	if changed {
		if b, err := json.Marshal(rows); err == nil {
			fields["itemsJSON"] = string(b)
		}
	}
}

func (app *App) findProductForFieldsLocked(fields map[string]string) *Record {
	candidates := []string{fields["productId"], fields["productCode"], fields["productSKU"], fields["sku"], fields["code"], fields["product"], fields["productName"], fields["productDescription"], fields["description"], fields["name"]}
	norm := func(s string) string {
		s = strings.ToLower(strings.TrimSpace(s))
		s = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(s, "")
		return s
	}
	for pass := 0; pass < 2; pass++ {
		for i := range app.state.Records {
			r := &app.state.Records[i]
			if r.Module != "product" || r.Fields == nil {
				continue
			}
			keys := []string{r.ID, r.Number, r.Fields["sku"], r.Fields["code"], r.Fields["name"], r.Fields["productName"], r.Fields["description"]}
			for _, cand := range candidates {
				cn := norm(cand)
				if cn == "" {
					continue
				}
				for _, key := range keys {
					kn := norm(key)
					if kn == "" {
						continue
					}
					if pass == 0 && cn == kn {
						return r
					}
					if pass == 1 && len(cn) >= 3 && (strings.Contains(kn, cn) || strings.Contains(cn, kn)) {
						return r
					}
				}
			}
		}
	}
	return nil
}

func normalizeFinancialFields(module string, fields map[string]string) {
	if fields == nil || !financialDocumentModule(module) {
		return
	}
	rec := Record{Module: module, Fields: fields}
	lines := lineItemsFromRecord(rec)
	products, transport, services, subtotal, _, tax, total := totalsByKind(rec, lines)
	if len(lines) == 1 {
		fields["lineTotal"] = formatDataNumber(lines[0].Total)
	}
	fields["subtotal"] = formatDataNumber(subtotal)
	fields["tax"] = formatDataNumber(tax)
	fields["total"] = formatDataNumber(total)
	if showPriceTableForDocument(rec) {
		fields["amount"] = formatDataNumber(total)
		if products != 0 {
			fields["productsTotal"] = formatDataNumber(products)
		}
		if transport != 0 {
			fields["transportTotal"] = formatDataNumber(transport)
		}
		if services != 0 {
			fields["servicesTotal"] = formatDataNumber(services)
		}
	}
}

func financialDocumentModule(module string) bool {
	switch module {
	case "rfq", "quotation", "proforma_invoice", "sales_invoice", "sales_order", "purchase_order", "commercial_invoice", "packing_list", "bill_of_lading", "delivery_note", "handover_sheet", "warehouse_document", "shipping_document", "shipment", "receipt_voucher", "payment_voucher":
		return true
	default:
		return false
	}
}

func formatDataNumber(v float64) string {
	if v == 0 {
		return "0.00"
	}
	return strconv.FormatFloat(v, 'f', 2, 64)
}

func startsBusinessFlow(module string) bool {
	switch module {
	case "rfq", "quotation", "proforma_invoice", "sales_invoice", "sales_order", "purchase_order", "commercial_invoice", "packing_list", "shipment", "bill_of_lading", "delivery_note":
		return true
	default:
		return false
	}
}

func defaultStatus(module string, fields map[string]string) string {
	switch module {
	case "customer", "supplier", "contract", "quotation", "proforma_invoice", "sales_invoice", "commercial_invoice", "payment_voucher", "receipt_voucher", "bill_of_lading", "delivery_note":
		return "Pending Approval"
	case "shipment":
		return "Booked"
	case "task":
		return "Open"
	default:
		return "Draft"
	}
}

func (app *App) nextNumberLocked(module string, fields map[string]string) string {
	if app.state.Serial == nil {
		app.state.Serial = map[string]int{}
	}
	app.state.Serial[module]++
	seq := app.state.Serial[module]
	moduleCode := prefixes[module]
	if moduleCode == "" {
		moduleCode = strings.ToUpper(module[:min(3, len(module))])
	}
	prefix := firstNonEmpty(app.state.Company.Prefix, "ZE")
	padding := intSetting(app.state.Settings, "serialPadding", 4)
	if padding < 1 || padding > 8 {
		padding = 4
	}
	year := intSetting(app.state.Settings, "serialYear", time.Now().Year())
	customerCode := codeFromFields(fields)
	format := strings.TrimSpace(app.state.Settings["serialFormat"])
	if format == "" {
		if startsBusinessFlow(module) || module == "letterhead" || module == "receipt_voucher" || module == "payment_voucher" || module == "commercial_invoice" || module == "packing_list" {
			format = "{MODULE}-{YEAR}-{JOBSEQ}"
		} else {
			format = "{MODULE}-{YEAR}-{SEQ}"
		}
	}
	seqText := fmt.Sprintf("%0*d", padding, seq)
	jobSeq := firstNonEmpty(fields["jobSequence"], sequenceFromJobRef(fields["jobRef"]), seqText)
	if jobSeq == "" {
		jobSeq = seqText
	}
	rev := firstNonEmpty(app.state.Settings["serialRevision"], "R0")
	replacements := map[string]string{
		"{PREFIX}":   prefix,
		"{COMPANY}":  prefix,
		"{CUSTOMER}": customerCode,
		"{PARTY}":    customerCode,
		"{YEAR}":     strconv.Itoa(year),
		"{YY}":       fmt.Sprintf("%02d", year%100),
		"{SEQ}":      seqText,
		"{JOBSEQ}":   jobSeq,
		"{JOB}":      jobSeq,
		"{MODULE}":   moduleCode,
		"{DOC}":      moduleCode,
		"{REV}":      rev,
	}
	out := format
	for k, v := range replacements {
		out = strings.ReplaceAll(out, k, v)
	}
	out = strings.ToUpper(strings.TrimSpace(out))
	out = regexp.MustCompile(`[^A-Z0-9\-_/]+`).ReplaceAllString(out, "-")
	out = regexp.MustCompile(`[-_]{2,}`).ReplaceAllString(out, "-")
	out = strings.Trim(out, "-_/ ")
	if out == "" {
		out = fmt.Sprintf("%s-%d-%s", moduleCode, year, seqText)
	}
	return out
}

func intSetting(settings map[string]string, key string, def int) int {
	if settings == nil {
		return def
	}
	v, err := strconv.Atoi(strings.TrimSpace(settings[key]))
	if err != nil || v == 0 {
		return def
	}
	return v
}

func codeFromFields(fields map[string]string) string {
	if fields == nil {
		return "GEN"
	}
	raw := firstNonEmpty(fields["customerCode"], fields["partyCode"], fields["baseSerial"], fields["customer"], fields["buyer"], fields["consignee"], fields["shipper"], fields["supplier"], fields["name"], fields["partyName"], fields["to"])
	if raw == "" {
		return "GEN"
	}
	cleanRaw := strings.ToUpper(regexp.MustCompile(`[^A-Za-z0-9]+`).ReplaceAllString(raw, ""))
	if cleanRaw != "" && len(cleanRaw) >= 2 && len(cleanRaw) <= 10 && !strings.ContainsAny(raw, " -_.,/") {
		if len(cleanRaw) > 8 {
			cleanRaw = cleanRaw[:8]
		}
		return cleanRaw
	}
	if strings.Contains(raw, "-") {
		parts := strings.Split(raw, "-")
		if len(parts) > 1 && len(parts[1]) >= 2 && len(parts[1]) <= 10 {
			raw = parts[1]
		}
	}
	words := strings.FieldsFunc(raw, func(r rune) bool {
		return !(r >= 'A' && r <= 'Z') && !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9')
	})
	stop := map[string]bool{"and": true, "the": true, "of": true, "co": true, "company": true, "llc": true, "l.l.c": true, "trading": true, "trade": true, "general": true, "mr": true, "mrs": true, "ms": true}
	code := ""
	for _, w := range words {
		lw := strings.ToLower(w)
		if stop[lw] || w == "" {
			continue
		}
		if len(w) <= 3 && regexp.MustCompile(`^[A-Za-z0-9]+$`).MatchString(w) && len(code) == 0 {
			code += strings.ToUpper(w)
		} else {
			code += strings.ToUpper(w[:1])
		}
		if len(code) >= 8 {
			break
		}
	}
	if code == "" {
		code = strings.ToUpper(regexp.MustCompile(`[^A-Za-z0-9]+`).ReplaceAllString(raw, ""))
	}
	if len(code) > 8 {
		code = code[:8]
	}
	if code == "" {
		code = "GEN"
	}
	return code
}

func (app *App) nextJobRefLocked() string {
	if app.state.Serial == nil {
		app.state.Serial = map[string]int{}
	}
	app.state.Serial["job"]++
	prefix := firstNonEmpty(app.state.Settings["jobPrefix"], "ZEN")
	year := intSetting(app.state.Settings, "serialYear", time.Now().Year())
	padding := intSetting(app.state.Settings, "jobPadding", 5)
	if padding < 3 || padding > 8 {
		padding = 5
	}
	return fmt.Sprintf("%s-%d-%0*d", strings.ToUpper(prefix), year, padding, app.state.Serial["job"])
}

func sequenceFromJobRef(jobRef string) string {
	jobRef = strings.TrimSpace(jobRef)
	if jobRef == "" {
		return ""
	}
	re := regexp.MustCompile(`(\d+)$`)
	m := re.FindStringSubmatch(jobRef)
	if len(m) == 2 {
		return m[1]
	}
	return ""
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func copyMap(m map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range m {
		out[k] = v
	}
	return out
}

func (app *App) handleRecordUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorJSON(w, 405, "POST required")
		return
	}
	user := app.sessionUser(r)
	var req struct {
		ID     string            `json:"id"`
		Number string            `json:"number"`
		Fields map[string]string `json:"fields"`
		Status string            `json:"status"`
		Reason string            `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorJSON(w, 400, "invalid json")
		return
	}
	if err := validateEmailFields(req.Fields); err != nil {
		errorJSON(w, 400, err.Error())
		return
	}
	app.mu.Lock()
	defer app.mu.Unlock()
	rec := app.findRecordLocked(req.ID)
	if rec == nil {
		errorJSON(w, 404, "record not found")
		return
	}
	prevFields := copyMap(rec.Fields)
	prevStatus := rec.Status
	if strings.TrimSpace(req.Number) != "" && strings.TrimSpace(req.Number) != rec.Number {
		newNumber := strings.ToUpper(strings.TrimSpace(req.Number))
		for _, existing := range app.state.Records {
			if existing.ID != rec.ID && strings.EqualFold(existing.Number, newNumber) {
				errorJSON(w, 409, "document number already exists")
				return
			}
		}
		rec.Number = newNumber
	}
	if req.Fields != nil {
		if rec.Fields == nil {
			rec.Fields = map[string]string{}
		}
		for k, v := range req.Fields {
			rec.Fields[k] = v
		}
	}
	if req.Status != "" {
		rec.Status = req.Status
	}
	app.normalizeRecordFieldsLocked(rec.Module, rec.Fields)
	rec.Version++
	rec.UpdatedAt = time.Now().Format(time.RFC3339)
	rec.UpdatedBy = user
	rec.History = append(rec.History, Change{Version: rec.Version - 1, Time: rec.UpdatedAt, User: user, Reason: req.Reason, PreviousStatus: prevStatus, PreviousFields: prevFields})
	app.addAuditLocked(user, "UPDATE", rec.Module, rec.ID, rec.Number, safeReason(req.Reason, "Record updated"))
	app.afterRecordChangeLocked(user, rec)
	if err := app.saveLocked(); err != nil {
		errorJSON(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "record": rec})
}

func (app *App) handleRecordStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorJSON(w, 405, "POST required")
		return
	}
	user := app.sessionUser(r)
	var req struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorJSON(w, 400, "invalid json")
		return
	}
	if strings.TrimSpace(req.Status) == "" {
		errorJSON(w, 400, "status required")
		return
	}
	app.mu.Lock()
	defer app.mu.Unlock()
	rec := app.findRecordLocked(req.ID)
	if rec == nil {
		errorJSON(w, 404, "record not found")
		return
	}
	prevFields := copyMap(rec.Fields)
	prevStatus := rec.Status
	rec.Status = req.Status
	rec.Version++
	rec.UpdatedAt = time.Now().Format(time.RFC3339)
	rec.UpdatedBy = user
	rec.History = append(rec.History, Change{Version: rec.Version - 1, Time: rec.UpdatedAt, User: user, Reason: req.Reason, PreviousStatus: prevStatus, PreviousFields: prevFields})
	action := "STATUS_CHANGE"
	if strings.EqualFold(req.Status, "Approved") {
		action = "APPROVE"
	}
	if strings.EqualFold(req.Status, "Rejected") {
		action = "REJECT"
	}
	if strings.EqualFold(req.Status, "Cancelled") {
		action = "CANCEL"
	}
	app.addAuditLocked(user, action, rec.Module, rec.ID, rec.Number, safeReason(req.Reason, "Status changed to "+req.Status))
	app.afterRecordChangeLocked(user, rec)
	if err := app.saveLocked(); err != nil {
		errorJSON(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "record": rec})
}

func (app *App) afterRecordChangeLocked(user string, rec *Record) {
	status := strings.ToLower(strings.TrimSpace(rec.Status))
	approved := status == "accepted" || status == "approved"
	if rec.Module == "quotation" && approved {
		pi := app.ensureConvertedLocked(user, rec, "proforma_invoice", "Quotation accepted; proforma invoice draft created")
		if pi != nil {
			si := app.ensureConvertedLocked(user, pi, "sales_invoice", "Proforma invoice linked from quotation; sales invoice draft created")
			if si != nil {
				ci := app.ensureConvertedLocked(user, si, "commercial_invoice", "Sales invoice linked from quotation; commercial invoice draft created")
				if ci != nil {
					app.ensureConvertedLocked(user, ci, "packing_list", "Commercial invoice linked from quotation; packing list draft created")
				}
			}
		}
	}
	if rec.Module == "proforma_invoice" && approved {
		si := app.ensureConvertedLocked(user, rec, "sales_invoice", "Proforma invoice accepted/approved; sales invoice draft created")
		if si != nil {
			ci := app.ensureConvertedLocked(user, si, "commercial_invoice", "Sales invoice linked from proforma; commercial invoice draft created")
			if ci != nil {
				app.ensureConvertedLocked(user, ci, "packing_list", "Commercial invoice linked from proforma; packing list draft created")
			}
		}
	}
	if rec.Module == "sales_invoice" && approved {
		ci := app.ensureConvertedLocked(user, rec, "commercial_invoice", "Sales invoice accepted/approved; commercial invoice draft created")
		if ci != nil {
			app.ensureConvertedLocked(user, ci, "packing_list", "Commercial invoice linked from sales invoice; packing list draft created")
		}
	}
	if rec.Module == "commercial_invoice" && approved {
		app.ensureConvertedLocked(user, rec, "packing_list", "Commercial invoice accepted/approved; packing list draft created")
	}
	if rec.Module == "delivery_note" && strings.EqualFold(rec.Status, "Delivered") {
		app.ensureConvertedLocked(user, rec, "sales_invoice", "Delivery note marked Delivered; sales invoice draft created")
	}
}

func (app *App) ensureConvertedLocked(user string, source *Record, target string, reason string) *Record {
	if source == nil || target == "" || source.JobRef == "" {
		return nil
	}
	for i := range app.state.Records {
		existing := &app.state.Records[i]
		if existing.Module == target && existing.JobRef == source.JobRef {
			return existing
		}
	}
	fields := copyCommonFields(source.Fields)
	for k, v := range source.Fields {
		lk := strings.ToLower(k)
		if _, ok := fields[k]; !ok && (strings.Contains(lk, "customer") || strings.Contains(lk, "supplier") || strings.Contains(lk, "party") || strings.Contains(lk, "address")) {
			fields[k] = v
		}
	}
	fields["sourceDocument"] = source.Number
	if fields["jobSequence"] == "" {
		fields["jobSequence"] = sequenceFromJobRef(source.JobRef)
	}
	links := copyMap(source.Links)
	links[source.Module] = source.ID
	created := app.createRecordLocked(user, target, fields, defaultStatus(target, fields), source.JobRef, links)
	app.addAuditLocked(user, "AUTO_WORKFLOW", target, created.ID, created.Number, reason)
	for i := range app.state.Records {
		if app.state.Records[i].ID == created.ID {
			return &app.state.Records[i]
		}
	}
	return nil
}

func copyCommonFields(fields map[string]string) map[string]string {
	common := []string{"jobRef", "jobSequence", "transactionType", "customer", "customerCode", "customerAddress", "customerEmail", "customerPhone", "customerTaxNumber", "contactPerson", "taxNumber", "creditLimit", "outstandingBalance", "supplier", "amount", "total", "subtotal", "discount", "tax", "taxRate", "cost", "saleAmount", "estimatedCost", "currency", "productDescription", "serviceDescription", "quantity", "weight", "unit", "unitPrice", "hsCode", "route", "loadingLocation", "deliveryLocation", "containerNumber", "sealNumber", "truckNumber", "driverName", "driverMobile", "vesselVoyage", "pol", "pod", "fpod", "incoterm", "itemsJSON", "notes", "terms", "paymentTerms", "date", "invoiceDate", "dueDate", "validUntil", "verificationCode", "bankDetailsOption", "bankAccountId", "bankDetails"}
	out := map[string]string{}
	for _, k := range common {
		if v := fields[k]; v != "" {
			out[k] = v
		}
	}
	return out
}

func safeReason(reason, fallback string) string {
	if strings.TrimSpace(reason) != "" {
		return strings.TrimSpace(reason)
	}
	return fallback
}

func (app *App) handleRecordConvert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorJSON(w, 405, "POST required")
		return
	}
	user := app.sessionUser(r)
	var req struct {
		SourceID     string            `json:"sourceId"`
		TargetModule string            `json:"targetModule"`
		ExtraFields  map[string]string `json:"extraFields"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorJSON(w, 400, "invalid json")
		return
	}
	target := cleanModule(req.TargetModule)
	if target == "" {
		errorJSON(w, 400, "target module required")
		return
	}
	app.mu.Lock()
	defer app.mu.Unlock()
	source := app.findRecordLocked(req.SourceID)
	if source == nil {
		errorJSON(w, 404, "source not found")
		return
	}
	if source.JobRef != "" {
		for i := range app.state.Records {
			if app.state.Records[i].Module == target && app.state.Records[i].JobRef == source.JobRef {
				writeJSON(w, 200, map[string]any{"ok": true, "record": app.state.Records[i], "existing": true})
				return
			}
		}
	}
	fields := copyCommonFields(source.Fields)
	for k, v := range source.Fields {
		if _, ok := fields[k]; !ok && (strings.Contains(strings.ToLower(k), "customer") || strings.Contains(strings.ToLower(k), "supplier")) {
			fields[k] = v
		}
	}
	for k, v := range req.ExtraFields {
		fields[k] = v
	}
	fields["sourceDocument"] = source.Number
	links := copyMap(source.Links)
	links[source.Module] = source.ID
	rec := app.createRecordLocked(user, target, fields, defaultStatus(target, fields), source.JobRef, links)
	app.addAuditLocked(user, "CONVERT", target, rec.ID, rec.Number, "Converted from "+source.Module+" "+source.Number)
	if err := app.saveLocked(); err != nil {
		errorJSON(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "record": rec})
}

func (app *App) findRecordLocked(id string) *Record {
	for i := range app.state.Records {
		if app.state.Records[i].ID == id {
			return &app.state.Records[i]
		}
	}
	return nil
}

func (app *App) addAuditLocked(user, action, module, recordID, number, details string) {
	app.state.Audit = append(app.state.Audit, AuditEntry{ID: genID(), Time: time.Now().Format(time.RFC3339), User: user, Action: action, Module: module, RecordID: recordID, Number: number, Details: details})
	if len(app.state.Audit) > 3000 {
		app.state.Audit = app.state.Audit[len(app.state.Audit)-3000:]
	}
}

func passwordPolicyOK(password string) (bool, string) {
	if len(password) < 10 {
		return false, "password must be at least 10 characters"
	}
	hasUpper, hasLower, hasDigit, hasSymbol := false, false, false, false
	for _, r := range password {
		switch {
		case r >= 'A' && r <= 'Z':
			hasUpper = true
		case r >= 'a' && r <= 'z':
			hasLower = true
		case r >= '0' && r <= '9':
			hasDigit = true
		default:
			hasSymbol = true
		}
	}
	if !hasUpper || !hasLower || !hasDigit || !hasSymbol {
		return false, "password must include uppercase, lowercase, number and symbol"
	}
	return true, ""
}

func (app *App) handleUserCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorJSON(w, 405, "POST required")
		return
	}
	current := app.sessionUser(r)
	var req struct {
		Username    string `json:"username"`
		DisplayName string `json:"displayName"`
		Role        string `json:"role"`
		Department  string `json:"department"`
		Password    string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorJSON(w, 400, "invalid json")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		errorJSON(w, 400, "username and password required")
		return
	}
	if ok, msg := passwordPolicyOK(req.Password); !ok {
		errorJSON(w, 400, msg)
		return
	}
	app.mu.Lock()
	defer app.mu.Unlock()
	for _, u := range app.state.Users {
		if strings.EqualFold(u.Username, req.Username) {
			errorJSON(w, 409, "username already exists")
			return
		}
	}
	if req.DisplayName == "" {
		req.DisplayName = req.Username
	}
	if req.Role == "" {
		req.Role = "Employee"
	}
	user := User{ID: genID(), Username: req.Username, DisplayName: req.DisplayName, Role: req.Role, Department: req.Department, PasswordHash: hashPassword(req.Username, req.Password), Active: true, CreatedAt: time.Now().Format(time.RFC3339)}
	app.state.Users = append(app.state.Users, user)
	app.addAuditLocked(current, "CREATE_USER", "user", user.ID, user.Username, "User account created")
	if err := app.saveLocked(); err != nil {
		errorJSON(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "user": publicUser(user)})
}

func (app *App) handlePasswordChange(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorJSON(w, 405, "POST required")
		return
	}
	current := app.sessionUser(r)
	var req struct {
		OldPassword string `json:"oldPassword"`
		NewPassword string `json:"newPassword"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorJSON(w, 400, "invalid json")
		return
	}
	if ok, msg := passwordPolicyOK(req.NewPassword); !ok {
		errorJSON(w, 400, msg)
		return
	}
	app.mu.Lock()
	defer app.mu.Unlock()
	for i := range app.state.Users {
		if strings.EqualFold(app.state.Users[i].Username, current) {
			if app.state.Users[i].PasswordHash != hashPassword(app.state.Users[i].Username, req.OldPassword) {
				errorJSON(w, 401, "old password incorrect")
				return
			}
			app.state.Users[i].PasswordHash = hashPassword(app.state.Users[i].Username, req.NewPassword)
			app.addAuditLocked(current, "CHANGE_PASSWORD", "user", app.state.Users[i].ID, app.state.Users[i].Username, "Password changed")
			app.state.Settings["firstRun"] = "false"
			if err := app.saveLocked(); err != nil {
				errorJSON(w, 500, err.Error())
				return
			}
			writeJSON(w, 200, map[string]any{"ok": true})
			return
		}
	}
	errorJSON(w, 404, "user not found")
}

func (app *App) handleExport(w http.ResponseWriter, r *http.Request) {
	// Backward-compatible JSON export for old backups.
	if !app.userIsAdmin(app.sessionUser(r)) {
		errorJSON(w, http.StatusForbidden, "admin permission required")
		return
	}
	app.mu.Lock()
	b, err := json.MarshalIndent(app.state, "", "  ")
	app.mu.Unlock()
	if err != nil {
		errorJSON(w, 500, err.Error())
		return
	}
	name := "zenith_erp_backup_" + time.Now().Format("20060102_150405") + ".json"
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename="+name)
	_, _ = w.Write(b)
}

func (app *App) handleFullBackup(w http.ResponseWriter, r *http.Request) {
	user := app.sessionUser(r)
	if !app.userIsAdmin(user) {
		errorJSON(w, http.StatusForbidden, "admin permission required")
		return
	}
	b, name, err := app.makeFullBackupBytes("manual-download")
	if err != nil {
		errorJSON(w, 500, err.Error())
		return
	}
	app.mu.Lock()
	app.addAuditLocked(user, "DOWNLOAD_FULL_BACKUP", "system", "", name, "Full backup downloaded")
	_ = app.saveLocked()
	app.mu.Unlock()
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename="+name)
	_, _ = w.Write(b)
}

func (app *App) handleBackupRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		errorJSON(w, 405, "GET or POST required")
		return
	}
	user := app.sessionUser(r)
	if !app.userIsAdmin(user) {
		errorJSON(w, http.StatusForbidden, "admin permission required")
		return
	}
	path, copied, err := app.createLocalBackup("manual-run")
	if err != nil {
		errorJSON(w, 500, err.Error())
		return
	}
	app.mu.Lock()
	app.addAuditLocked(user, "CREATE_FULL_BACKUP", "system", "", filepath.Base(path), "Full backup created")
	_ = app.saveLocked()
	app.mu.Unlock()
	writeJSON(w, 200, map[string]any{"ok": true, "path": path, "cloudCopy": copied})
}

func (app *App) handleBackupSettings(w http.ResponseWriter, r *http.Request) {
	user := app.sessionUser(r)
	if !app.userIsAdmin(user) {
		errorJSON(w, http.StatusForbidden, "admin permission required")
		return
	}
	if r.Method == http.MethodGet {
		app.mu.Lock()
		settings := map[string]string{}
		for _, k := range []string{"autoBackupFrequency", "backupLocalPath", "backupCloudPath", "backupRetentionCount", "lastAutoBackupAt"} {
			settings[k] = app.state.Settings[k]
		}
		if settings["backupLocalPath"] == "" {
			settings["backupLocalPath"] = app.backupDir
		}
		app.mu.Unlock()
		writeJSON(w, 200, map[string]any{"ok": true, "settings": settings})
		return
	}
	if r.Method != http.MethodPost {
		errorJSON(w, 405, "POST required")
		return
	}
	var req map[string]string
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorJSON(w, 400, "invalid json")
		return
	}
	freq := firstNonEmpty(req["autoBackupFrequency"], req["backupSchedule"], req["schedule"], "Off")
	switch strings.ToLower(freq) {
	case "off", "daily", "weekly", "monthly":
	default:
		errorJSON(w, 400, "schedule must be Off, Daily, Weekly, or Monthly")
		return
	}
	app.mu.Lock()
	if app.state.Settings == nil {
		app.state.Settings = map[string]string{}
	}
	backupLocalPath := firstNonEmpty(req["backupLocalPath"], req["localBackupDir"], app.state.Settings["backupLocalPath"], app.backupDir)
	backupCloudPath := firstNonEmpty(req["backupCloudPath"], req["cloudBackupTarget"], app.state.Settings["backupCloudPath"])
	app.state.Settings["autoBackupFrequency"] = titleCase(freq)
	app.state.Settings["backupSchedule"] = titleCase(freq)
	app.state.Settings["backupLocalPath"] = strings.TrimSpace(backupLocalPath)
	app.state.Settings["localBackupDir"] = strings.TrimSpace(backupLocalPath)
	app.state.Settings["backupCloudPath"] = strings.TrimSpace(backupCloudPath)
	app.state.Settings["cloudBackupTarget"] = strings.TrimSpace(backupCloudPath)
	app.state.Settings["backupRetentionCount"] = firstNonEmpty(req["backupRetentionCount"], app.state.Settings["backupRetentionCount"], "10")
	app.addAuditLocked(user, "UPDATE_BACKUP_SETTINGS", "system", "", "", "Backup schedule/settings updated")
	err := app.saveLocked()
	app.mu.Unlock()
	if err != nil {
		errorJSON(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (app *App) handleRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorJSON(w, 405, "POST required")
		return
	}
	user := app.sessionUser(r)
	if !app.userIsAdmin(user) {
		errorJSON(w, http.StatusForbidden, "admin permission required")
		return
	}
	b, err := io.ReadAll(io.LimitReader(r.Body, 500<<20))
	if err != nil {
		errorJSON(w, 400, "invalid backup file")
		return
	}
	restored, uploads, err := app.parseBackupFile(b)
	if err != nil {
		errorJSON(w, 400, err.Error())
		return
	}
	safetyPath, _, err := app.createLocalBackup("safety-before-restore")
	if err != nil {
		errorJSON(w, 500, "safety backup failed before restore: "+err.Error())
		return
	}
	app.mu.Lock()
	defer app.mu.Unlock()
	old := app.state
	app.state = restored
	app.ensureStateDefaultsLocked()
	if err := app.saveLocked(); err != nil {
		app.state = old
		errorJSON(w, 500, err.Error())
		return
	}
	if uploads != nil {
		_ = os.RemoveAll(app.uploadDir)
		_ = os.MkdirAll(app.uploadDir, 0755)
		for rel, data := range uploads {
			clean := filepath.Clean(rel)
			if strings.Contains(clean, "..") || filepath.IsAbs(clean) {
				continue
			}
			out := filepath.Join(app.uploadDir, clean)
			_ = os.MkdirAll(filepath.Dir(out), 0755)
			_ = os.WriteFile(out, data, 0644)
		}
	}
	app.addAuditLocked(user, "RESTORE_BACKUP", "system", "", "", "Full backup restored after admin confirmation. Safety backup: "+safetyPath)
	_ = app.saveLocked()
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (app *App) parseBackupFile(b []byte) (State, map[string][]byte, error) {
	var restored State
	uploads := map[string][]byte(nil)
	if len(b) >= 4 && string(b[:2]) == "PK" {
		zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
		if err != nil {
			return restored, nil, fmt.Errorf("invalid backup zip")
		}
		var stateBytes []byte
		uploads = map[string][]byte{}
		for _, f := range zr.File {
			name := strings.TrimLeft(f.Name, "/\\")
			if f.FileInfo().IsDir() {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				continue
			}
			data, _ := io.ReadAll(io.LimitReader(rc, 100<<20))
			_ = rc.Close()
			switch name {
			case "erp_data.json", "state.json", "backup_state.json":
				stateBytes = data
			default:
				if strings.HasPrefix(name, "uploads/") {
					uploads[strings.TrimPrefix(name, "uploads/")] = data
				}
			}
		}
		if len(stateBytes) == 0 {
			return restored, nil, fmt.Errorf("backup zip does not contain erp_data.json")
		}
		if err := json.Unmarshal(stateBytes, &restored); err != nil || len(restored.Users) == 0 || restored.Records == nil {
			return restored, nil, fmt.Errorf("invalid ERP database inside backup zip")
		}
		return restored, uploads, nil
	}
	if err := json.Unmarshal(b, &restored); err != nil || len(restored.Users) == 0 || restored.Records == nil {
		if looksLikeLegacyBackup(b) {
			restored = defaultState()
			if restored.Settings == nil {
				restored.Settings = map[string]string{}
			}
			restored.Settings["restoredFromLegacyBackup"] = time.Now().Format(time.RFC3339)
			return restored, nil, nil
		}
		return restored, nil, fmt.Errorf("invalid backup json")
	}
	return restored, nil, nil
}

func (app *App) makeFullBackupBytes(reason string) ([]byte, string, error) {
	app.mu.Lock()
	stateCopy := app.state
	if stateCopy.Settings == nil {
		stateCopy.Settings = map[string]string{}
	}
	stateCopy.Settings["backupCreatedAt"] = time.Now().Format(time.RFC3339)
	stateCopy.Settings["backupReason"] = reason
	stateCopy.Settings["backupIncludes"] = "database, records, users, settings, serials, audit, uploads, templates, QR verification records"
	company := stateCopy.Company
	records := append([]Record(nil), stateCopy.Records...)
	uploadDir := app.uploadDir
	app.mu.Unlock()

	stateJSON, err := json.MarshalIndent(stateCopy, "", "  ")
	if err != nil {
		return nil, "", err
	}
	qrRecords := []map[string]string{}
	for _, rec := range records {
		if rec.Number == "" {
			continue
		}
		st, valid := verificationStatus(&rec)
		qrRecords = append(qrRecords, map[string]string{"number": rec.Number, "module": rec.Module, "jobRef": rec.JobRef, "status": firstNonEmpty(rec.Status, st), "verificationStatus": st, "valid": strconv.FormatBool(valid), "url": verificationURL(company, rec, rec.Number)})
	}
	qrJSON, _ := json.MarshalIndent(qrRecords, "", "  ")
	manifest := map[string]any{"app": appName, "version": appVersion, "createdAt": time.Now().Format(time.RFC3339), "reason": reason, "includes": []string{"erp_data.json", "uploads/", "qr_records.json", "templates/"}}
	manifestJSON, _ := json.MarshalIndent(manifest, "", "  ")

	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	addBytes := func(name string, data []byte) error {
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	}
	if err := addBytes("backup_manifest.json", manifestJSON); err != nil {
		return nil, "", err
	}
	if err := addBytes("erp_data.json", stateJSON); err != nil {
		return nil, "", err
	}
	if err := addBytes("qr_records.json", qrJSON); err != nil {
		return nil, "", err
	}
	if err := addBytes("templates/README.txt", []byte("Document templates are stored in the ERP database/company settings and regenerated by the document engine.\n")); err != nil {
		return nil, "", err
	}
	_ = filepath.Walk(uploadDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(uploadDir, path)
		if err != nil {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		_ = addBytes("uploads/"+filepath.ToSlash(rel), data)
		return nil
	})
	if err := zw.Close(); err != nil {
		return nil, "", err
	}
	name := "zenith_erp_full_backup_" + time.Now().Format("20060102_150405") + ".zip"
	return buf.Bytes(), name, nil
}

func (app *App) createLocalBackup(reason string) (string, string, error) {
	b, name, err := app.makeFullBackupBytes(reason)
	if err != nil {
		return "", "", err
	}
	app.mu.Lock()
	backupDir := firstNonEmpty(app.state.Settings["backupLocalPath"], app.backupDir)
	cloudPath := strings.TrimSpace(app.state.Settings["backupCloudPath"])
	app.mu.Unlock()
	if backupDir == "" {
		backupDir = app.backupDir
	}
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return "", "", err
	}
	path := filepath.Join(backupDir, name)
	if err := os.WriteFile(path, b, 0644); err != nil {
		return "", "", err
	}
	copied := ""
	if cloudPath != "" {
		if err := os.MkdirAll(cloudPath, 0755); err == nil {
			copyPath := filepath.Join(cloudPath, name)
			if err := os.WriteFile(copyPath, b, 0644); err == nil {
				copied = copyPath
			}
		}
	}
	app.cleanupOldBackups(backupDir)
	return path, copied, nil
}

func (app *App) cleanupOldBackups(dir string) {
	app.mu.Lock()
	keep, _ := strconv.Atoi(firstNonEmpty(app.state.Settings["backupRetentionCount"], "10"))
	app.mu.Unlock()
	if keep < 1 {
		keep = 10
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	type fileInfo struct {
		path string
		mod  time.Time
	}
	files := []fileInfo{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "zenith_erp_full_backup_") || !strings.HasSuffix(e.Name(), ".zip") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, fileInfo{filepath.Join(dir, e.Name()), info.ModTime()})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mod.After(files[j].mod) })
	for i := keep; i < len(files); i++ {
		_ = os.Remove(files[i].path)
	}
}

func (app *App) startBackupScheduler() {
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			app.maybeAutoBackup()
		}
	}()
}

func (app *App) maybeAutoBackup() {
	app.mu.Lock()
	freq := strings.ToLower(strings.TrimSpace(firstNonEmpty(app.state.Settings["autoBackupFrequency"], app.state.Settings["backupSchedule"])))
	lastRaw := strings.TrimSpace(app.state.Settings["lastAutoBackupAt"])
	app.mu.Unlock()
	var interval time.Duration
	switch freq {
	case "daily":
		interval = 24 * time.Hour
	case "weekly":
		interval = 7 * 24 * time.Hour
	case "monthly":
		interval = 30 * 24 * time.Hour
	default:
		return
	}
	if lastRaw != "" {
		if last, err := time.Parse(time.RFC3339, lastRaw); err == nil && time.Since(last) < interval {
			return
		}
	}
	path, copied, err := app.createLocalBackup("scheduled-" + freq)
	app.mu.Lock()
	defer app.mu.Unlock()
	if err != nil {
		app.addAuditLocked("system", "AUTO_BACKUP_FAILED", "system", "", "", err.Error())
		_ = app.saveLocked()
		return
	}
	app.state.Settings["lastAutoBackupAt"] = time.Now().Format(time.RFC3339)
	detail := "Scheduled backup created: " + path
	if copied != "" {
		detail += " | server/cloud copy: " + copied
	}
	app.addAuditLocked("system", "AUTO_BACKUP", "system", "", filepath.Base(path), detail)
	_ = app.saveLocked()
}

func (app *App) userIsAdmin(username string) bool {
	username = strings.TrimSpace(username)
	if username == "" {
		return false
	}
	app.mu.Lock()
	defer app.mu.Unlock()
	for _, u := range app.state.Users {
		if strings.EqualFold(u.Username, username) && u.Active {
			role := strings.ToLower(u.Role + " " + u.Department + " " + u.Username)
			return strings.Contains(role, "owner") || strings.Contains(role, "admin")
		}
	}
	return false
}

func titleCase(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	l := strings.ToLower(s)
	return strings.ToUpper(l[:1]) + l[1:]
}

func looksLikeLegacyBackup(b []byte) bool {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return false
	}
	_, hasCompany := raw["company"]
	_, hasCustomers := raw["customers"]
	_, hasDocuments := raw["documents"]
	return hasCompany && (hasCustomers || hasDocuments)
}

func (app *App) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorJSON(w, 405, "POST required")
		return
	}
	user := app.sessionUser(r)
	if err := r.ParseMultipartForm(60 << 20); err != nil {
		errorJSON(w, 400, "upload too large or invalid")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		errorJSON(w, 400, "file required")
		return
	}
	defer file.Close()
	saved, size, err := app.saveUploadedFile(file, header)
	if err != nil {
		errorJSON(w, 500, err.Error())
		return
	}
	fields := map[string]string{"fileName": header.Filename, "savedPath": saved, "sizeBytes": fmt.Sprintf("%d", size), "recordId": r.FormValue("recordId"), "documentType": r.FormValue("documentType"), "notes": r.FormValue("notes")}
	app.mu.Lock()
	defer app.mu.Unlock()
	rec := app.createRecordLocked(user, "document_upload", fields, "Uploaded", "", nil)
	app.addAuditLocked(user, "UPLOAD_FILE", "document_upload", rec.ID, rec.Number, "File uploaded: "+header.Filename)
	if err := app.saveLocked(); err != nil {
		errorJSON(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "record": rec})
}

func (app *App) saveUploadedFile(file multipart.File, header *multipart.FileHeader) (string, int64, error) {
	name := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, header.Filename)
	if name == "" {
		name = "upload.bin"
	}
	dstPath := filepath.Join(app.uploadDir, time.Now().Format("20060102_150405_")+genID()+"_"+name)
	dst, err := os.Create(dstPath)
	if err != nil {
		return "", 0, err
	}
	defer dst.Close()
	n, err := io.Copy(dst, file)
	return dstPath, n, err
}

func (app *App) handleAIExtract(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorJSON(w, 405, "POST required")
		return
	}
	var req struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 5<<20)).Decode(&req); err != nil {
		errorJSON(w, 400, "invalid json")
		return
	}
	text := req.Text
	result := map[string]string{}
	patterns := map[string]string{
		"containerNumber": `\b[A-Z]{4}\d{7}\b`,
		"sealNumber":      `(?i)\bseal\s*(?:no|number|#)?\s*[:\-]?\s*([A-Z0-9\-]{4,})`,
		"blNumber":        `(?i)\bB\/?L\s*(?:no|number|#)?\s*[:\-]?\s*([A-Z0-9\-\/]{4,})`,
		"invoiceNumber":   `(?i)\binvoice\s*(?:no|number|#)?\s*[:\-]?\s*([A-Z0-9\-\/]{3,})`,
		"date":            `\b\d{4}-\d{2}-\d{2}\b|\b\d{1,2}[\-/]\d{1,2}[\-/]\d{2,4}\b`,
		"email":           `[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`,
		"currency":        `\b(USD|EUR|OMR|AED|GBP|CNY|AFN)\b`,
	}
	for key, pat := range patterns {
		re := regexp.MustCompile(pat)
		m := re.FindStringSubmatch(text)
		if len(m) > 1 {
			result[key] = strings.TrimSpace(m[1])
		} else if len(m) == 1 {
			result[key] = strings.TrimSpace(m[0])
		}
	}
	amountRe := regexp.MustCompile(`(?i)(?:amount|total|value|invoice total|grand total)\s*[:\-]?\s*(?:USD|EUR|OMR|AED|GBP|CNY|AFN)?\s*([0-9][0-9,]*(?:\.\d{1,2})?)`)
	if m := amountRe.FindStringSubmatch(text); len(m) > 1 {
		result["amount"] = strings.ReplaceAll(m[1], ",", "")
	}
	companyRe := regexp.MustCompile(`(?i)(?:shipper|consignee|customer|supplier)\s*[:\-]?\s*([^\n\r]{3,80})`)
	if m := companyRe.FindStringSubmatch(text); len(m) > 1 {
		result["partyName"] = strings.TrimSpace(m[1])
	}
	notes := []string{"Local extraction only: regex-based MVP helper, not a cloud AI approval engine."}
	writeJSON(w, 200, map[string]any{"ok": true, "extracted": result, "notes": notes})
}

func (app *App) handleDocument(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/doc/")
	id = strings.Trim(id, "/")
	app.mu.Lock()
	var rec *Record
	for i := range app.state.Records {
		if app.state.Records[i].ID == id || app.state.Records[i].Number == id {
			rec = &app.state.Records[i]
			break
		}
	}
	company := app.state.Company
	records := append([]Record(nil), app.state.Records...)
	app.mu.Unlock()
	if rec == nil {
		http.NotFound(w, r)
		return
	}
	html := renderDocHTML(company, *rec, records)
	secureHeaders(w)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(html))
}

func (app *App) handleDocumentFileExport(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/export/"), "/"), "/")
	if len(parts) < 2 {
		errorJSON(w, 400, "export path must be /export/{record_id}/{pdf|word|docx}")
		return
	}
	id := strings.TrimSpace(parts[0])
	format := strings.ToLower(strings.TrimSpace(parts[1]))
	app.mu.Lock()
	var rec *Record
	for i := range app.state.Records {
		if app.state.Records[i].ID == id || app.state.Records[i].Number == id {
			rec = &app.state.Records[i]
			break
		}
	}
	company := app.state.Company
	records := append([]Record(nil), app.state.Records...)
	app.mu.Unlock()
	if rec == nil {
		http.NotFound(w, r)
		return
	}
	baseName := sanitizeFileName(firstNonEmpty(rec.Number, rec.ID))
	switch format {
	case "pdf":
		html := renderDocHTML(company, *rec, records)
		pdf, err := renderPDFBytes(html)
		if err != nil || len(pdf) == 0 {
			errorJSON(w, 500, "PDF export needs Microsoft Edge/Chrome available to the ERP server: "+fmt.Sprint(err))
			return
		}
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", "attachment; filename="+baseName+".pdf")
		_, _ = w.Write(pdf)
	case "word", "docx":
		docx, err := renderDocxBytes(company, *rec, records)
		if err != nil {
			errorJSON(w, 500, err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
		w.Header().Set("Content-Disposition", "attachment; filename="+baseName+".docx")
		_, _ = w.Write(docx)
	default:
		errorJSON(w, 400, "format must be pdf or word")
	}
}

func (app *App) handleLetterhead(w http.ResponseWriter, r *http.Request) {
	app.mu.Lock()
	company := app.state.Company
	var rec Record
	for _, x := range app.state.Records {
		if x.Module == "letterhead" {
			rec = x
			break
		}
	}
	app.mu.Unlock()
	if rec.ID == "" {
		now := time.Now().Format(time.RFC3339)
		rec = Record{ID: "letterhead", Module: "letterhead", Number: "ZE-LHD-" + time.Now().Format("2006") + "-0001", Status: "Draft", CreatedAt: now, CreatedBy: "system", UpdatedAt: now, UpdatedBy: "system", Version: 1, Fields: map[string]string{"title": "Official Company Letterhead", "body": "Use this page for official company letters."}, Links: map[string]string{}, History: []Change{}}
	}
	html := renderDocHTML(company, rec, nil)
	secureHeaders(w)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(html))
}

func (app *App) handleVerify(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/verify/")
	id = strings.Trim(id, "/")
	if decoded, err := url.PathUnescape(id); err == nil {
		id = decoded
	}
	app.mu.Lock()
	company := app.state.Company
	var found *Record
	for i := range app.state.Records {
		rec := &app.state.Records[i]
		if rec.ID == id || rec.Number == id || strings.EqualFold(rec.Number, id) {
			copyRec := *rec
			found = &copyRec
			break
		}
	}
	app.mu.Unlock()

	status, valid := verificationStatus(found)
	if wantsJSON(r) {
		payload, httpStatus := app.verifyDocumentPayload(id)
		writeJSON(w, httpStatus, payload)
		return
	}
	secureHeaders(w)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(renderVerifyPage(company, found, id, status, valid)))
}

func (app *App) verifyDocumentPayload(number string) (map[string]any, int) {
	number = strings.TrimSpace(number)
	app.mu.Lock()
	company := app.state.Company
	var found *Record
	for i := range app.state.Records {
		r := &app.state.Records[i]
		if r.ID == number || r.Number == number || strings.EqualFold(r.Number, number) {
			copyRec := *r
			found = &copyRec
			break
		}
	}
	app.mu.Unlock()
	status, valid := verificationStatus(found)
	now := time.Now().Format(time.RFC3339)
	if found == nil {
		return map[string]any{"ok": false, "valid": false, "documentNumber": number, "currentStatus": "Not Found", "verificationTimestamp": now}, 404
	}
	issueDate := firstNonEmpty(found.Fields["invoiceDate"], found.Fields["date"], found.Fields["startDate"], found.CreatedAt)
	party := firstNonEmpty(found.Fields["customer"], found.Fields["customerName"], found.Fields["buyer"], found.Fields["partyName"], found.Fields["consignee"], found.Fields["shipper"], found.Fields["to"])
	return map[string]any{
		"ok":                    true,
		"valid":                 valid,
		"documentNumber":        found.Number,
		"documentType":          moduleLabel(found.Module),
		"module":                found.Module,
		"jobReference":          firstNonEmpty(found.JobRef, found.Fields["jobRef"]),
		"companyName":           firstNonEmpty(company.Name, company.LegalName, "ZENITH ECLIPSE CO"),
		"issueDate":             formatDocDate(issueDate),
		"customerOrPartyName":   party,
		"customerPartyName":     party,
		"currentStatus":         firstNonEmpty(status, found.Status, "Valid"),
		"erpStatus":             found.Status,
		"verificationURL":       verificationURL(company, *found, found.Number),
		"verificationTimestamp": now,
	}, 200
}

func (app *App) handleVerifyDocumentAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorJSON(w, 405, "GET required")
		return
	}
	number := strings.TrimPrefix(r.URL.Path, "/api/verify-document/")
	number = strings.Trim(number, "/")
	if decoded, err := url.PathUnescape(number); err == nil {
		number = decoded
	}
	if number == "" {
		errorJSON(w, 400, "document number required")
		return
	}
	app.mu.Lock()
	company := app.state.Company
	var found *Record
	for i := range app.state.Records {
		r := &app.state.Records[i]
		if r.ID == number || r.Number == number || strings.EqualFold(r.Number, number) {
			copyRec := *r
			found = &copyRec
			break
		}
	}
	app.mu.Unlock()
	status, valid := verificationStatus(found)
	now := time.Now().Format(time.RFC3339)
	if found == nil {
		writeJSON(w, 404, map[string]any{"ok": false, "valid": false, "documentNumber": number, "currentStatus": "Not Found", "verificationTimestamp": now})
		return
	}
	issueDate := firstNonEmpty(found.Fields["invoiceDate"], found.Fields["date"], found.Fields["startDate"], found.CreatedAt)
	party := firstNonEmpty(found.Fields["customer"], found.Fields["customerName"], found.Fields["buyer"], found.Fields["partyName"], found.Fields["consignee"], found.Fields["shipper"], found.Fields["to"])
	writeJSON(w, 200, map[string]any{
		"ok":                    true,
		"valid":                 valid,
		"documentNumber":        found.Number,
		"documentType":          moduleLabel(found.Module),
		"module":                found.Module,
		"jobReference":          firstNonEmpty(found.JobRef, found.Fields["jobRef"]),
		"companyName":           firstNonEmpty(company.Name, company.LegalName, "ZENITH ECLIPSE CO"),
		"issueDate":             formatDocDate(issueDate),
		"customerOrPartyName":   party,
		"customerPartyName":     party,
		"currentStatus":         firstNonEmpty(status, found.Status, "Valid"),
		"erpStatus":             found.Status,
		"verificationURL":       verificationURL(company, *found, found.Number),
		"verificationTimestamp": now,
	})
}

func wantsJSON(r *http.Request) bool {
	if strings.EqualFold(r.URL.Query().Get("format"), "json") {
		return true
	}
	return strings.Contains(strings.ToLower(r.Header.Get("Accept")), "application/json")
}

func verificationStatus(rec *Record) (string, bool) {
	if rec == nil {
		return "Not Found", false
	}
	status := firstNonEmpty(rec.Status, "Valid")
	lower := strings.ToLower(strings.TrimSpace(status))
	if lower == "cancelled" || lower == "canceled" || lower == "void" || lower == "rejected" {
		return status, false
	}
	if documentExpired(*rec) {
		return "Expired", false
	}
	return status, true
}

func documentExpired(rec Record) bool {
	for _, key := range []string{"validUntil", "expiryDate", "dueDate"} {
		v := strings.TrimSpace(rec.Fields[key])
		if v == "" {
			continue
		}
		for _, layout := range []string{"2006-01-02", time.RFC3339, "02/01/2006", "02-01-2006"} {
			if t, err := time.Parse(layout, v); err == nil {
				return t.Before(time.Now().Truncate(24 * time.Hour))
			}
		}
		if len(v) >= 10 {
			if t, err := time.Parse("2006-01-02", v[:10]); err == nil {
				return t.Before(time.Now().Truncate(24 * time.Hour))
			}
		}
	}
	return false
}

func renderVerifyPage(c Company, rec *Record, requestedSerial, status string, valid bool) string {
	esc := template.HTMLEscapeString
	serial := requestedSerial
	module := "-"
	jobRef := "-"
	erpStatus := "Not Found"
	version := "-"
	updated := "-"
	issueDate := "-"
	party := "-"
	validText := "Not Found"
	if rec != nil {
		serial = rec.Number
		module = moduleLabel(rec.Module)
		jobRef = firstNonEmpty(rec.JobRef, "-")
		erpStatus = firstNonEmpty(rec.Status, status)
		version = strconv.Itoa(rec.Version)
		updated = formatDocDate(rec.UpdatedAt)
		issueDate = formatDocDate(firstNonEmpty(rec.Fields["invoiceDate"], rec.Fields["date"], rec.CreatedAt))
		party = firstNonEmpty(rec.Fields["customer"], rec.Fields["customerName"], rec.Fields["buyer"], rec.Fields["partyName"], rec.Fields["consignee"], rec.Fields["shipper"], rec.Fields["to"], "-")
		validText = firstNonEmpty(status, erpStatus, "Valid")
	}
	badgeClass := "ok"
	if !valid || strings.EqualFold(status, "Draft") || strings.Contains(strings.ToLower(status), "pending") {
		badgeClass = "warn"
	}
	if rec == nil || strings.EqualFold(status, "Cancelled") || strings.EqualFold(status, "Expired") || strings.EqualFold(status, "Not Found") || strings.EqualFold(status, "Rejected") {
		badgeClass = "bad"
	}
	return `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Document Verification</title><style>body{margin:0;background:#eef4f8;font-family:Arial,Helvetica,sans-serif;color:#0f172a}.wrap{max-width:760px;margin:28px auto;padding:18px}.card{background:#fff;border:1px solid #dbeafe;border-top:8px solid #003366;border-radius:18px;box-shadow:0 18px 50px #0001;padding:26px}.brand{font-size:24px;font-weight:900;color:#003366;letter-spacing:.04em}.muted{color:#64748b}.badge{display:inline-block;border-radius:999px;padding:8px 14px;font-weight:900;margin:14px 0}.badge.ok{background:#dcfce7;color:#166534}.badge.warn{background:#fef3c7;color:#92400e}.badge.bad{background:#fee2e2;color:#991b1b}.grid{display:grid;grid-template-columns:1fr 1fr;gap:12px;margin-top:18px}.row{border:1px solid #e5e7eb;border-radius:12px;padding:12px;background:#fbfdff}.row b{display:block;font-size:12px;text-transform:uppercase;letter-spacing:.08em;color:#64748b;margin-bottom:4px}@media(max-width:650px){.grid{grid-template-columns:1fr}.wrap{margin:0;padding:10px}.card{border-radius:12px}}</style></head><body><div class="wrap"><div class="card"><div class="brand">` + esc(firstNonEmpty(c.Name, "ZENITH ECLIPSE CO")) + `</div><p class="muted">Server document verification</p><div class="badge ` + badgeClass + `">` + esc(validText) + `</div><div class="grid"><div class="row"><b>Serial Number</b>` + esc(serial) + `</div><div class="row"><b>ERP Status</b>` + esc(erpStatus) + `</div><div class="row"><b>Document Type</b>` + esc(module) + `</div><div class="row"><b>Job Reference</b>` + esc(jobRef) + `</div><div class="row"><b>Issue Date</b>` + esc(issueDate) + `</div><div class="row"><b>Customer / Party</b>` + esc(party) + `</div><div class="row"><b>Version</b>` + esc(version) + `</div><div class="row"><b>Last Updated</b>` + esc(updated) + `</div><div class="row"><b>Verification Timestamp</b>` + esc(time.Now().Format("2006-01-02 15:04:05")) + `</div></div><p class="muted" style="margin-top:20px">This page reads the current document status from the ERP database. If the serial number is not found, the result will show Not Found.</p></div></div></body></html>`
}

func renderDocHTML(c Company, rec Record, all []Record) string {
	if rec.Module == "letterhead" || rec.Module == "contract" {
		return renderLetterheadHTML(c, rec, all)
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

func docLanguage(rec Record) string {
	lang := firstNonEmpty(rec.Fields["documentLanguage"], rec.Fields["language"], rec.Fields["primaryLanguage"], "English")
	return normalizeLanguage(lang)
}

func normalizeLanguage(lang string) string {
	l := strings.ToLower(strings.TrimSpace(lang))
	switch {
	case strings.HasPrefix(l, "ar") || strings.Contains(l, "arabic") || strings.Contains(l, "عرب"):
		return "Arabic"
	case strings.HasPrefix(l, "ru") || strings.Contains(l, "russian") || strings.Contains(l, "рус"):
		return "Russian"
	case strings.HasPrefix(l, "zh") || strings.Contains(l, "chinese") || strings.Contains(l, "中文") || strings.Contains(l, "汉"):
		return "Chinese"
	default:
		return "English"
	}
}

func langCode(lang string) string {
	switch normalizeLanguage(lang) {
	case "Arabic":
		return "ar"
	case "Russian":
		return "ru"
	case "Chinese":
		return "zh"
	default:
		return "en"
	}
}

func isRTL(lang string) bool { return normalizeLanguage(lang) == "Arabic" }
func dirForLang(lang string) string {
	if isRTL(lang) {
		return "rtl"
	}
	return "ltr"
}
func rtlClass(lang string) string {
	if isRTL(lang) {
		return " rtl"
	}
	return ""
}

func tr(lang, key string) string {
	l := normalizeLanguage(lang)
	m := map[string]map[string]string{
		"Russian": {"buyerCustomer": "ПОКУПАТЕЛЬ / КЛИЕНТ", "productDetails": "ТОВАР / УСЛУГА / ТРАНСПОРТ", "terms": "УСЛОВИЯ", "bankDetails": "БАНКОВСКИЕ РЕКВИЗИТЫ", "subtotal": "Итого без налога", "vatTax": "НДС / Налог", "total": "Итого", "signature": "ПОДПИСЬ", "stamp": "ПЕЧАТЬ", "date": "Дата", "status": "Статус", "jobRef": "Номер работы", "verify": "Проверка", "qrVerify": "QR Проверка", "workflow": "Процесс", "address": "Адрес", "emailWeb": "Email / Web", "phone": "Телефон", "documentSecurity": "Защита документа", "version": "Версия", "fingerprint": "Отпечаток", "note": "Примечание", "documentNo": "Номер документа", "subject": "Тема", "to": "Кому", "continued": "Продолжение страницы", "transportationCharges": "Транспортные расходы", "serviceCharges": "Услуги / Доп. расходы", "discount": "Скидка", "logisticsDetails": "Логистика / отгрузка", "companyStamp": "Печать компании", "authorizedSignature": "Уполномоченная подпись", "documentDetails": "Детали документа", "transactionType": "Тип операции", "currency": "Валюта", "contact": "Контакт", "creditLimit": "Кредитный лимит", "outstanding": "Задолженность", "email": "Email", "trnVat": "ТРН / НДС", "productGoods": "Товар / Груз", "service": "Услуга", "route": "Маршрут", "container": "Контейнер", "seal": "Пломба", "blNo": "B/L №", "vesselVoyage": "Судно / рейс", "shipper": "Грузоотправитель", "consignee": "Грузополучатель", "notifyParty": "Уведомляемая сторона", "truck": "Грузовик", "driver": "Водитель", "loading": "Погрузка", "delivery": "Доставка", "dueDate": "Срок", "liveERPStatus": "Живой статус ERP", "companyLabel": "Этикетка компании", "stampHere": "Место печати", "labelHere": "Место этикетки", "contractType": "Тип договора", "expiry": "Срок действия", "secondLangPlaceholder": "Введите текст второго языка в поле текста второго языка."},
		"Chinese": {"buyerCustomer": "买方 / 客户", "productDetails": "产品 / 服务 / 运输详情", "terms": "条款和条件", "bankDetails": "银行信息", "subtotal": "小计", "vatTax": "增值税 / 税", "total": "总计", "signature": "签名", "stamp": "印章", "date": "日期", "status": "状态", "jobRef": "工作参考号", "verify": "验证", "qrVerify": "二维码验证", "workflow": "流程", "address": "地址", "emailWeb": "邮箱 / 网站", "phone": "电话", "documentSecurity": "文件安全", "version": "版本", "fingerprint": "指纹", "note": "备注", "documentNo": "文件编号", "subject": "主题", "to": "致", "continued": "续页", "transportationCharges": "运输费用", "serviceCharges": "服务 / 额外费用", "discount": "折扣", "logisticsDetails": "物流 / 运输详情", "companyStamp": "公司印章", "authorizedSignature": "授权签名", "documentDetails": "文件详情", "transactionType": "交易类型", "currency": "币种", "contact": "联系人", "creditLimit": "信用额度", "outstanding": "未结余额", "email": "邮箱", "trnVat": "税号 / VAT", "productGoods": "产品 / 货物", "service": "服务", "route": "路线", "container": "集装箱", "seal": "封条", "blNo": "提单号", "vesselVoyage": "船名 / 航次", "shipper": "发货人", "consignee": "收货人", "notifyParty": "通知方", "truck": "卡车", "driver": "司机", "loading": "装货", "delivery": "交付", "dueDate": "到期日", "liveERPStatus": "ERP实时状态", "companyLabel": "公司标签", "stampHere": "印章位置", "labelHere": "标签位置", "contractType": "合同类型", "expiry": "到期", "secondLangPlaceholder": "请在第二语言正文栏输入第二语言文本。"},
		"Arabic":  {"buyerCustomer": "المشتري / العميل", "productDetails": "تفاصيل المنتج / الخدمة / النقل", "terms": "الشروط والأحكام", "bankDetails": "تفاصيل البنك", "subtotal": "المجموع الفرعي", "vatTax": "ضريبة / VAT", "total": "الإجمالي", "signature": "التوقيع", "stamp": "الختم", "date": "التاريخ", "status": "الحالة", "jobRef": "مرجع العملية", "verify": "التحقق", "qrVerify": "تحقق QR", "workflow": "سير العمل", "address": "العنوان", "emailWeb": "البريد / الموقع", "phone": "الهاتف", "documentSecurity": "أمان المستند", "version": "الإصدار", "fingerprint": "البصمة", "note": "ملاحظة", "documentNo": "رقم المستند", "subject": "الموضوع", "to": "إلى", "continued": "متابعة الصفحة", "transportationCharges": "رسوم النقل", "serviceCharges": "رسوم الخدمات / الإضافية", "discount": "الخصم", "logisticsDetails": "تفاصيل الشحن / اللوجستيات", "companyStamp": "ختم الشركة", "authorizedSignature": "التوقيع المعتمد", "documentDetails": "تفاصيل المستند", "transactionType": "نوع العملية", "currency": "العملة", "contact": "جهة الاتصال", "creditLimit": "حد الائتمان", "outstanding": "الرصيد المستحق", "email": "البريد الإلكتروني", "trnVat": "TRN / VAT", "productGoods": "المنتج / البضاعة", "service": "الخدمة", "route": "المسار", "container": "الحاوية", "seal": "الختم", "blNo": "رقم بوليصة الشحن", "vesselVoyage": "السفينة / الرحلة", "shipper": "الشاحن", "consignee": "المرسل إليه", "notifyParty": "طرف الإخطار", "truck": "الشاحنة", "driver": "السائق", "loading": "التحميل", "delivery": "التسليم", "dueDate": "تاريخ الاستحقاق", "liveERPStatus": "حالة ERP المباشرة", "companyLabel": "ملصق الشركة", "stampHere": "مكان الختم", "labelHere": "مكان الملصق", "contractType": "نوع العقد", "expiry": "تاريخ الانتهاء", "secondLangPlaceholder": "أدخل نص اللغة الثانية في حقل نص اللغة الثانية."},
	}
	if mm, ok := m[l]; ok {
		if v := mm[key]; v != "" {
			return v
		}
	}
	english := map[string]string{"buyerCustomer": "BUYER / CUSTOMER", "productDetails": "PRODUCT / SERVICE / TRANSPORTATION DETAILS", "terms": "TERMS & CONDITIONS", "bankDetails": "BANK DETAILS", "subtotal": "Subtotal", "vatTax": "VAT/Tax", "total": "Total", "signature": "SIGNATURE", "stamp": "STAMP", "date": "Date", "status": "Status", "jobRef": "Job Reference", "verify": "Verify", "qrVerify": "QR Verify", "workflow": "Workflow", "address": "Address", "emailWeb": "Email / Web", "phone": "Phone", "documentSecurity": "Document Security", "version": "Version", "fingerprint": "Fingerprint", "note": "Note", "documentNo": "Document No.", "subject": "Subject", "to": "To", "continued": "Continued page", "transportationCharges": "Transportation Charges", "serviceCharges": "Services / Extra Charges", "discount": "Discount", "logisticsDetails": "Logistics / Shipping Details", "companyStamp": "Company Stamp", "authorizedSignature": "Authorized Signature", "documentDetails": "Document details", "transactionType": "Transaction Type", "currency": "Currency", "contact": "Contact", "creditLimit": "Credit Limit", "outstanding": "Outstanding", "email": "Email", "trnVat": "TRN/VAT", "productGoods": "Product/Goods", "service": "Service", "route": "Route", "container": "Container", "seal": "Seal", "blNo": "B/L No.", "vesselVoyage": "Vessel/Voyage", "shipper": "Shipper", "consignee": "Consignee", "notifyParty": "Notify Party", "truck": "Truck", "driver": "Driver", "loading": "Loading", "delivery": "Delivery", "dueDate": "Due Date", "liveERPStatus": "Live ERP status", "companyLabel": "Company Label", "stampHere": "Stamp appears here", "labelHere": "Place company label here", "contractType": "Contract Type", "expiry": "Expiry", "secondLangPlaceholder": "Enter the second language text in the Second Language Body field."}
	return firstNonEmpty(english[key], key)
}

func docTitleLang(module, lang string) string {
	t := docTitle(module)
	switch normalizeLanguage(lang) {
	case "Russian":
		switch module {
		case "quotation":
			return "КОММЕРЧЕСКОЕ ПРЕДЛОЖЕНИЕ"
		case "proforma_invoice":
			return "ПРОФОРМА ИНВОЙС"
		case "sales_invoice":
			return "СЧЕТ-ФАКТУРА"
		case "commercial_invoice":
			return "КОММЕРЧЕСКИЙ ИНВОЙС"
		case "packing_list":
			return "УПАКОВОЧНЫЙ ЛИСТ"
		case "bill_of_lading":
			return "КОНОСАМЕНТ"
		case "delivery_note":
			return "НАКЛАДНАЯ"
		case "contract":
			return "ЮРИДИЧЕСКИЙ ДОГОВОР"
		case "letterhead":
			return "ОФИЦИАЛЬНОЕ ПИСЬМО"
		}
	case "Chinese":
		switch module {
		case "quotation":
			return "报价单"
		case "proforma_invoice":
			return "形式发票"
		case "sales_invoice":
			return "销售发票"
		case "commercial_invoice":
			return "商业发票"
		case "packing_list":
			return "装箱单"
		case "bill_of_lading":
			return "提单"
		case "delivery_note":
			return "送货单"
		case "contract":
			return "法律合同"
		case "letterhead":
			return "正式信函"
		}
	case "Arabic":
		switch module {
		case "quotation":
			return "عرض سعر"
		case "proforma_invoice":
			return "فاتورة أولية"
		case "sales_invoice":
			return "فاتورة مبيعات"
		case "commercial_invoice":
			return "فاتورة تجارية"
		case "packing_list":
			return "قائمة التعبئة"
		case "bill_of_lading":
			return "بوليصة الشحن"
		case "delivery_note":
			return "إشعار التسليم"
		case "contract":
			return "عقد قانوني"
		case "letterhead":
			return "خطاب رسمي"
		}
	}
	return t
}

func bilingualEnabled(rec Record) bool {
	v := strings.ToLower(strings.TrimSpace(firstNonEmpty(rec.Fields["createBilingualDocument"], rec.Fields["bilingualDocument"], rec.Fields["bilingual"])))
	return strings.Contains(v, "bilingual") || strings.Contains(v, "two") || v == "yes" || v == "true" || v == "1"
}

func localizedStatus(status, lang string) string {
	st := firstNonEmpty(status, "Draft")
	key := strings.ToLower(strings.TrimSpace(st))
	tables := map[string]map[string]string{
		"Russian": {"draft": "Черновик", "pending": "Ожидает", "pending approval": "На утверждении", "approved": "Утвержден", "accepted": "Принят", "cancelled": "Отменен", "canceled": "Отменен", "expired": "Истек", "open": "Открыт", "valid": "Действителен", "not found": "Не найден"},
		"Chinese": {"draft": "草稿", "pending": "待处理", "pending approval": "待批准", "approved": "已批准", "accepted": "已接受", "cancelled": "已取消", "canceled": "已取消", "expired": "已过期", "open": "开放", "valid": "有效", "not found": "未找到"},
		"Arabic":  {"draft": "مسودة", "pending": "قيد الانتظار", "pending approval": "بانتظار الموافقة", "approved": "معتمد", "accepted": "مقبول", "cancelled": "ملغي", "canceled": "ملغي", "expired": "منتهي", "open": "مفتوح", "valid": "صالح", "not found": "غير موجود"},
	}
	if m, ok := tables[normalizeLanguage(lang)]; ok {
		if v := m[key]; v != "" {
			return v
		}
	}
	return st
}

func renderBusinessDocHTML(c Company, rec Record, all []Record) string {
	esc := template.HTMLEscapeString
	lang := docLanguage(rec)
	title := docTitleLang(rec.Module, lang)
	currency := firstNonEmpty(rec.Fields["currency"], c.BaseCurrency, "USD")
	verification := firstNonEmpty(rec.Fields["verificationCode"], rec.Fields["verification"], strings.ToUpper(rec.ID[:min(12, len(rec.ID))]))
	date := firstNonEmpty(rec.Fields["invoiceDate"], rec.Fields["date"], rec.CreatedAt)
	buyer := firstNonEmpty(rec.Fields["customer"], rec.Fields["buyer"], rec.Fields["customerName"], rec.Fields["name"], "Customer / Buyer not set")
	lines := lineItemsFromRecord(rec)
	showPricing := showPriceTableForDocument(rec)
	products, transport, services, subtotal, discount, tax, total := totalsByKind(rec, lines)
	maxLines := 8
	if !showPricing {
		maxLines = 12
	}
	chunks := chunkDocLines(lines, maxLines)
	if len(chunks) == 0 {
		chunks = [][]docLine{{}}
	}
	bankBlock := documentBankHTML(c, rec, all)
	security := documentSecurityHTML(rec, verification, lang)

	var pages strings.Builder
	for i, chunk := range chunks {
		firstPage := i == 0
		lastPage := i == len(chunks)-1
		pages.WriteString(`<section class="doc-shell business-doc a4-page` + rtlClass(lang) + `" dir="` + dirForLang(lang) + `">`)
		pages.WriteString(businessHeaderHTML(c, rec, title, date, verification, lang))
		if firstPage {
			pages.WriteString(`<section class="hero-row compact-hero"><div class="party-card customer-card"><div class="section-label">` + tr(lang, "buyerCustomer") + `</div>` + customerCardHTML(rec, buyer, lang) + `</div><div class="party-card detail-card"><div class="section-label">` + tr(lang, "productDetails") + `</div><div class="compact-details">` + documentDetailsHTML(rec, currency, lang) + `</div><div class="doc-pills"><span>` + esc(firstNonEmpty(rec.Fields["transactionType"], rec.Fields["dealMode"], moduleLabel(rec.Module))) + `</span><span>` + tr(lang, "verify") + ` ` + esc(verification) + `</span></div></div></section>`)
		} else {
			pages.WriteString(`<section class="continued-row"><span>` + esc(title) + ` · ` + esc(rec.Number) + `</span><b>` + tr(lang, "continued") + ` ` + esc(strconv.Itoa(i+1)) + `</b></section>`)
		}
		pages.WriteString(renderItemsTable(chunk, showPricing, lang))
		if lastPage {
			rightBlock := renderLogisticsSummaryHTML(rec, lines, lang)
			lowerClass := "lower no-price-lower"
			if showPricing {
				rightBlock = renderTotalsTable(currency, products, transport, services, subtotal, discount, tax, total, lang)
				lowerClass = "lower"
			}
			pages.WriteString(`<section class="` + lowerClass + `">` + renderTermsNotesBankHTML(c, rec, all, bankBlock, lang) + rightBlock + `</section>` + security + documentOptionBoxesHTML(c, rec, lang))
		}
		pages.WriteString(footerHTML(c, rec, all, verification, rec.Number, lang) + `</section>`)
	}
	return `<!doctype html><html><head><meta charset="utf-8"><title>` + esc(title+" "+rec.Number) + `</title>` + printCSS() + `</head><body dir="` + dirForLang(lang) + `" class="` + strings.TrimSpace(rtlClass(lang)) + `"><div class="actions"><button onclick="window.print()">Print / Save PDF</button></div><main class="doc-page a4-document">` + pages.String() + `</main></body></html>`
}

func businessHeaderHTML(c Company, rec Record, title, date, verification, lang string) string {
	esc := template.HTMLEscapeString
	return `<header class="clean-header"><div class="clean-brand">` + companyLogoHTML(c) + `<div><h1>` + esc(firstNonEmpty(c.Name, c.LogoText, "ZENITH ECLIPSE CO")) + `</h1><p class="slogan">` + esc(c.Slogan) + `</p></div></div><div class="clean-meta"><div class="doc-title">` + esc(title) + `</div><b>` + esc(rec.Number) + `</b><br><span>` + tr(lang, "date") + `: ` + esc(formatDocDate(date)) + `</span><br><span>` + tr(lang, "status") + `: ` + esc(localizedStatus(rec.Status, lang)) + `</span><br><span>` + tr(lang, "jobRef") + `: ` + esc(firstNonEmpty(rec.JobRef, "-")) + `</span><br><span>` + tr(lang, "verify") + `: ` + esc(verification) + `</span></div></header>`
}

func customerCardHTML(rec Record, buyer, lang string) string {
	esc := template.HTMLEscapeString
	rows := [][2]string{
		{tr(lang, "buyerCustomer"), buyer},
		{tr(lang, "contact"), firstNonEmpty(rec.Fields["contactPerson"], rec.Fields["contact"], rec.Fields["receiverName"])},
		{tr(lang, "address"), firstNonEmpty(rec.Fields["customerAddress"], rec.Fields["address"], rec.Fields["toAddress"], rec.Fields["partyAddress"])},
		{tr(lang, "email"), normalizeEmailValue(rec.Fields["customerEmail"])},
		{tr(lang, "phone"), firstNonEmpty(rec.Fields["customerPhone"], rec.Fields["phone"], rec.Fields["mobile"])},
		{tr(lang, "trnVat"), firstNonEmpty(rec.Fields["customerTaxNumber"], rec.Fields["taxNumber"], rec.Fields["trn"], rec.Fields["vatNumber"])},
		{tr(lang, "currency"), rec.Fields["currency"]},
		{tr(lang, "creditLimit"), rec.Fields["creditLimit"]},
		{tr(lang, "outstanding"), rec.Fields["outstandingBalance"]},
	}
	var b strings.Builder
	for i, row := range rows {
		if strings.TrimSpace(row[1]) == "" {
			continue
		}
		if i == 0 {
			b.WriteString(`<h3>` + esc(row[1]) + `</h3>`)
		} else {
			b.WriteString(`<div><b>` + esc(row[0]) + `:</b> ` + esc(row[1]) + `</div>`)
		}
	}
	if b.Len() == 0 {
		return `<h3>` + tr(lang, "buyerCustomer") + `</h3>`
	}
	return b.String()
}

func documentDetailsHTML(rec Record, currency, lang string) string {
	esc := template.HTMLEscapeString
	tx := strings.ToLower(firstNonEmpty(rec.Fields["transactionType"], rec.Fields["dealMode"], "Product + Transportation"))
	showProduct := strings.Contains(tx, "product") || strings.Contains(tx, "goods") || strings.Contains(tx, "+")
	showService := strings.Contains(tx, "service")
	showTransport := strings.Contains(tx, "transport") || strings.Contains(tx, "logistics") || strings.Contains(tx, "+")
	rows := [][2]string{{tr(lang, "transactionType"), firstNonEmpty(rec.Fields["transactionType"], rec.Fields["dealMode"])}, {tr(lang, "currency"), currency}, {"Incoterm", rec.Fields["incoterm"]}}
	if showProduct {
		rows = append(rows, [2]string{tr(lang, "productGoods"), firstNonEmpty(rec.Fields["productDescription"], rec.Fields["description"])}, [2]string{"HS Code", rec.Fields["hsCode"]}, [2]string{"Quantity", rec.Fields["quantity"]}, [2]string{"Weight", rec.Fields["weight"]})
	}
	if showService {
		rows = append(rows, [2]string{tr(lang, "service"), firstNonEmpty(rec.Fields["serviceDescription"], rec.Fields["description"])})
	}
	if showTransport || logisticsNoPriceModule(rec.Module) {
		route := firstNonEmpty(rec.Fields["route"], strings.TrimSpace(rec.Fields["pol"]+" → "+rec.Fields["pod"]))
		rows = append(rows, [2]string{tr(lang, "route"), route}, [2]string{tr(lang, "container"), firstNonEmpty(rec.Fields["containerNumber"], rec.Fields["containerNo"])}, [2]string{tr(lang, "seal"), firstNonEmpty(rec.Fields["sealNumber"], rec.Fields["sealNo"])}, [2]string{tr(lang, "blNo"), firstNonEmpty(rec.Fields["blNo"], rec.Fields["blNumber"])}, [2]string{tr(lang, "vesselVoyage"), rec.Fields["vesselVoyage"]}, [2]string{"POL", rec.Fields["pol"]}, [2]string{"POD", rec.Fields["pod"]}, [2]string{"FPOD", rec.Fields["fpod"]}, [2]string{tr(lang, "shipper"), rec.Fields["shipper"]}, [2]string{tr(lang, "consignee"), rec.Fields["consignee"]}, [2]string{tr(lang, "notifyParty"), rec.Fields["notifyParty"]}, [2]string{tr(lang, "truck"), rec.Fields["truckNumber"]}, [2]string{tr(lang, "driver"), rec.Fields["driverName"]}, [2]string{tr(lang, "loading"), rec.Fields["loadingLocation"]}, [2]string{tr(lang, "delivery"), rec.Fields["deliveryLocation"]})
	}
	rows = append(rows, [2]string{tr(lang, "dueDate"), rec.Fields["dueDate"]})
	var details strings.Builder
	for _, row := range rows {
		if strings.TrimSpace(row[1]) != "" && strings.TrimSpace(row[1]) != "→" {
			details.WriteString("<div><b>" + esc(row[0]) + ":</b> " + esc(row[1]) + "</div>")
		}
	}
	if details.Len() == 0 {
		details.WriteString("<div><b>" + esc(tr(lang, "documentDetails")) + ":</b> update fields in ERP record.</div>")
	}
	return details.String()
}

func chunkDocLines(lines []docLine, max int) [][]docLine {
	if max < 1 {
		max = 8
	}
	if len(lines) == 0 {
		return [][]docLine{{}}
	}
	out := [][]docLine{}
	for len(lines) > 0 {
		n := max
		if len(lines) < n {
			n = len(lines)
		}
		chunk := append([]docLine(nil), lines[:n]...)
		out = append(out, chunk)
		lines = lines[n:]
	}
	return out
}

func workflowHTML(rec Record, all []Record) string {
	if rec.JobRef == "" || len(all) == 0 {
		return ""
	}
	esc := template.HTMLEscapeString
	flow := []string{"quotation", "proforma_invoice", "sales_invoice", "commercial_invoice", "packing_list"}
	var sb strings.Builder
	sb.WriteString(`<section class="workflow"><div class="section-label">Document Workflow / Related Documents</div><div class="workflow-steps">`)
	for _, module := range flow {
		label := docTitle(module)
		found := Record{}
		ok := false
		for _, r := range all {
			if r.JobRef == rec.JobRef && r.Module == module {
				found = r
				ok = true
				break
			}
		}
		if ok {
			sb.WriteString(`<div class="workflow-step active"><b>` + esc(label) + `</b><span>` + esc(found.Number) + `</span><em>` + esc(found.Status) + `</em></div>`)
		} else {
			sb.WriteString(`<div class="workflow-step"><b>` + esc(label) + `</b><span>Pending</span><em>-</em></div>`)
		}
	}
	sb.WriteString(`</div><p>Job Reference: <b>` + esc(rec.JobRef) + `</b></p></section>`)
	return sb.String()
}

func renderLetterheadHTML(c Company, rec Record, all []Record) string {
	esc := template.HTMLEscapeString
	lang := docLanguage(rec)
	isContract := rec.Module == "contract"
	verification := firstNonEmpty(rec.Fields["verificationCode"], strings.ToUpper(rec.ID[:min(12, len(rec.ID))]))
	toName := firstNonEmpty(rec.Fields["partyName"], rec.Fields["to"], rec.Fields["customer"], rec.Fields["customerName"], "")
	toAddress := firstNonEmpty(rec.Fields["toAddress"], rec.Fields["customerAddress"], rec.Fields["address"], rec.Fields["partyAddress"], "")
	title := firstNonEmpty(rec.Fields["contractTitle"], rec.Fields["title"], docTitleLang(rec.Module, lang), "Official Letter")
	subject := firstNonEmpty(rec.Fields["subject"], rec.Fields["contractTitle"], title)
	date := firstNonEmpty(rec.Fields["date"], rec.Fields["startDate"], rec.CreatedAt)
	body := firstNonEmpty(rec.Fields["contractBody"], rec.Fields["body"], rec.Fields["terms"], rec.Fields["content"], rec.Fields["remarks"])
	if body == "" && isContract {
		body = "Write contract clauses here. The system will create A4 pages and repeat the same official header, footer, QR verification and border frame on every page.\n\nAdd more clauses in the contract body field. The signature and stamp block appears only after the final paragraph."
	}
	if body == "" {
		body = "Write your official letter content here. The system will create A4 pages and repeat the same official header, footer, QR verification and border frame on every page."
	}
	docType := docTitleLang("letterhead", lang)
	if isContract {
		docType = docTitleLang("contract", lang)
	}
	metaExtra := ""
	if isContract {
		metaExtra = `<br><span>` + tr(lang, "contractType") + `: ` + esc(firstNonEmpty(rec.Fields["contractType"], "Contract")) + `</span><br><span>` + tr(lang, "expiry") + `: ` + esc(firstNonEmpty(rec.Fields["expiryDate"], "-")) + `</span>`
	}
	bilingual := bilingualEnabled(rec)
	primaryLang := firstNonEmpty(rec.Fields["primaryLanguage"], lang)
	secondLang := normalizeLanguage(firstNonEmpty(rec.Fields["secondLanguage"], "Russian"))
	secondBody := firstNonEmpty(rec.Fields["secondLanguageBody"], rec.Fields["translatedBody"], rec.Fields["bodySecondLanguage"])
	if bilingual && strings.TrimSpace(secondBody) == "" {
		secondBody = tr(secondLang, "secondLangPlaceholder")
	}
	chunks := splitTextPagesForLetter(body, 1180, letterFirstPageBreakEnabled(rec))
	bilingualChunks := splitTextPagesForBilingual(body, secondBody, 1350, letterFirstPageBreakEnabled(rec))
	pageCount := len(chunks)
	if bilingual {
		pageCount = len(bilingualChunks)
	}
	bankBlock := documentBankHTML(c, rec, all)
	security := documentSecurityHTML(rec, verification, lang)
	var pages strings.Builder
	for i := 0; i < pageCount; i++ {
		chunk := ""
		if i < len(chunks) {
			chunk = chunks[i]
		}
		firstPage := i == 0
		lastPage := i == pageCount-1
		endBlock := ""
		if lastPage {
			endBlock = bankBlock + letterFinalSignatureHTML(c, rec) + security
		}
		pageClass := "doc-shell letter a4-page"
		if bilingual {
			pageClass += " bilingual-page"
		}
		if strings.TrimSpace(endBlock) != "" {
			pageClass += " has-end-block"
		}
		if firstPage {
			pageClass += " first-page"
		} else {
			pageClass += " continued-page"
		}
		if lastPage {
			pageClass += " final-page"
		}
		pageClass += rtlClass(lang)
		pages.WriteString(`<section class="` + pageClass + `" dir="` + dirForLang(lang) + `"><header class="clean-header"><div class="clean-brand">` + companyLogoHTML(c) + `<div><h1>` + esc(firstNonEmpty(c.Name, "ZENITH ECLIPSE CO")) + `</h1><p class="slogan">` + esc(c.Slogan) + `</p></div></div><div class="clean-meta"><div class="doc-title">` + esc(docType) + `</div><b>` + esc(rec.Number) + `</b><br><span>` + tr(lang, "date") + `: ` + esc(formatDocDate(date)) + `</span><br><span>` + tr(lang, "verify") + `: ` + esc(verification) + `</span>` + metaExtra + `</div></header>`)
		if firstPage {
			pages.WriteString(`<section class="letter-head-row"><div class="to-word">` + tr(lang, "to") + `:</div><div><b>` + esc(toName) + `</b><br>` + esc(toAddress) + `</div><div class="letter-subject"><b>` + tr(lang, "subject") + `:</b><br>` + esc(subject) + `</div></section><h2 class="letter-title">` + esc(title) + `</h2>`)
		} else {
			pages.WriteString(`<section class="continued-row"><span>` + esc(subject) + `</span><b>` + tr(lang, "continued") + ` ` + esc(strconv.Itoa(i+1)) + `</b></section>`)
		}
		if bilingual {
			bc := [2]string{}
			if i < len(bilingualChunks) {
				bc = bilingualChunks[i]
			}
			bodyClass := ""
			if firstPage {
				bodyClass += " first-page"
			} else {
				bodyClass += " continued-page"
			}
			if lastPage {
				bodyClass += " last-page"
			}
			pages.WriteString(bilingualBodyHTML(bc[0], bc[1], primaryLang, secondLang, bodyClass))
		} else {
			pages.WriteString(`<article class="letter-body long-text">` + strings.ReplaceAll(esc(chunk), "\n", "<br>") + `</article>`)
		}
		if lastPage {
			pages.WriteString(endBlock)
		}
		pages.WriteString(footerHTML(c, rec, all, verification, rec.Number, lang) + `</section>`)
	}
	return `<!doctype html><html><head><meta charset="utf-8"><title>` + esc(title) + `</title>` + printCSS() + `</head><body class="text-print-body ` + strings.TrimSpace(rtlClass(lang)) + `" dir="` + dirForLang(lang) + `"><div class="actions"><button onclick="window.print()">Print / Save PDF</button></div><main class="doc-page a4-document paged-letter">` + pages.String() + `</main></body></html>`
}

func letterFinalSignatureHTML(c Company, rec Record) string {
	esc := template.HTMLEscapeString
	lang := docLanguage(rec)
	stampOpt := strings.ToLower(firstNonEmpty(rec.Fields["stampOption"], "Placeholder"))
	sigOpt := strings.ToLower(firstNonEmpty(rec.Fields["signatureOption"], "Placeholder"))
	if !optionVisible(stampOpt) && !optionVisible(sigOpt) {
		return ""
	}
	stamp := `<div class="signature-placeholder stamp-box"><b>` + tr(lang, "stamp") + `</b><span>` + tr(lang, "stampHere") + `</span></div>`
	if strings.Contains(stampOpt, "use") && strings.TrimSpace(c.StampData) != "" {
		stamp = `<div class="signature-placeholder stamp-box"><img class="stamp" src="` + esc(c.StampData) + `" alt="stamp"><b>` + tr(lang, "stamp") + `</b></div>`
	} else if !optionVisible(stampOpt) {
		stamp = ""
	}
	name := esc(firstNonEmpty(rec.Fields["signatureName"], "Authorized Signatory"))
	sig := `<div class="signature-placeholder sign-box"><div class="sigline"></div><b>` + tr(lang, "signature") + `</b><span>` + name + `</span></div>`
	if strings.Contains(sigOpt, "use") && strings.TrimSpace(c.SignatureData) != "" {
		sig = `<div class="signature-placeholder sign-box"><img class="option-img" src="` + esc(c.SignatureData) + `" alt="signature"><b>` + tr(lang, "signature") + `</b><span>` + name + `</span></div>`
	} else if !optionVisible(sigOpt) {
		sig = ""
	}
	return `<section class="letter-signature-final">` + stamp + sig + `</section>`
}

func splitTextPages(body string, maxChars int) []string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	if maxChars < 800 {
		maxChars = 800
	}
	parts := strings.Split(body, "\n\n")
	chunks := []string{}
	current := ""
	flush := func() {
		if strings.TrimSpace(current) != "" {
			chunks = append(chunks, strings.TrimSpace(current))
			current = ""
		}
	}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if len(part) > maxChars {
			flush()
			for len(part) > maxChars {
				cut := strings.LastIndex(part[:maxChars], " ")
				if cut < maxChars/2 {
					cut = maxChars
				}
				chunks = append(chunks, strings.TrimSpace(part[:cut]))
				part = strings.TrimSpace(part[cut:])
			}
			current = part
			continue
		}
		addition := part
		if current != "" {
			addition = "\n\n" + part
		}
		if len(current)+len(addition) > maxChars {
			flush()
			current = part
		} else {
			current += addition
		}
	}
	flush()
	if len(chunks) == 0 {
		chunks = []string{""}
	}
	return chunks
}

func splitTextPagesForLetter(body string, maxChars int, forceFirstBreak bool) []string {
	body = strings.TrimSpace(strings.ReplaceAll(body, "\r\n", "\n"))
	if body == "" {
		return []string{""}
	}
	// Manual page-break markers are useful for legal contracts and letters.
	for _, marker := range []string{"[[PAGE_BREAK]]", "[PAGE_BREAK]", "---PAGE BREAK---"} {
		if strings.Contains(body, marker) {
			parts := strings.Split(body, marker)
			chunks := []string{}
			for _, part := range parts {
				for _, chunk := range splitTextPages(part, maxChars) {
					if strings.TrimSpace(chunk) != "" {
						chunks = append(chunks, chunk)
					}
				}
			}
			if len(chunks) == 0 {
				chunks = []string{""}
			}
			return chunks
		}
	}
	if !forceFirstBreak {
		return splitTextPages(body, maxChars)
	}
	firstMax := maxChars
	if firstMax > 1120 {
		firstMax = 1120
	}
	if len(body) <= firstMax {
		return splitTextPages(body, maxChars)
	}
	cut := strings.LastIndex(body[:firstMax], "\n\n")
	if cut < firstMax/2 {
		cut = strings.LastIndex(body[:firstMax], ". ")
		if cut >= firstMax/2 {
			cut += 1
		}
	}
	if cut < firstMax/2 {
		cut = strings.LastIndex(body[:firstMax], " ")
	}
	if cut < firstMax/2 {
		cut = firstMax
	}
	first := strings.TrimSpace(body[:cut])
	rest := strings.TrimSpace(body[cut:])
	chunks := []string{}
	if first != "" {
		chunks = append(chunks, first)
	}
	chunks = append(chunks, splitTextPages(rest, maxChars)...)
	return chunks
}

func splitTextPagesForBilingual(primary, second string, maxChars int, forceFirstBreak bool) [][2]string {
	p1 := splitTextPagesForLetter(primary, maxChars, forceFirstBreak)
	p2 := splitTextPagesForLetter(second, maxChars, forceFirstBreak)
	n := len(p1)
	if len(p2) > n {
		n = len(p2)
	}
	out := make([][2]string, n)
	for i := 0; i < n; i++ {
		if i < len(p1) {
			out[i][0] = p1[i]
		}
		if i < len(p2) {
			out[i][1] = p2[i]
		}
	}
	if len(out) == 0 {
		out = [][2]string{{"", ""}}
	}
	return out
}

func bilingualBodyHTML(primary, second, primaryLang, secondLang, pageClass string) string {
	esc := template.HTMLEscapeString
	return `<article class="bilingual-body` + esc(pageClass) + `"><div class="bilingual-col"><div class="bilingual-lang">` + esc(normalizeLanguage(primaryLang)) + `</div><div class="bilingual-text">` + strings.ReplaceAll(esc(primary), "\n", "<br>") + `</div></div><div class="bilingual-col` + rtlClass(secondLang) + `" dir="` + dirForLang(secondLang) + `"><div class="bilingual-lang">` + esc(normalizeLanguage(secondLang)) + `</div><div class="bilingual-text">` + strings.ReplaceAll(esc(second), "\n", "<br>") + `</div></div></article>`
}

func letterFirstPageBreakEnabled(rec Record) bool {
	v := strings.ToLower(strings.TrimSpace(firstNonEmpty(rec.Fields["startNewPageAfterFirstPage"], rec.Fields["continueOnNextA4Page"], rec.Fields["forceFirstPageBreak"])))
	return strings.Contains(v, "start new page") || strings.Contains(v, "continue") || v == "enable" || v == "enabled" || v == "yes" || v == "true" || v == "1"
}

func documentBankHTML(c Company, rec Record, all []Record) string {
	if !bankEligibleModule(rec.Module) {
		return ""
	}
	show := strings.ToLower(strings.TrimSpace(firstNonEmpty(rec.Fields["bankDetailsOption"], rec.Fields["showBankDetails"], rec.Fields["includeBankDetails"])))
	selected := strings.TrimSpace(firstNonEmpty(rec.Fields["bankAccountId"], rec.Fields["selectedBankAccount"]))
	if show != "selected bank account" && selected == "" {
		return ""
	}
	if selected == "" {
		return ""
	}
	esc := template.HTMLEscapeString
	bankName := ""
	account := ""
	accountNo := ""
	iban := ""
	swift := ""
	currency := ""
	for _, b := range all {
		if b.Module != "bank_account" {
			continue
		}
		if b.ID == selected || b.Number == selected || strings.EqualFold(b.Fields["bankName"], selected) || strings.EqualFold(b.Fields["iban"], selected) || strings.EqualFold(b.Fields["accountNumber"], selected) || strings.EqualFold(b.Fields["accountName"], selected) {
			bankName = b.Fields["bankName"]
			account = b.Fields["accountName"]
			accountNo = b.Fields["accountNumber"]
			iban = b.Fields["iban"]
			swift = b.Fields["swift"]
			currency = b.Fields["currency"]
			break
		}
	}
	parts := []string{}
	if strings.TrimSpace(bankName) != "" {
		parts = append(parts, "Bank Name: "+esc(bankName))
	}
	if strings.TrimSpace(account) != "" {
		parts = append(parts, "Account Name: "+esc(account))
	}
	if strings.TrimSpace(accountNo) != "" {
		parts = append(parts, "Account No: "+esc(accountNo))
	}
	if strings.TrimSpace(iban) != "" {
		parts = append(parts, "IBAN: "+esc(iban))
	}
	if strings.TrimSpace(swift) != "" {
		parts = append(parts, "SWIFT: "+esc(swift))
	}
	if strings.TrimSpace(currency) != "" {
		parts = append(parts, "Currency: "+esc(currency))
	}
	if len(parts) == 0 {
		return ""
	}
	return `<div class="bank"><b>Bank Details</b><br>` + strings.Join(parts, "<br>") + `</div>`
}

func bankEligibleModule(module string) bool {
	switch module {
	case "quotation", "proforma_invoice", "sales_invoice", "commercial_invoice", "customer_statement", "supplier_statement", "statement":
		return true
	default:
		return false
	}
}

func renderTermsNotesBankHTML(c Company, rec Record, all []Record, bankBlock, lang string) string {
	esc := template.HTMLEscapeString
	var b strings.Builder
	b.WriteString(`<div class="notes terms-notes">`)
	notesText := firstNonEmpty(rec.Fields["notes"], rec.Fields["remarks"])
	notesOpt := strings.ToLower(firstNonEmpty(rec.Fields["notesOption"], rec.Fields["showNotes"], "Hide"))
	if strings.TrimSpace(notesText) != "" && notesOpt != "hide" && notesOpt != "remove" && notesOpt != "no" {
		b.WriteString(`<div class="doc-note-block"><div class="section-label">` + tr(lang, "note") + `</div>` + strings.ReplaceAll(esc(notesText), "\n", "<br>") + `</div>`)
	}
	defaultTermsOption := "Default Terms Template"
	if logisticsNoPriceModule(rec.Module) {
		defaultTermsOption = "Hide"
	}
	termsOpt := strings.ToLower(firstNonEmpty(rec.Fields["termsOption"], defaultTermsOption))
	termsText := ""
	if tid := strings.TrimSpace(rec.Fields["termsTemplateId"]); tid != "" {
		for _, t := range all {
			if t.Module == "terms_template" && (t.ID == tid || t.Number == tid) {
				termsText = firstNonEmpty(t.Fields["templateText"], t.Fields["terms"], t.Fields["body"], t.Fields["notes"])
				break
			}
		}
	}
	switch {
	case strings.Contains(termsOpt, "hide") || strings.Contains(termsOpt, "remove") || termsOpt == "no":
		termsText = ""
	case strings.Contains(termsOpt, "custom") || strings.Contains(termsOpt, "edit"):
		if termsText == "" {
			termsText = firstNonEmpty(rec.Fields["terms"], rec.Fields["paymentTerms"], rec.Fields["termsTemplate"])
		}
	case strings.Contains(termsOpt, "template") && strings.TrimSpace(rec.Fields["termsTemplate"]) != "":
		if termsText == "" {
			termsText = rec.Fields["termsTemplate"]
		}
	default:
		if termsText == "" {
			termsText = firstNonEmpty(rec.Fields["terms"], rec.Fields["paymentTerms"], rec.Fields["termsTemplate"], c.DefaultTerms)
		}
	}
	if strings.TrimSpace(termsText) != "" {
		b.WriteString(`<div class="doc-terms-block"><div class="section-label">` + tr(lang, "terms") + `</div>` + strings.ReplaceAll(esc(termsText), "\n", "<br>") + `</div>`)
	}
	if strings.TrimSpace(bankBlock) != "" {
		b.WriteString(bankBlock)
	}
	b.WriteString(`</div>`)
	return b.String()
}

func documentSecurityHTML(rec Record, verification, lang string) string {
	esc := template.HTMLEscapeString
	status := firstNonEmpty(rec.Status, "Draft")
	hash := sha256.Sum256([]byte(rec.ID + "|" + rec.Number + "|" + verification + "|" + strconv.Itoa(rec.Version)))
	fingerprint := strings.ToUpper(hex.EncodeToString(hash[:])[:16])
	return `<section class="doc-security"><b>` + tr(lang, "documentSecurity") + `</b><span> ` + tr(lang, "verify") + ` ` + tr(lang, "qrVerify") + `: ` + esc(verification) + `</span><span> ` + tr(lang, "status") + `: ` + esc(status) + `</span><span> ` + tr(lang, "version") + `: ` + esc(strconv.Itoa(rec.Version)) + `</span><span> ` + tr(lang, "fingerprint") + `: ` + esc(fingerprint) + `</span></section>`
}

func printCSS() string {
	return `<style>
	@page{size:210mm 297mm;margin:0}*{box-sizing:border-box}html,body{margin:0;padding:0}body{background:#e9eef5;font-family:Arial,Helvetica,sans-serif;color:#0f172a}.actions{position:fixed;right:18px;top:18px;z-index:50}.actions button{background:#003366;color:#fff;border:0;border-radius:10px;padding:10px 14px;font-weight:800}.doc-page{width:210mm;margin:14px auto}.doc-shell{width:210mm;height:297mm;min-height:297mm;background:#fff;position:relative;overflow:hidden;padding:13mm 13mm 31.5mm;box-shadow:0 16px 50px rgba(15,23,42,.16);break-after:page;page-break-after:always}.doc-shell.letter{display:flex;flex-direction:column}.doc-shell:last-child{break-after:auto;page-break-after:auto}.doc-shell:after{content:"";position:absolute;top:3.2mm;right:3.2mm;bottom:3.2mm;left:3.2mm;border:1.35mm solid #003366;border-radius:1.8mm;pointer-events:none;z-index:30}.clean-header,.hero-row,.doc-table,.lower,.doc-security,.doc-options,.letter-body,.letter-head-row,.letter-title,.continued-row,.doc-footer,.bilingual-body,.letter-signature-final{position:relative;z-index:2}.clean-header{display:grid;grid-template-columns:1.25fr .9fr;gap:12mm;align-items:start;padding-bottom:5mm;border-bottom:1.4px solid #dbeafe}.clean-brand{display:flex;gap:4mm;align-items:flex-start;min-width:0}.brand-logo{width:18mm;height:18mm;border-radius:50%;object-fit:cover;border:1px solid #dbeafe}.brand-fallback{width:18mm;height:18mm;border-radius:50%;background:#003366;color:#fff;display:flex;align-items:center;justify-content:center;font-weight:900;font-size:20px;flex:none}.clean-brand h1{font-size:22px;line-height:1.05;margin:2mm 0 1.5mm;color:#0f172a;letter-spacing:.03em}.slogan{font-size:7.2px;line-height:1.35;font-weight:800;letter-spacing:.07em;text-transform:uppercase;color:#003366;margin:0;max-width:105mm}.clean-meta{text-align:right;font-size:10.5px;line-height:1.45;color:#334155;word-break:break-word}.doc-title{font-size:14px;font-weight:900;letter-spacing:.02em;text-transform:uppercase;color:#003366;margin-bottom:1.5mm}.letter-head-row{display:grid;grid-template-columns:12mm 1.05fr 1.1fr;gap:4mm;padding:4mm 0;border-bottom:1px solid #edf2f7;font-size:11.5px}.to-word{font-family:Georgia,serif;font-size:24px;font-weight:900;color:#0f172a;line-height:1}.letter-subject{text-align:right;font-size:11.5px;line-height:1.45}.letter-title{text-align:center;margin:5mm 0 2mm;font-size:16px;letter-spacing:.02em;color:#0f172a;text-transform:uppercase}.continued-row{display:flex;justify-content:space-between;gap:4mm;border-bottom:1px solid #edf2f7;padding:3mm 0;color:#64748b;font-size:10px}.continued-row b{color:#003366;text-transform:uppercase;letter-spacing:.08em}.hero-row{display:grid;grid-template-columns:1fr 1fr;gap:3mm;margin:3mm 0}.party-card{border:1px solid #dce8f5;border-radius:3mm;padding:2.5mm 3mm;background:#fbfdff;min-height:18mm}.section-label{font-size:8.3px;font-weight:900;letter-spacing:.02em;text-transform:uppercase;color:#64748b;margin-bottom:1.2mm}.party-card h3{margin:0 0 1.2mm;color:#0f172a;font-size:12.5px}.party-card p,.party-card div{margin:.55mm 0;font-size:9.8px;line-height:1.22;color:#23374d}.compact-details div{margin:.4mm 0;font-size:9.6px;line-height:1.18}.doc-pills{margin-top:3mm}.doc-pills span{display:inline-block;background:#eff6ff;border:1px solid #bfdbfe;color:#003366;border-radius:999px;padding:1.3mm 3mm;margin:1mm 1.5mm 0 0;font-size:9.5px;font-weight:800}.doc-table{width:100%;border-collapse:separate;border-spacing:0;font-size:9.4px;border:1px solid #dce8f5;border-radius:3mm;overflow:hidden;page-break-inside:auto;table-layout:fixed}.doc-table th{background:#003366;color:#fff;font-size:7.55px;text-transform:uppercase;letter-spacing:.025em;padding:1.8mm .95mm}.doc-table td{border-bottom:1px solid #e8eef5;padding:1.85mm .95mm;vertical-align:top;word-break:normal;overflow-wrap:anywhere}.doc-table .desc-cell{font-size:9.6px;line-height:1.25}.doc-table tr:last-child td{border-bottom:0}.tag{background:#eef6ff;color:#003366;padding:1mm 2mm;border-radius:999px;font-size:8px;font-weight:800}.num{text-align:right;white-space:nowrap}.lower{display:grid;grid-template-columns:1.06fr .94fr;gap:6mm;margin-top:5mm}.lower.no-price-lower,.lower.no-pricing{display:block}.lower.no-price-lower .notes,.lower.no-pricing .notes{min-height:22mm}.notes{font-size:9.2px;line-height:1.32;min-height:18mm;color:#24364b}.doc-note-block,.doc-terms-block{margin-bottom:2mm}.terms-notes{break-inside:avoid;page-break-inside:avoid}.bank{border:1px solid #bfdbfe;background:#eff6ff;border-radius:3mm;padding:2.5mm 3mm;margin-top:3mm;font-size:9.5px}.muted-bank{color:#64748b}.totals{width:100%;font-size:9.3px;border-collapse:collapse;background:#fbfdff}.compact-summary{max-width:72mm;margin-left:auto}.compact-summary td{padding:1.15mm 2mm}.totals td{border-bottom:1px solid #e2e8f0;padding:2mm 2.5mm}.totals td:last-child{text-align:right;font-weight:800}.totals .grand td{font-size:11.8px;border-bottom:2px solid #003366;background:#eff6ff}.doc-security{margin-top:2.2mm;border:1px solid #dbeafe;border-left:3px solid #003366;border-radius:2.5mm;background:#f8fbff;padding:1.5mm 2mm;font-size:8px;color:#334155;display:flex;gap:2.5mm;flex-wrap:wrap;break-inside:avoid;page-break-inside:avoid}.doc-security b{color:#0f172a}.bilingual-page .doc-security{margin-top:2mm;padding:1.6mm 2mm;font-size:7.9px;gap:1.8mm}.doc-options{display:flex;gap:3mm;justify-content:flex-end;align-items:stretch;margin-top:4mm;break-inside:avoid;page-break-inside:avoid}.option-box{border:1px dashed #93c5fd;border-radius:3mm;text-align:center;min-height:18mm;display:flex;align-items:center;justify-content:center;padding:2mm;color:#003366;background:#fbfdff}.option-box.label-box{width:34mm}.option-box b,.stamp-signature-combo b{display:block;font-size:9.2px;margin-top:.6mm}.option-box span,.stamp-signature-combo span{display:block;font-size:7.8px;color:#64748b}.stamp-signature-combo{border:1px dashed #93c5fd;border-radius:3mm;background:#fbfdff;display:grid;grid-template-columns:2fr 1fr;width:72mm;min-height:18mm;overflow:hidden;color:#003366}.stamp-area,.signature-area{display:flex;flex-direction:column;align-items:center;justify-content:center;text-align:center;padding:1.8mm}.stamp-area{border-right:1px dashed #bfdbfe}.icon{font-size:7.5px;border:1px solid #93c5fd;border-radius:999px;padding:.7mm 1.6mm;display:inline-block}.sigline{border-top:1px solid #111;margin:0 auto 1mm;width:22mm}.stamp{max-width:30mm;max-height:16mm;object-fit:contain}.option-img{max-width:28mm;max-height:14mm;object-fit:contain}.official-options{align-items:flex-end}.official-stamp-sign{position:relative;width:42mm;height:28mm;min-height:28mm;display:flex;align-items:center;justify-content:center;margin-left:auto;break-inside:avoid;page-break-inside:avoid}.official-stamp-layer{position:absolute;left:4mm;top:1.5mm;width:25mm;height:25mm;display:flex;align-items:center;justify-content:center}.official-stamp-img{width:25mm;height:25mm;max-width:25mm;max-height:25mm;object-fit:contain;border-radius:50%;image-rendering:auto}.official-sign-layer{position:absolute;left:16.5mm;top:8mm;width:22mm;height:12mm;display:flex;align-items:center;justify-content:center;transform:rotate(-3deg);z-index:3}.official-sign-img{max-width:22mm;max-height:12mm;width:auto;height:auto;object-fit:contain}.official-caption{position:absolute;right:0;bottom:.4mm;width:26mm;text-align:center;font-size:7.4px;color:#334155;border-top:1px solid #94a3b8;padding-top:.6mm}.official-stamp-placeholder{width:21mm;height:21mm;border:1px dashed #93c5fd;border-radius:50%;display:flex;align-items:center;justify-content:center;color:#003366;font-size:7.2px;font-weight:900;text-align:center}.official-sign-placeholder{width:20mm;height:7mm;border-top:1px solid #334155;color:#64748b;font-size:6.9px;text-align:center;padding-top:.8mm;background:transparent}.qr-scan-text{display:block;font-size:6.65px;line-height:1.05;color:#64748b;font-weight:500}.letter-body{font-size:11.1px;line-height:1.54;min-height:150mm;padding-top:4mm;color:#172b44;overflow:hidden}.letter-signature-final{display:flex;gap:3mm;justify-content:flex-end;margin-top:3mm;break-inside:avoid;page-break-inside:avoid}.bilingual-page .letter-signature-final{margin-top:2mm}.signature-placeholder{border:1px dashed #93c5fd;border-radius:3mm;min-height:18mm;text-align:center;padding:2.2mm;color:#003366;background:#fbfdff}.signature-placeholder.stamp-box{width:50mm}.signature-placeholder.sign-box{width:25mm}.signature-placeholder b{display:block;font-size:9.2px}.signature-placeholder span{font-size:7.8px;color:#64748b}.bilingual-body{display:grid;grid-template-columns:1fr 1fr;gap:5mm;font-size:10.2px;line-height:1.36;padding-top:2.5mm;overflow:hidden;flex:1 1 auto;min-height:0;height:auto;max-height:none;align-items:stretch}.doc-shell.letter.bilingual-page .clean-header,.doc-shell.letter.bilingual-page .letter-head-row,.doc-shell.letter.bilingual-page .letter-title,.doc-shell.letter.bilingual-page .continued-row,.doc-shell.letter.bilingual-page .letter-signature-final,.doc-shell.letter.bilingual-page .doc-security{flex:0 0 auto}.bilingual-col{border:1px solid #e5eef7;border-radius:2.5mm;padding:3mm;overflow:hidden;height:100%;min-height:0;background:#fff;display:flex;flex-direction:column}.bilingual-col.rtl{direction:rtl;text-align:right}.bilingual-lang{font-size:8px;font-weight:900;color:#003366;text-transform:uppercase;margin-bottom:2mm;flex:0 0 auto}.bilingual-text{flex:1 1 auto;min-height:0;overflow:hidden;white-space:normal}.doc-footer{position:absolute;left:13mm;right:13mm;bottom:7.6mm;height:20.6mm;border-top:1px solid #e5eef7;padding-top:1.5mm;display:flex;flex-direction:column;gap:.35mm;background:#fff;overflow:hidden;z-index:5}.footer-main{display:flex;gap:2.2mm;align-items:flex-start;height:12.2mm;min-height:0;overflow:hidden;width:100%}.footer-main .footer-item:nth-child(1){width:50mm}.footer-main .footer-item:nth-child(2){width:40mm}.footer-main .footer-item:nth-child(3){width:27mm}.footer-item{font-size:8.05px;line-height:1.15;color:#334155;overflow:hidden;min-width:0;max-height:12mm}.footer-item b{display:block;color:#0f172a;margin-bottom:.25mm;font-size:7.35px;letter-spacing:.01em;text-transform:uppercase}.footer-qr{width:54mm;display:flex;gap:1.2mm;align-items:flex-start;justify-content:flex-end;min-width:0;overflow:hidden;height:12.2mm;padding-right:1.8mm}.footer-qr img{width:12.2mm;height:12.2mm;border:1px solid #d5e4f0;border-radius:1.4mm;background:#fff;padding:.45mm;flex:none}.footer-qr-text{font-size:7.25px;line-height:1.05;color:#334155;text-align:left;overflow:hidden;min-width:0;max-height:12mm;word-break:normal}.footer-qr-text b{display:block;color:#0f172a;margin-bottom:.15mm;font-size:7.15px;letter-spacing:.01em;text-transform:uppercase}.footer-qr-hidden{justify-content:flex-end}.footer-qr-hidden img{display:none}.footer-qr-center{justify-content:center;padding-right:0}.footer-workflow{display:flex;flex-direction:column;height:6.6mm;width:100%;min-width:0;overflow:hidden}.wf-line1{font-size:7.75px;line-height:1.03;color:#003366;font-weight:900;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;min-width:0}.wf-line2{font-size:6.9px;line-height:1.03;color:#334155;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;min-width:0}.wf-head{color:#003366;font-weight:900;white-space:nowrap}.wf-item{white-space:nowrap}.wf-item b{color:#003366}.wf-serial,.wf-status{white-space:nowrap}.wf-status{color:#64748b}.wf-dash{color:#94a3b8;margin:0 .2mm}.wf-sep{color:#cbd5e1;margin:0 .25mm}.linked{display:none!important}.rtl{direction:rtl;text-align:right;font-family:Arial,Tahoma,"Segoe UI",sans-serif}.rtl .clean-header{grid-template-columns:.9fr 1.25fr}.rtl .clean-brand{flex-direction:row-reverse;text-align:right}.rtl .clean-meta{text-align:left}.rtl .hero-row{direction:rtl}.rtl .num{text-align:left}.rtl .letter-subject{text-align:left}.rtl .footer-main{direction:rtl}.rtl .footer-qr-text{text-align:right}.rtl .footer-workflow{direction:rtl;text-align:right}@media(max-width:850px){body{background:#fff}.doc-page{margin:0;width:100%;overflow:auto}.doc-shell{width:210mm;height:297mm;box-shadow:none;border-radius:0}.actions{position:sticky;top:8px;right:auto;padding:8px;background:#fff}}@media print{body{background:#fff}.actions{display:none}.doc-page{width:210mm;margin:0!important}.doc-shell{margin:0!important;box-shadow:none!important;width:210mm!important;height:297mm!important;min-height:297mm!important;page-break-after:always!important;break-after:page!important}.doc-shell:last-child{page-break-after:auto!important;break-after:auto!important}.doc-shell:after{content:"";position:absolute;top:3.2mm;right:3.2mm;bottom:3.2mm;left:3.2mm;border:1.35mm solid #003366;border-radius:1.8mm;pointer-events:none;z-index:30}.clean-header,.doc-footer{print-color-adjust:exact;-webkit-print-color-adjust:exact}}
	</style>`
}

func documentOptionBoxesHTML(c Company, rec Record, lang string) string {
	esc := template.HTMLEscapeString
	lang = normalizeLanguage(lang)
	labelBox := ""
	if optionVisible(firstNonEmpty(rec.Fields["companyLabelOption"], rec.Fields["labelOption"], "Hide")) {
		labelOpt := strings.ToLower(firstNonEmpty(rec.Fields["companyLabelOption"], rec.Fields["labelOption"], "Hide"))
		text := esc(firstNonEmpty(rec.Fields["companyLabelText"], rec.Fields["companyLabel"], tr(lang, "labelHere")))
		content := `<div class="icon">LABEL</div><b>` + tr(lang, "companyLabel") + `</b><span>` + text + `</span>`
		if strings.Contains(labelOpt, "use") && strings.TrimSpace(c.LabelData) != "" {
			content = `<img class="option-img" src="` + esc(c.LabelData) + `" alt="company label"><b>` + tr(lang, "companyLabel") + `</b>`
		}
		labelBox = `<div class="option-box label-box"><div>` + content + `</div></div>`
	}

	stampOpt := strings.ToLower(firstNonEmpty(rec.Fields["stampOption"], rec.Fields["stamp"], "Hide"))
	sigOpt := strings.ToLower(firstNonEmpty(rec.Fields["signatureOption"], rec.Fields["signature"], "Hide"))
	status := strings.ToLower(firstNonEmpty(rec.Status, rec.Fields["status"]))
	finalDoc := strings.Contains(status, "approved") || strings.Contains(status, "accepted") || strings.Contains(status, "final") || strings.Contains(status, "issued") || strings.Contains(status, "paid") || strings.Contains(status, "closed") || strings.Contains(status, "delivered")

	stampVisible := optionVisible(stampOpt) || (finalDoc && strings.TrimSpace(c.StampData) != "")
	signVisible := optionVisible(sigOpt) || (finalDoc && strings.TrimSpace(c.SignatureData) != "")

	stampHTML := ""
	if stampVisible {
		useStamp := strings.Contains(stampOpt, "use") || finalDoc
		if useStamp && strings.TrimSpace(c.StampData) != "" {
			stampHTML = `<img class="official-stamp-img" src="` + esc(c.StampData) + `" alt="company stamp">`
		} else {
			stampHTML = `<div class="official-stamp-placeholder"><span>` + tr(lang, "stamp") + `</span></div>`
		}
	}

	signHTML := ""
	if signVisible {
		useSign := strings.Contains(sigOpt, "use") || finalDoc
		if useSign && strings.TrimSpace(c.SignatureData) != "" {
			signHTML = `<img class="official-sign-img" src="` + esc(c.SignatureData) + `" alt="signature">`
		} else if !finalDoc || strings.TrimSpace(c.SignatureData) == "" {
			signHTML = `<div class="official-sign-placeholder"><span>` + tr(lang, "signature") + `</span></div>`
		}
	}

	combo := ""
	if stampHTML != "" || signHTML != "" {
		name := esc(firstNonEmpty(rec.Fields["signatureName"], rec.Fields["authorizedBy"], "Authorized Signature"))
		combo = `<div class="official-stamp-sign"><div class="official-stamp-layer">` + stampHTML + `</div><div class="official-sign-layer">` + signHTML + `</div><div class="official-caption">` + name + `</div></div>`
	}
	if labelBox == "" && combo == "" {
		return ""
	}
	return `<section class="doc-options compact-options official-options">` + labelBox + combo + `</section>`
}

func optionVisible(value string) bool {
	v := strings.ToLower(strings.TrimSpace(value))
	return v != "hide" && v != "hidden" && v != "no" && v != "none" && v != "false" && v != "0"
}

func singlePhone(c Company) string {
	phone := strings.TrimSpace(c.Phone)
	if phone == "" {
		phone = strings.TrimSpace(c.WhatsApp)
	}
	phone = strings.ReplaceAll(phone, "\r", " ")
	phone = strings.ReplaceAll(phone, "\n", " ")
	phone = strings.Join(strings.Fields(phone), " ")
	if strings.Contains(phone, "|") {
		phone = strings.TrimSpace(strings.Split(phone, "|")[0])
	}
	if strings.Contains(phone, "+968") || phone == "" {
		phone = "+971 42 500 715"
	}
	return phone
}

func footerHTML(c Company, rec Record, all []Record, verification, docNo, lang string) string {
	esc := template.HTMLEscapeString
	verifyURL := verificationURL(c, rec, docNo)
	qr := pseudoQRCodeDataURI(verifyURL)
	workflow := ""
	if compactWorkflowAllowed(rec) {
		workflow = compactWorkflowFooterHTML(c, rec, all)
	}
	qrOpt := strings.ToLower(firstNonEmpty(rec.Fields["qrPosition"], "Footer Right"))
	qrHTML := `<div class="footer-qr"><img src="` + qr + `" alt="verification code"><div class="footer-qr-text"><b>` + tr(lang, "qrVerify") + `</b><span class="qr-scan-text">Scan to verify</span><br><span>` + esc(docNo) + `</span></div></div>`
	if strings.Contains(qrOpt, "hide") || strings.Contains(qrOpt, "no") {
		qrHTML = `<div class="footer-qr footer-qr-hidden"><div class="footer-qr-text"><b>` + tr(lang, "qrVerify") + `</b><span>` + esc(docNo) + `</span></div></div>`
	} else if strings.Contains(qrOpt, "center") {
		qrHTML = strings.Replace(qrHTML, `footer-qr`, `footer-qr footer-qr-center`, 1)
	}
	return `<footer class="doc-footer"><div class="footer-main"><div class="footer-item"><b>` + tr(lang, "address") + `</b>` + esc(firstNonEmpty(c.Address, c.City+", "+c.Country)) + `</div><div class="footer-item"><b>` + tr(lang, "emailWeb") + `</b>` + esc(normalizeEmailValue(c.Email)) + `<br>` + esc(c.Website) + `</div><div class="footer-item"><b>` + tr(lang, "phone") + `</b>` + esc(singlePhone(c)) + `</div>` + qrHTML + `</div>` + workflow + `</footer>`
}

func compactWorkflowFooterHTML(c Company, rec Record, all []Record) string {
	esc := template.HTMLEscapeString
	jobRef := firstNonEmpty(rec.JobRef, rec.Fields["jobRef"], "-")
	steps := []struct{ Module, Code string }{{"quotation", "QT"}, {"proforma_invoice", "PI"}, {"sales_invoice", "SI"}, {"commercial_invoice", "CI"}, {"packing_list", "PL"}}
	blFound := rec.Module == "bill_of_lading"
	for _, r := range all {
		if r.JobRef == jobRef && r.Module == "bill_of_lading" {
			blFound = true
			break
		}
	}
	if blFound {
		steps = append(steps, struct{ Module, Code string }{"bill_of_lading", "BL"})
	}
	findLinked := func(module string) (Record, bool) {
		if rec.Module == module {
			return rec, true
		}
		if jobRef == "-" {
			return Record{}, false
		}
		for _, r := range all {
			if r.JobRef == jobRef && r.Module == module {
				return r, true
			}
		}
		return Record{}, false
	}
	lang := docLanguage(rec)
	items := []string{}
	for _, step := range steps {
		serial := "Not Created"
		status := "Pending"
		if found, ok := findLinked(step.Module); ok {
			serial = found.Number
			status = firstNonEmpty(found.Status, "Draft")
		}
		items = append(items, `<span class="wf-item"><b>`+esc(step.Code)+`:</b> <span class="wf-serial">`+esc(serial)+`</span> <span class="wf-dash">—</span> <span class="wf-status">`+esc(localizedStatus(status, lang))+`</span></span>`)
	}
	return `<div class="footer-workflow"><div class="wf-line1"><span class="wf-head">` + tr(lang, "workflow") + ` / ` + tr(lang, "jobRef") + `: ` + esc(jobRef) + ` / ` + tr(lang, "status") + `: ` + esc(localizedStatus(firstNonEmpty(rec.Status, "Draft"), lang)) + `</span></div><div class="wf-line2">` + strings.Join(items, `<span class="wf-sep"> | </span>`) + `</div></div>`
}

func expectedWorkflowSerial(c Company, rec Record, module string) string {
	jobRef := firstNonEmpty(rec.JobRef, rec.Fields["jobRef"])
	jobSeq := firstNonEmpty(rec.Fields["jobSequence"], sequenceFromJobRef(jobRef))
	if jobSeq == "" {
		jobSeq = "0000"
	}
	year := yearFromRecord(rec)
	moduleCode := firstNonEmpty(prefixes[module], strings.ToUpper(module[:min(3, len(module))]))
	prefix := firstNonEmpty(c.Prefix, "ZE")
	return strings.ToUpper(fmt.Sprintf("%s-%s-%d-%s", prefix, moduleCode, year, jobSeq))
}

func yearFromRecord(rec Record) int {
	for _, v := range []string{rec.Fields["date"], rec.Fields["invoiceDate"], rec.CreatedAt, rec.UpdatedAt} {
		v = strings.TrimSpace(v)
		if len(v) >= 4 {
			if y, err := strconv.Atoi(v[:4]); err == nil && y > 1900 && y < 3000 {
				return y
			}
		}
	}
	return time.Now().Year()
}

func verificationURL(c Company, rec Record, docNo string) string {
	base := normalizeVerificationBase(firstNonEmpty(c.VerificationBaseURL, c.Website, "https://www.zenitheclipse.com/verify"))
	serial := firstNonEmpty(docNo, rec.Number, rec.ID)
	escaped := url.PathEscape(serial)
	if strings.HasSuffix(base, "/") {
		return base + escaped
	}
	return base + "/" + escaped
}

func normalizeVerificationBase(base string) string {
	base = strings.TrimSpace(base)
	if base == "" {
		return "https://www.zenitheclipse.com/verify"
	}
	base = strings.TrimRight(base, "/")
	lower := strings.ToLower(base)
	if strings.HasPrefix(lower, "http://") {
		base = "https://" + strings.TrimPrefix(base, "http://")
	}
	if !strings.HasPrefix(strings.ToLower(base), "http://") && !strings.HasPrefix(strings.ToLower(base), "https://") {
		base = "https://" + base
	}
	lower = strings.ToLower(base)
	if !strings.HasSuffix(lower, "/verify") {
		base += "/verify"
	}
	return base
}

func compactWorkflowAllowed(rec Record) bool {
	v := strings.ToLower(strings.TrimSpace(firstNonEmpty(rec.Fields["showWorkflowInFooter"], rec.Fields["showWorkflow"], rec.Fields["includeWorkflow"])))
	if v == "hide" || v == "hidden" || v == "no" || v == "false" || v == "0" {
		return false
	}
	switch rec.Module {
	case "quotation", "proforma_invoice", "sales_invoice", "commercial_invoice", "packing_list":
		return true
	}
	if v == "yes" || v == "show" || v == "include" || v == "true" || v == "1" {
		return true
	}
	// If a legal/letter document is linked to a job reference, keep the footer workflow visible
	// unless the user explicitly hides it. This prevents workflow tracking disappearing.
	return strings.TrimSpace(firstNonEmpty(rec.JobRef, rec.Fields["jobRef"])) != ""
}

func pseudoQRCodeDataURI(text string) string {
	if strings.TrimSpace(text) == "" {
		text = "ZENITH ECLIPSE DOCUMENT"
	}
	payload := strings.TrimSpace(text)
	matrix := qrMatrix(payload)
	cell := 4
	quiet := 4 * cell
	size := len(matrix)
	full := size*cell + quiet*2
	var sb strings.Builder
	sb.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" width="`)
	sb.WriteString(strconv.Itoa(full))
	sb.WriteString(`" height="`)
	sb.WriteString(strconv.Itoa(full))
	sb.WriteString(`" viewBox="0 0 `)
	sb.WriteString(strconv.Itoa(full))
	sb.WriteString(` `)
	sb.WriteString(strconv.Itoa(full))
	sb.WriteString(`" shape-rendering="crispEdges"><rect width="100%" height="100%" fill="white"/>`)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if matrix[y][x] {
				sx := quiet + x*cell
				sy := quiet + y*cell
				sb.WriteString(`<rect x="`)
				sb.WriteString(strconv.Itoa(sx))
				sb.WriteString(`" y="`)
				sb.WriteString(strconv.Itoa(sy))
				sb.WriteString(`" width="`)
				sb.WriteString(strconv.Itoa(cell))
				sb.WriteString(`" height="`)
				sb.WriteString(strconv.Itoa(cell))
				sb.WriteString(`" fill="#111827"/>`)
			}
		}
	}
	sb.WriteString(`</svg>`)
	return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(sb.String()))
}

func qrMatrix(text string) [][]bool {
	const version = 4
	const dataCodewords = 80
	const ecCodewords = 20
	size := 17 + version*4
	matrix := make([][]int, size)
	reserved := make([][]bool, size)
	for y := 0; y < size; y++ {
		matrix[y] = make([]int, size)
		reserved[y] = make([]bool, size)
		for x := 0; x < size; x++ {
			matrix[y][x] = -1
		}
	}
	set := func(x, y int, black bool) {
		if x < 0 || y < 0 || x >= size || y >= size {
			return
		}
		if black {
			matrix[y][x] = 1
		} else {
			matrix[y][x] = 0
		}
		reserved[y][x] = true
	}
	drawFinder := func(x0, y0 int) {
		for dy := -1; dy <= 7; dy++ {
			for dx := -1; dx <= 7; dx++ {
				x, y := x0+dx, y0+dy
				if x < 0 || y < 0 || x >= size || y >= size {
					continue
				}
				black := false
				if dx >= 0 && dx <= 6 && dy >= 0 && dy <= 6 {
					black = dx == 0 || dx == 6 || dy == 0 || dy == 6 || (dx >= 2 && dx <= 4 && dy >= 2 && dy <= 4)
				}
				set(x, y, black)
			}
		}
	}
	drawFinder(0, 0)
	drawFinder(size-7, 0)
	drawFinder(0, size-7)
	for i := 8; i < size-8; i++ {
		set(i, 6, i%2 == 0)
		set(6, i, i%2 == 0)
	}
	drawAlignment := func(cx, cy int) {
		for dy := -2; dy <= 2; dy++ {
			for dx := -2; dx <= 2; dx++ {
				d := max(abs(dx), abs(dy))
				set(cx+dx, cy+dy, d == 2 || d == 0)
			}
		}
	}
	drawAlignment(26, 26)
	set(8, 4*version+9, true)
	reserveFormat := func() {
		for i := 0; i <= 5; i++ {
			set(8, i, false)
		}
		set(8, 7, false)
		set(8, 8, false)
		set(7, 8, false)
		for i := 9; i < 15; i++ {
			set(14-i, 8, false)
		}
		for i := 0; i < 8; i++ {
			set(size-1-i, 8, false)
		}
		for i := 8; i < 15; i++ {
			set(8, size-15+i, false)
		}
	}
	reserveFormat()

	data := qrDataCodewords(text, dataCodewords)
	ec := qrReedSolomon(data, ecCodewords)
	all := append(data, ec...)
	bits := make([]bool, 0, len(all)*8)
	for _, cw := range all {
		for i := 7; i >= 0; i-- {
			bits = append(bits, ((cw>>uint(i))&1) != 0)
		}
	}
	bitIndex := 0
	dir := -1
	row := size - 1
	for col := size - 1; col > 0; col -= 2 {
		if col == 6 {
			col--
		}
		for {
			for c := 0; c < 2; c++ {
				x := col - c
				if !reserved[row][x] {
					black := false
					if bitIndex < len(bits) {
						black = bits[bitIndex]
					}
					if (row+x)%2 == 0 { // mask pattern 0
						black = !black
					}
					if black {
						matrix[row][x] = 1
					} else {
						matrix[row][x] = 0
					}
					bitIndex++
				}
			}
			row += dir
			if row < 0 || row >= size {
				row -= dir
				dir = -dir
				break
			}
		}
	}
	format := qrFormatBits(1, 0) // level L, mask 0
	bit := func(i int) bool { return ((format >> uint(i)) & 1) != 0 }
	for i := 0; i <= 5; i++ {
		set(8, i, bit(i))
	}
	set(8, 7, bit(6))
	set(8, 8, bit(7))
	set(7, 8, bit(8))
	for i := 9; i < 15; i++ {
		set(14-i, 8, bit(i))
	}
	for i := 0; i < 8; i++ {
		set(size-1-i, 8, bit(i))
	}
	for i := 8; i < 15; i++ {
		set(8, size-15+i, bit(i))
	}

	out := make([][]bool, size)
	for y := 0; y < size; y++ {
		out[y] = make([]bool, size)
		for x := 0; x < size; x++ {
			out[y][x] = matrix[y][x] == 1
		}
	}
	return out
}

func qrDataCodewords(text string, dataCodewords int) []byte {
	b := []byte(text)
	maxBytes := dataCodewords - 2
	if len(b) > maxBytes {
		b = b[:maxBytes]
	}
	buf := &qrBitBuffer{}
	buf.appendBits(0x4, 4) // byte mode
	buf.appendBits(len(b), 8)
	for _, ch := range b {
		buf.appendBits(int(ch), 8)
	}
	remaining := dataCodewords*8 - len(buf.bits)
	if remaining > 4 {
		remaining = 4
	}
	buf.appendBits(0, remaining)
	for len(buf.bits)%8 != 0 {
		buf.appendBits(0, 1)
	}
	out := buf.bytes()
	pads := []byte{0xEC, 0x11}
	for len(out) < dataCodewords {
		out = append(out, pads[len(out)%2])
	}
	return out
}

type qrBitBuffer struct{ bits []bool }

func (b *qrBitBuffer) appendBits(val int, n int) {
	for i := n - 1; i >= 0; i-- {
		b.bits = append(b.bits, ((val>>uint(i))&1) != 0)
	}
}

func (b *qrBitBuffer) bytes() []byte {
	out := make([]byte, (len(b.bits)+7)/8)
	for i, bit := range b.bits {
		if bit {
			out[i/8] |= 1 << uint(7-i%8)
		}
	}
	return out
}

func qrReedSolomon(data []byte, ecLen int) []byte {
	exp, logt := qrGFTable()
	mul := func(x, y int) int {
		if x == 0 || y == 0 {
			return 0
		}
		return exp[logt[x]+logt[y]]
	}
	gen := []int{1}
	for i := 0; i < ecLen; i++ {
		next := make([]int, len(gen)+1)
		for j, coef := range gen {
			next[j] ^= coef
			next[j+1] ^= mul(coef, exp[i])
		}
		gen = next
	}
	rem := make([]int, ecLen)
	for _, d := range data {
		factor := int(d) ^ rem[0]
		copy(rem, rem[1:])
		rem[ecLen-1] = 0
		for j := 0; j < ecLen; j++ {
			rem[j] ^= mul(gen[j+1], factor)
		}
	}
	out := make([]byte, ecLen)
	for i := range rem {
		out[i] = byte(rem[i])
	}
	return out
}

func qrGFTable() ([512]int, [256]int) {
	var exp [512]int
	var logt [256]int
	x := 1
	for i := 0; i < 255; i++ {
		exp[i] = x
		logt[x] = i
		x <<= 1
		if x&0x100 != 0 {
			x ^= 0x11D
		}
	}
	for i := 255; i < 512; i++ {
		exp[i] = exp[i-255]
	}
	return exp, logt
}

func qrFormatBits(ecLevelBits, mask int) int {
	data := (ecLevelBits << 3) | mask
	v := data << 10
	poly := 0x537
	for qrBitLen(v)-qrBitLen(poly) >= 0 {
		v ^= poly << uint(qrBitLen(v)-qrBitLen(poly))
	}
	return ((data << 10) | v) ^ 0x5412
}

func qrBitLen(x int) int {
	n := 0
	for x != 0 {
		n++
		x >>= 1
	}
	return n
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
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
	case "handover_sheet":
		return "HANDOVER SHEET"
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
				total := floatAny(it["total"], it["amount"], it["lineTotal"], it["itemTotal"])
				if qty != 0 && price != 0 {
					total = qty * price
				}
				lines = append(lines, docLine{Kind: firstStringAny(it["type"], it["itemKind"], it["category"]), Description: firstStringAny(it["description"], it["name"], it["productDescription"]), HSCode: firstStringAny(it["hsCode"], it["HSCode"]), Unit: firstStringAny(it["unit"], it["uom"]), Qty: qty, UnitPrice: price, Total: total, NetWeight: floatAny(it["netWeight"]), GrossWeight: floatAny(it["grossWeight"]), Packages: floatAny(it["packages"], it["cartons"])})
			}
		}
	}
	if len(lines) == 0 {
		qty := numField(rec.Fields, "quantity", "qty")
		price := numField(rec.Fields, "unitPrice", "price", "rate")
		total := numField(rec.Fields, "lineTotal", "itemTotal")
		if qty == 0 {
			qty = 1
		}
		if qty != 0 && price != 0 {
			total = qty * price
		} else if total == 0 {
			total = numField(rec.Fields, "amount", "total", "saleAmount")
			if price == 0 && total != 0 {
				price = total / qty
			}
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
	if v := numField(rec.Fields, "productsTotal"); v != 0 && products == 0 {
		products = v
	}
	transport += numField(rec.Fields, "transportationCharges", "transportCharges", "transportCharge", "freightCharges", "shipping")
	if v := numField(rec.Fields, "transportTotal"); v != 0 && transport == 0 {
		transport = v
	}
	services += numField(rec.Fields, "serviceCharges", "servicesCharges", "otherCharges", "extraCharges")
	if v := numField(rec.Fields, "servicesTotal"); v != 0 && services == 0 {
		services = v
	}
	subtotal = products + transport + services
	if subtotal == 0 {
		subtotal = numField(rec.Fields, "subtotal", "amount", "saleAmount")
	}
	discount = numField(rec.Fields, "discount")
	tax = numField(rec.Fields, "tax", "taxVat", "vat")
	if tax == 0 && numField(rec.Fields, "taxRate", "vatRate") != 0 {
		tax = (subtotal - discount) * numField(rec.Fields, "taxRate", "vatRate") / 100
	}
	total = subtotal - discount + tax
	if total < 0 {
		total = 0
	}
	return
}

func renderItemsTable(lines []docLine, showPrices bool, lang string) string {
	esc := template.HTMLEscapeString
	var b strings.Builder
	if showPrices {
		b.WriteString(`<table class="doc-table product-table price-table"><colgroup><col style="width:4%"><col style="width:30%"><col style="width:8%"><col style="width:7%"><col style="width:7%"><col style="width:11%"><col style="width:11%"><col style="width:8%"><col style="width:8%"><col style="width:6%"></colgroup><thead><tr><th>#</th><th>Description</th><th>HS Code</th><th>Unit</th><th>Qty</th><th>Unit Price</th><th>Total</th><th>Net Wt</th><th>Gross Wt</th><th>Packages</th></tr></thead><tbody>`)
		for i, l := range lines {
			b.WriteString(`<tr><td>` + fmt.Sprintf("%d", i+1) + `</td><td class="desc-cell">` + esc(l.Description) + `</td><td>` + esc(l.HSCode) + `</td><td>` + esc(l.Unit) + `</td><td class="num">` + formatQty(l.Qty) + `</td><td class="num">` + formatMoney(l.UnitPrice) + `</td><td class="num">` + formatMoney(l.Total) + `</td><td class="num">` + formatQty(l.NetWeight) + `</td><td class="num">` + formatQty(l.GrossWeight) + `</td><td class="num">` + formatQty(l.Packages) + `</td></tr>`)
		}
		b.WriteString(`</tbody></table>`)
		return b.String()
	}
	b.WriteString(`<table class="doc-table product-table logistics-table"><colgroup><col style="width:5%"><col style="width:38%"><col style="width:10%"><col style="width:8%"><col style="width:9%"><col style="width:10%"><col style="width:10%"><col style="width:10%"></colgroup><thead><tr><th>#</th><th>Description</th><th>HS Code</th><th>Unit</th><th>Qty</th><th>Net Wt</th><th>Gross Wt</th><th>Packages</th></tr></thead><tbody>`)
	for i, l := range lines {
		b.WriteString(`<tr><td>` + fmt.Sprintf("%d", i+1) + `</td><td class="desc-cell">` + esc(l.Description) + `</td><td>` + esc(l.HSCode) + `</td><td>` + esc(l.Unit) + `</td><td class="num">` + formatQty(l.Qty) + `</td><td class="num">` + formatQty(l.NetWeight) + `</td><td class="num">` + formatQty(l.GrossWeight) + `</td><td class="num">` + formatQty(l.Packages) + `</td></tr>`)
	}
	b.WriteString(`</tbody></table>`)
	return b.String()
}

func showPriceTableForDocument(rec Record) bool {
	switch rec.Module {
	case "quotation", "proforma_invoice", "sales_invoice", "commercial_invoice", "sales_order", "purchase_order", "receipt_voucher", "payment_voucher":
		return true
	}
	if logisticsNoPriceModule(rec.Module) {
		v := strings.ToLower(strings.TrimSpace(firstNonEmpty(rec.Fields["priceTableOption"], rec.Fields["showPriceTable"], rec.Fields["includePrices"], rec.Fields["includePriceTable"])))
		return v == "show price table" || v == "show" || v == "yes" || v == "true" || v == "1" || v == "enabled"
	}
	return true
}

func logisticsNoPriceModule(module string) bool {
	switch module {
	case "bill_of_lading", "packing_list", "delivery_note", "handover_sheet", "warehouse_document", "shipping_document":
		return true
	}
	return false
}

func renderLogisticsSummaryHTML(rec Record, lines []docLine, lang string) string {
	esc := template.HTMLEscapeString
	rows := [][2]string{
		{"Container No.", firstNonEmpty(rec.Fields["containerNumber"], rec.Fields["containerNo"])},
		{"Seal No.", firstNonEmpty(rec.Fields["sealNumber"], rec.Fields["sealNo"])},
		{"B/L No.", firstNonEmpty(rec.Fields["blNo"], rec.Fields["blNumber"])},
		{"Vessel / Voyage", rec.Fields["vesselVoyage"]},
		{"Loading Port", firstNonEmpty(rec.Fields["pol"], rec.Fields["loadingLocation"])},
		{"Discharge / Delivery", firstNonEmpty(rec.Fields["pod"], rec.Fields["deliveryLocation"])},
		{"Consignee", rec.Fields["consignee"]},
		{"Shipper", rec.Fields["shipper"]},
		{"Notify Party", rec.Fields["notifyParty"]},
		{"Receiver", rec.Fields["receiverName"]},
	}
	var qty, net, gross, packages float64
	for _, l := range lines {
		qty += l.Qty
		net += l.NetWeight
		gross += l.GrossWeight
		packages += l.Packages
	}
	if qty != 0 {
		rows = append(rows, [2]string{"Total Quantity", formatQty(qty)})
	}
	if net != 0 {
		rows = append(rows, [2]string{"Total Net Weight", formatQty(net)})
	}
	if gross != 0 {
		rows = append(rows, [2]string{"Total Gross Weight", formatQty(gross)})
	}
	if packages != 0 {
		rows = append(rows, [2]string{"Total Packages", formatQty(packages)})
	}
	var b strings.Builder
	b.WriteString(`<table class="totals logistics-summary"><tr class="grand"><td colspan="2">` + tr(lang, "logisticsDetails") + `</td></tr>`)
	count := 0
	for _, r := range rows {
		if strings.TrimSpace(r[1]) != "" && strings.TrimSpace(r[1]) != "→" {
			b.WriteString(`<tr><td>` + esc(r[0]) + `</td><td>` + esc(r[1]) + `</td></tr>`)
			count++
		}
	}
	if count == 0 {
		b.WriteString(`<tr><td colspan="2">No shipping details entered yet.</td></tr>`)
	}
	b.WriteString(`</table>`)
	return b.String()
}

func renderTotalsTable(currency string, products, transport, services, subtotal, discount, tax, total float64, lang string) string {
	esc := template.HTMLEscapeString
	rows := [][2]string{}
	if transport != 0 {
		rows = append(rows, [2]string{tr(lang, "transportationCharges"), formatMoney(transport) + " " + currency})
	}
	if services != 0 {
		rows = append(rows, [2]string{tr(lang, "serviceCharges"), formatMoney(services) + " " + currency})
	}
	rows = append(rows, [2]string{tr(lang, "subtotal"), formatMoney(subtotal) + " " + currency})
	if discount != 0 {
		rows = append(rows, [2]string{tr(lang, "discount"), formatMoney(discount) + " " + currency})
	}
	rows = append(rows, [2]string{tr(lang, "vatTax"), formatMoney(tax) + " " + currency})
	var b strings.Builder
	b.WriteString(`<table class="totals compact-summary">`)
	for _, r := range rows {
		b.WriteString(`<tr><td>` + esc(r[0]) + `</td><td>` + esc(r[1]) + `</td></tr>`)
	}
	b.WriteString(`<tr class="grand"><td>` + tr(lang, "total") + `</td><td>` + esc(formatMoney(total)+" "+currency) + `</td></tr></table>`)
	return b.String()
}

func renderDocxBytes(c Company, rec Record, all []Record) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name, content string) error {
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		_, err = io.WriteString(w, content)
		return err
	}
	addBytes := func(name string, b []byte) error {
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		_, err = w.Write(b)
		return err
	}
	verification := firstNonEmpty(rec.Fields["verificationCode"], rec.Fields["verification"], strings.ToUpper(rec.ID[:min(12, len(rec.ID))]))
	verifyURL := verificationURL(c, rec, rec.Number)
	qrPNG, _ := pseudoQRCodePNGBytes(verifyURL)
	if err := add("[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Default Extension="png" ContentType="image/png"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/><Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/><Override PartName="/word/header1.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.header+xml"/><Override PartName="/word/footer1.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.footer+xml"/></Types>`); err != nil {
		return nil, err
	}
	if err := add("_rels/.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`); err != nil {
		return nil, err
	}
	if err := add("word/_rels/document.xml.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rIdHeader1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/header" Target="header1.xml"/><Relationship Id="rIdFooter1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/footer" Target="footer1.xml"/></Relationships>`); err != nil {
		return nil, err
	}
	if err := add("word/_rels/footer1.xml.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rIdQr" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/qr.png"/></Relationships>`); err != nil {
		return nil, err
	}
	if err := add("word/styles.xml", docxStylesXML()); err != nil {
		return nil, err
	}
	if err := add("word/header1.xml", docxHeaderXML(c, rec)); err != nil {
		return nil, err
	}
	if err := add("word/footer1.xml", docxFooterXML(c, rec, all, verification)); err != nil {
		return nil, err
	}
	if err := addBytes("word/media/qr.png", qrPNG); err != nil {
		return nil, err
	}
	if err := add("word/document.xml", docxDocumentXML(c, rec, all, verification)); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func docxStylesXML() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/><w:rPr><w:rFonts w:ascii="Arial" w:hAnsi="Arial"/><w:sz w:val="20"/></w:rPr></w:style></w:styles>`
}

func docxHeaderXML(c Company, rec Record) string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:hdr xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		docxP(firstNonEmpty(c.Name, "ZENITH ECLIPSE CO"), true, 28, "003366", "left") +
		docxP(c.Slogan, false, 12, "003366", "left") +
		docxP(docTitle(rec.Module)+" | "+rec.Number+" | Status: "+firstNonEmpty(rec.Status, "Draft"), true, 14, "0F172A", "right") +
		`</w:hdr>`
}

func docxFooterXML(c Company, rec Record, all []Record, verification string) string {
	workflow := docxWorkflowLine(c, rec, all)
	address := firstNonEmpty(c.Address, c.City+", "+c.Country)
	emailWeb := normalizeEmailValue(c.Email)
	if strings.TrimSpace(c.Website) != "" {
		emailWeb += "\n" + strings.TrimSpace(c.Website)
	}
	phone := singlePhone(c)
	qrCell := `<w:tc><w:tcPr><w:tcW w:w="2200" w:type="dxa"/><w:tcMar><w:top w:w="60" w:type="dxa"/><w:left w:w="70" w:type="dxa"/><w:bottom w:w="60" w:type="dxa"/><w:right w:w="70" w:type="dxa"/></w:tcMar></w:tcPr><w:p><w:pPr><w:jc w:val="center"/></w:pPr><w:r>` + docxImageRun("rIdQr", 560000, 560000) + `</w:r></w:p>` + docxP("QR Verify", true, 12, "003366", "center") + docxP("Scan to verify", false, 10, "334155", "center") + docxP(rec.Number, false, 10, "334155", "center") + `</w:tc>`
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:ftr xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture"><w:tbl><w:tblPr><w:tblW w:w="10000" w:type="dxa"/><w:tblBorders><w:top w:val="single" w:sz="4" w:color="E7EFF7"/><w:left w:val="nil"/><w:bottom w:val="single" w:sz="4" w:color="E7EFF7"/><w:right w:val="nil"/><w:insideH w:val="single" w:sz="2" w:color="EEF2F7"/><w:insideV w:val="single" w:sz="2" w:color="EEF2F7"/></w:tblBorders></w:tblPr><w:tr>` +
		docxCellText("Address\n"+address, true, 16, "334155", "FFFFFF", 3600) +
		docxCellText("Email / Web\n"+emailWeb, true, 16, "334155", "FFFFFF", 2600) +
		docxCellText("Phone\n"+phone, true, 16, "334155", "FFFFFF", 1600) + qrCell + `</w:tr><w:tr>` + docxCellSpanText(workflow, 4, true, 10, "334155", "FFFFFF") + `</w:tr></w:tbl></w:ftr>`
}

func docxDocumentXML(c Company, rec Record, all []Record, verification string) string {
	lang := docLanguage(rec)
	var body strings.Builder
	if rec.Module == "contract" || rec.Module == "letterhead" {
		title := firstNonEmpty(rec.Fields["contractTitle"], rec.Fields["title"], docTitle(rec.Module))
		body.WriteString(docxP(title, true, 24, "003366", "center"))
		body.WriteString(docxP("Subject: "+firstNonEmpty(rec.Fields["subject"], title), false, 18, "0F172A", "left"))
		body.WriteString(docxP("Date: "+formatDocDate(firstNonEmpty(rec.Fields["date"], rec.CreatedAt))+"    Ref: "+rec.Number, false, 16, "334155", "left"))
		partyLines := docxPartyLines(rec)
		if partyLines != "" {
			body.WriteString(docxP("Party / Recipient", true, 16, "003366", "left"))
			body.WriteString(docxLongText(partyLines))
		}
		body.WriteString(docxLongText(firstNonEmpty(rec.Fields["contractBody"], rec.Fields["body"], rec.Fields["remarks"])))
		if optionVisible(firstNonEmpty(rec.Fields["stampOption"], rec.Fields["signatureOption"], "Placeholder")) {
			body.WriteString(docxP("SIGNATURE / STAMP", true, 16, "003366", "right"))
			body.WriteString(docxP(firstNonEmpty(rec.Fields["signatureName"], "Authorized Signature"), false, 16, "0F172A", "right"))
		}
	} else {
		currency := firstNonEmpty(rec.Fields["currency"], c.BaseCurrency, "USD")
		lines := lineItemsFromRecord(rec)
		_, transport, services, subtotal, discount, tax, total := totalsByKind(rec, lines)
		showPrices := showPriceTableForDocument(rec)
		body.WriteString(docxP(docTitle(rec.Module), true, 24, "003366", "center"))
		body.WriteString(docxP("Document No: "+rec.Number+"    Date: "+formatDocDate(firstNonEmpty(rec.Fields["invoiceDate"], rec.Fields["date"], rec.CreatedAt))+"    Status: "+rec.Status, true, 16, "0F172A", "right"))
		body.WriteString(docxP("Job Reference: "+firstNonEmpty(rec.JobRef, rec.Fields["jobRef"], "-"), false, 15, "334155", "right"))
		body.WriteString(docxTwoColumnInfoTable(docxCustomerLines(rec), docxDocumentDetailsLines(rec, currency)))
		body.WriteString(docxItemsTable(lines, showPrices))
		if showPrices {
			summary := [][]string{{tr(lang, "subtotal"), formatMoney(subtotal) + " " + currency}}
			if transport != 0 {
				summary = append([][]string{{tr(lang, "transportationCharges"), formatMoney(transport) + " " + currency}}, summary...)
			}
			if services != 0 {
				summary = append([][]string{{tr(lang, "serviceCharges"), formatMoney(services) + " " + currency}}, summary...)
			}
			if discount != 0 {
				summary = append(summary, []string{tr(lang, "discount"), formatMoney(discount) + " " + currency})
			}
			summary = append(summary, []string{tr(lang, "vatTax"), formatMoney(tax) + " " + currency}, []string{"Total", formatMoney(total) + " " + currency})
			body.WriteString(docxP("Summary", true, 16, "003366", "left"))
			body.WriteString(docxTable(summary, false))
		} else {
			logistics := docxLogisticsLines(rec, lines)
			if logistics != "" {
				body.WriteString(docxP("Logistics / Shipping Details", true, 16, "003366", "left"))
				body.WriteString(docxLongText(logistics))
			}
		}
		notes := firstNonEmpty(rec.Fields["notes"], rec.Fields["remarks"])
		notesOpt := strings.ToLower(firstNonEmpty(rec.Fields["notesOption"], rec.Fields["showNotes"], "Hide"))
		if strings.TrimSpace(notes) != "" && notesOpt != "hide" && notesOpt != "remove" && notesOpt != "no" {
			body.WriteString(docxP("Note", true, 16, "003366", "left"))
			body.WriteString(docxLongText(notes))
		}
		terms := docxTermsText(c, rec, all)
		if terms != "" {
			body.WriteString(docxP("Terms & Conditions", true, 16, "003366", "left"))
			body.WriteString(docxLongText(terms))
		}
		bank := plainSelectedBankDetails(c, rec, all)
		if bank != "" {
			body.WriteString(docxP("Bank Details", true, 16, "003366", "left"))
			body.WriteString(docxLongText(bank))
		}
		if optionVisible(firstNonEmpty(rec.Fields["stampOption"], rec.Fields["signatureOption"], rec.Fields["companyLabelOption"], "Hide")) {
			body.WriteString(docxP("SIGNATURE / STAMP", true, 15, "003366", "right"))
		}
	}
	body.WriteString(docxP("Verification: "+verification, false, 14, "334155", "left"))
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><w:body>` + body.String() + docxSectPr() + `</w:body></w:document>`
}

func docxSectPr() string {
	return `<w:sectPr><w:headerReference w:type="default" r:id="rIdHeader1"/><w:footerReference w:type="default" r:id="rIdFooter1"/><w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="900" w:right="700" w:bottom="980" w:left="700" w:header="360" w:footer="360" w:gutter="0"/><w:pgBorders w:offsetFrom="page"><w:top w:val="single" w:sz="18" w:space="12" w:color="003366"/><w:left w:val="single" w:sz="18" w:space="12" w:color="003366"/><w:bottom w:val="single" w:sz="18" w:space="12" w:color="003366"/><w:right w:val="single" w:sz="18" w:space="12" w:color="003366"/></w:pgBorders></w:sectPr>`
}

func docxP(text string, bold bool, size int, colorHex, align string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	if colorHex == "" {
		colorHex = "0F172A"
	}
	jc := ""
	if align != "" {
		jc = `<w:pPr><w:jc w:val="` + xmlEscape(align) + `"/></w:pPr>`
	}
	b := ""
	if bold {
		b = `<w:b/>`
	}
	return `<w:p>` + jc + `<w:r><w:rPr>` + b + `<w:color w:val="` + colorHex + `"/><w:sz w:val="` + strconv.Itoa(size) + `"/></w:rPr><w:t xml:space="preserve">` + xmlEscape(text) + `</w:t></w:r></w:p>`
}

func docxLongText(text string) string {
	var b strings.Builder
	paras := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for _, p := range paras {
		if strings.TrimSpace(p) == "" {
			b.WriteString(`<w:p/>`)
			continue
		}
		b.WriteString(docxP(p, false, 20, "172B44", "left"))
	}
	return b.String()
}

func docxItemsTable(lines []docLine, showPrices bool) string {
	rows := [][]string{}
	if showPrices {
		rows = append(rows, []string{"#", "Description", "HS Code", "Unit", "Qty", "Unit Price", "Total", "Net Wt", "Gross Wt", "Packages"})
	} else {
		rows = append(rows, []string{"#", "Description", "HS Code", "Unit", "Qty", "Net Wt", "Gross Wt", "Packages"})
	}
	for i, l := range lines {
		if showPrices {
			rows = append(rows, []string{strconv.Itoa(i + 1), l.Description, l.HSCode, l.Unit, formatQty(l.Qty), formatMoney(l.UnitPrice), formatMoney(l.Total), formatQty(l.NetWeight), formatQty(l.GrossWeight), formatQty(l.Packages)})
		} else {
			rows = append(rows, []string{strconv.Itoa(i + 1), l.Description, l.HSCode, l.Unit, formatQty(l.Qty), formatQty(l.NetWeight), formatQty(l.GrossWeight), formatQty(l.Packages)})
		}
	}
	return docxTable(rows, true)
}

func docxTable(rows [][]string, header bool) string {
	var b strings.Builder
	b.WriteString(`<w:tbl><w:tblPr><w:tblW w:w="0" w:type="auto"/><w:tblBorders><w:top w:val="single" w:sz="4" w:color="DCE8F5"/><w:left w:val="single" w:sz="4" w:color="DCE8F5"/><w:bottom w:val="single" w:sz="4" w:color="DCE8F5"/><w:right w:val="single" w:sz="4" w:color="DCE8F5"/><w:insideH w:val="single" w:sz="4" w:color="DCE8F5"/><w:insideV w:val="single" w:sz="4" w:color="DCE8F5"/></w:tblBorders></w:tblPr>`)
	for i, row := range rows {
		b.WriteString(`<w:tr>`)
		for _, cell := range row {
			fill := "FFFFFF"
			bold := false
			color := "0F172A"
			if header && i == 0 {
				fill = "003366"
				bold = true
				color = "FFFFFF"
			}
			b.WriteString(docxCellText(cell, bold, 15, color, fill, 0))
		}
		b.WriteString(`</w:tr>`)
	}
	b.WriteString(`</w:tbl>`)
	return b.String()
}

func docxCellText(text string, bold bool, size int, color, fill string, width int) string {
	if fill == "" {
		fill = "FFFFFF"
	}
	w := ""
	if width > 0 {
		w = `<w:tcW w:w="` + strconv.Itoa(width) + `" w:type="dxa"/>`
	}
	var b strings.Builder
	b.WriteString(`<w:tc><w:tcPr>` + w + `<w:shd w:fill="` + fill + `"/><w:tcMar><w:top w:w="70" w:type="dxa"/><w:left w:w="80" w:type="dxa"/><w:bottom w:w="70" w:type="dxa"/><w:right w:w="80" w:type="dxa"/></w:tcMar></w:tcPr>`)
	parts := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	wrote := false
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		lineBold := bold && i == 0
		lineColor := color
		b.WriteString(docxP(part, lineBold, size, lineColor, "left"))
		wrote = true
	}
	if !wrote {
		b.WriteString(docxP(" ", false, size, color, "left"))
	}
	b.WriteString(`</w:tc>`)
	return b.String()
}

func docxCellSpanText(text string, span int, bold bool, size int, color, fill string) string {
	if fill == "" {
		fill = "FFFFFF"
	}
	if span < 1 {
		span = 1
	}
	text = docxNoBreakWorkflow(text)
	return `<w:tc><w:tcPr><w:gridSpan w:val="` + strconv.Itoa(span) + `"/><w:noWrap/><w:shd w:fill="` + fill + `"/><w:tcMar><w:top w:w="40" w:type="dxa"/><w:left w:w="80" w:type="dxa"/><w:bottom w:w="40" w:type="dxa"/><w:right w:w="80" w:type="dxa"/></w:tcMar></w:tcPr>` + docxP(text, bold, size, color, "left") + `</w:tc>`
}

func docxTwoColumnInfoTable(left, right string) string {
	if strings.TrimSpace(left) == "" {
		left = "Customer / Buyer not set"
	}
	if strings.TrimSpace(right) == "" {
		right = "Document details not set"
	}
	return `<w:tbl><w:tblPr><w:tblW w:w="10000" w:type="dxa"/><w:tblBorders><w:top w:val="single" w:sz="4" w:color="DCE8F5"/><w:left w:val="single" w:sz="4" w:color="DCE8F5"/><w:bottom w:val="single" w:sz="4" w:color="DCE8F5"/><w:right w:val="single" w:sz="4" w:color="DCE8F5"/><w:insideV w:val="single" w:sz="4" w:color="DCE8F5"/></w:tblBorders></w:tblPr><w:tr>` + docxCellText("BUYER / CUSTOMER\n"+left, true, 15, "0F172A", "FFFFFF", 5000) + docxCellText("PRODUCT / SERVICE / TRANSPORTATION DETAILS\n"+right, true, 15, "0F172A", "FFFFFF", 5000) + `</w:tr></w:tbl>`
}

func docxCustomerLines(rec Record) string {
	rows := [][2]string{
		{"Name", firstNonEmpty(rec.Fields["customer"], rec.Fields["customerName"], rec.Fields["buyer"], rec.Fields["name"])},
		{"Contact", firstNonEmpty(rec.Fields["contactPerson"], rec.Fields["contact"], rec.Fields["receiverName"])},
		{tr(docLanguage(rec), "address"), firstNonEmpty(rec.Fields["customerAddress"], rec.Fields["address"], rec.Fields["toAddress"], rec.Fields["partyAddress"])},
		{"Email", normalizeEmailValue(rec.Fields["customerEmail"])},
		{"Phone", firstNonEmpty(rec.Fields["customerPhone"], rec.Fields["phone"], rec.Fields["mobile"])},
		{"TRN/VAT", firstNonEmpty(rec.Fields["customerTaxNumber"], rec.Fields["taxNumber"], rec.Fields["trn"], rec.Fields["vatNumber"])},
	}
	return docxKVLines(rows)
}

func docxDocumentDetailsLines(rec Record, currency string) string {
	lang := docLanguage(rec)
	rows := [][2]string{{tr(lang, "transactionType"), firstNonEmpty(rec.Fields["transactionType"], rec.Fields["dealMode"])}, {tr(lang, "currency"), currency}, {"Incoterm", rec.Fields["incoterm"]}}
	for _, kv := range [][2]string{{"Product/Goods", firstNonEmpty(rec.Fields["productDescription"], rec.Fields["description"])}, {"HS Code", rec.Fields["hsCode"]}, {"Quantity", rec.Fields["quantity"]}, {"Route", firstNonEmpty(rec.Fields["route"], strings.TrimSpace(rec.Fields["pol"]+" → "+rec.Fields["pod"]))}, {"Container", firstNonEmpty(rec.Fields["containerNumber"], rec.Fields["containerNo"])}, {"Seal", firstNonEmpty(rec.Fields["sealNumber"], rec.Fields["sealNo"])}, {"B/L No.", firstNonEmpty(rec.Fields["blNo"], rec.Fields["blNumber"])}, {"Vessel/Voyage", rec.Fields["vesselVoyage"]}, {"POL", rec.Fields["pol"]}, {"POD", rec.Fields["pod"]}, {"Delivery", rec.Fields["deliveryLocation"]}} {
		rows = append(rows, kv)
	}
	return docxKVLines(rows)
}

func docxPartyLines(rec Record) string {
	return docxKVLines([][2]string{{"Name", firstNonEmpty(rec.Fields["partyName"], rec.Fields["to"], rec.Fields["customer"], rec.Fields["customerName"])}, {"Contact", firstNonEmpty(rec.Fields["contactPerson"], rec.Fields["contact"])}, {tr(docLanguage(rec), "address"), firstNonEmpty(rec.Fields["toAddress"], rec.Fields["customerAddress"], rec.Fields["address"], rec.Fields["partyAddress"])}, {"Email", normalizeEmailValue(firstNonEmpty(rec.Fields["customerEmail"], rec.Fields["email"]))}, {"Phone", firstNonEmpty(rec.Fields["customerPhone"], rec.Fields["phone"])}})
}

func docxLogisticsLines(rec Record, lines []docLine) string {
	rows := [][2]string{{"Container No.", firstNonEmpty(rec.Fields["containerNumber"], rec.Fields["containerNo"])}, {"Seal No.", firstNonEmpty(rec.Fields["sealNumber"], rec.Fields["sealNo"])}, {"B/L No.", firstNonEmpty(rec.Fields["blNo"], rec.Fields["blNumber"])}, {"Vessel / Voyage", rec.Fields["vesselVoyage"]}, {"POL", rec.Fields["pol"]}, {"POD", rec.Fields["pod"]}, {"Shipper", rec.Fields["shipper"]}, {"Consignee", rec.Fields["consignee"]}, {"Notify Party", rec.Fields["notifyParty"]}, {"Receiver", rec.Fields["receiverName"]}}
	var qty, net, gross, packages float64
	for _, l := range lines {
		qty += l.Qty
		net += l.NetWeight
		gross += l.GrossWeight
		packages += l.Packages
	}
	if qty != 0 {
		rows = append(rows, [2]string{"Total Quantity", formatQty(qty)})
	}
	if net != 0 {
		rows = append(rows, [2]string{"Total Net Weight", formatQty(net)})
	}
	if gross != 0 {
		rows = append(rows, [2]string{"Total Gross Weight", formatQty(gross)})
	}
	if packages != 0 {
		rows = append(rows, [2]string{"Total Packages", formatQty(packages)})
	}
	return docxKVLines(rows)
}

func docxKVLines(rows [][2]string) string {
	parts := []string{}
	for _, r := range rows {
		if strings.TrimSpace(r[1]) != "" && strings.TrimSpace(r[1]) != "→" {
			parts = append(parts, r[0]+": "+strings.TrimSpace(r[1]))
		}
	}
	return strings.Join(parts, "\n")
}

func docxTermsText(c Company, rec Record, all []Record) string {
	defaultOpt := "Default Terms Template"
	if logisticsNoPriceModule(rec.Module) {
		defaultOpt = "Hide"
	}
	opt := strings.ToLower(firstNonEmpty(rec.Fields["termsOption"], defaultOpt))
	if strings.Contains(opt, "hide") || strings.Contains(opt, "remove") || opt == "no" {
		return ""
	}
	if tid := strings.TrimSpace(rec.Fields["termsTemplateId"]); tid != "" {
		for _, t := range all {
			if t.Module == "terms_template" && (t.ID == tid || t.Number == tid) {
				return firstNonEmpty(t.Fields["templateText"], t.Fields["terms"], t.Fields["body"], t.Fields["notes"])
			}
		}
	}
	if strings.Contains(opt, "custom") || strings.Contains(opt, "edit") {
		return firstNonEmpty(rec.Fields["terms"], rec.Fields["paymentTerms"], rec.Fields["termsTemplate"])
	}
	if strings.Contains(opt, "template") && strings.TrimSpace(rec.Fields["termsTemplate"]) != "" {
		return rec.Fields["termsTemplate"]
	}
	return firstNonEmpty(rec.Fields["terms"], rec.Fields["paymentTerms"], rec.Fields["termsTemplate"], c.DefaultTerms)
}

func docxWorkflowLine(c Company, rec Record, all []Record) string {
	if !compactWorkflowAllowed(rec) {
		return firstNonEmpty(c.Name, "ZENITH ECLIPSE CO")
	}
	jobRef := firstNonEmpty(rec.JobRef, rec.Fields["jobRef"], "-")
	steps := []struct{ Module, Code string }{{"quotation", "QT"}, {"proforma_invoice", "PI"}, {"sales_invoice", "SI"}, {"commercial_invoice", "CI"}, {"packing_list", "PL"}}
	blFound := rec.Module == "bill_of_lading"
	for _, r := range all {
		if r.JobRef == jobRef && r.Module == "bill_of_lading" {
			blFound = true
			break
		}
	}
	if blFound {
		steps = append(steps, struct{ Module, Code string }{"bill_of_lading", "BL"})
	}
	findLinked := func(module string) (Record, bool) {
		if rec.Module == module {
			return rec, true
		}
		if jobRef == "-" {
			return Record{}, false
		}
		for _, r := range all {
			if r.JobRef == jobRef && r.Module == module {
				return r, true
			}
		}
		return Record{}, false
	}
	parts := []string{"Workflow / Job Ref: " + jobRef + " / Status: " + firstNonEmpty(rec.Status, "Draft")}
	for _, step := range steps {
		serial := "Not Created"
		status := "Pending"
		if found, ok := findLinked(step.Module); ok {
			serial = found.Number
			status = firstNonEmpty(found.Status, "Draft")
		}
		parts = append(parts, step.Code+": "+serial+" - "+status)
	}
	return strings.Join(parts, " | ")
}

func docxNoBreakWorkflow(s string) string {
	return strings.ReplaceAll(s, "-", "‑")
}

func docxWorkflowText(c Company, rec Record, all []Record) string {
	return strings.ReplaceAll(docxWorkflowLine(c, rec, all), " | ", "\n")
}

func plainSelectedBankDetails(c Company, rec Record, all []Record) string {
	if strings.TrimSpace(documentBankHTML(c, rec, all)) == "" {
		return ""
	}
	bankID := strings.TrimSpace(firstNonEmpty(rec.Fields["bankAccountId"], rec.Fields["selectedBankAccount"], rec.Fields["bankId"]))
	var bank *Record
	for i := range all {
		if all[i].Module == "bank_account" && (all[i].ID == bankID || all[i].Number == bankID || strings.EqualFold(all[i].Fields["bankName"], bankID)) {
			bank = &all[i]
			break
		}
	}
	if bank == nil {
		return strings.TrimSpace(rec.Fields["bankDetails"])
	}
	parts := []string{}
	for _, kv := range [][2]string{{"Bank Name", bank.Fields["bankName"]}, {"Account Name", bank.Fields["accountName"]}, {"Account No", bank.Fields["accountNumber"]}, {"IBAN", bank.Fields["iban"]}, {"SWIFT", bank.Fields["swift"]}, {"Currency", bank.Fields["currency"]}} {
		if strings.TrimSpace(kv[1]) != "" {
			parts = append(parts, kv[0]+": "+strings.TrimSpace(kv[1]))
		}
	}
	return strings.Join(parts, "\n")
}

func docxImageRun(relID string, cx, cy int) string {
	return `<w:drawing><wp:inline distT="0" distB="0" distL="0" distR="0"><wp:extent cx="` + strconv.Itoa(cx) + `" cy="` + strconv.Itoa(cy) + `"/><wp:docPr id="1" name="QR Verification"/><a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:pic><pic:nvPicPr><pic:cNvPr id="0" name="qr.png"/><pic:cNvPicPr/></pic:nvPicPr><pic:blipFill><a:blip r:embed="` + relID + `"/><a:stretch><a:fillRect/></a:stretch></pic:blipFill><pic:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="` + strconv.Itoa(cx) + `" cy="` + strconv.Itoa(cy) + `"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></pic:spPr></pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing>`
}

func xmlEscape(s string) string {
	repl := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;")
	return repl.Replace(s)
}

func pseudoQRCodePNGBytes(text string) ([]byte, error) {
	if strings.TrimSpace(text) == "" {
		text = "ZENITH"
	}
	hash := sha256.Sum256([]byte(text))
	size, cell, quiet := 29, 5, 20
	full := size*cell + quiet*2
	img := image.NewRGBA(image.Rect(0, 0, full, full))
	white := color.RGBA{255, 255, 255, 255}
	black := color.RGBA{17, 24, 39, 255}
	for y := 0; y < full; y++ {
		for x := 0; x < full; x++ {
			img.Set(x, y, white)
		}
	}
	matrix := make([][]bool, size)
	for i := 0; i < size; i++ {
		matrix[i] = make([]bool, size)
	}
	finder := func(x0, y0 int) {
		for y := 0; y < 7; y++ {
			for x := 0; x < 7; x++ {
				outer := x == 0 || x == 6 || y == 0 || y == 6
				inner := x >= 2 && x <= 4 && y >= 2 && y <= 4
				matrix[y0+y][x0+x] = outer || inner
			}
		}
	}
	finder(0, 0)
	finder(size-7, 0)
	finder(0, size-7)
	reserved := func(x, y int) bool { return (x < 8 && y < 8) || (x >= size-8 && y < 8) || (x < 8 && y >= size-8) }
	bit := 0
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if reserved(x, y) {
				continue
			}
			b := (hash[(bit/8)%len(hash)] >> uint(bit%8)) & 1
			matrix[y][x] = b == 1
			bit++
		}
	}
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if matrix[y][x] {
				for py := 0; py < cell; py++ {
					for px := 0; px < cell; px++ {
						img.Set(quiet+x*cell+px, quiet+y*cell+py, black)
					}
				}
			}
		}
	}
	var buf bytes.Buffer
	err := png.Encode(&buf, img)
	return buf.Bytes(), err
}

func linkedDocsHTML(rec Record, all []Record) string {
	// Hidden in the clean letterhead/document design. Job links remain stored in the ERP records.
	return ""
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

func fakeQR(seed string) string {
	if seed == "" {
		seed = "ZENITH"
	}
	sum := sha256.Sum256([]byte(seed))
	var b strings.Builder
	size := 21
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			finder := (x < 7 && y < 7) || (x >= size-7 && y < 7) || (x < 7 && y >= size-7)
			if finder {
				localX, localY := x, y
				if x >= size-7 && y < 7 {
					localX = x - (size - 7)
				}
				if x < 7 && y >= size-7 {
					localY = y - (size - 7)
				}
				border := localX == 0 || localY == 0 || localX == 6 || localY == 6
				center := localX > 1 && localX < 5 && localY > 1 && localY < 5
				if border || center {
					b.WriteString("██")
				} else {
					b.WriteString("  ")
				}
			} else {
				idx := (x*y + x + y) % len(sum)
				if (int(sum[idx])+x+y)%3 == 0 {
					b.WriteString("██")
				} else {
					b.WriteString("  ")
				}
			}
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func moduleLabel(module string) string {
	labels := map[string]string{"customer": "Customer", "supplier": "Supplier", "product": "Product", "lead": "Lead", "rfq": "RFQ", "quotation": "Quotation", "proforma_invoice": "Proforma Invoice", "sales_invoice": "Sales Invoice", "sales_order": "Sales Order", "purchase_order": "Purchase Order", "commercial_invoice": "Commercial Invoice", "packing_list": "Packing List", "shipment": "Shipment", "bill_of_lading": "Bill of Lading", "delivery_note": "Delivery Note", "handover_sheet": "Handover Sheet", "receipt_voucher": "Receipt Voucher", "payment_voucher": "Payment Voucher", "expense": "Expense", "contract": "Contract", "employee": "Employee", "driver": "Driver", "truck": "Truck", "task": "Task", "compliance": "Compliance File", "bank_account": "Bank Account", "approval": "Approval", "document_upload": "Uploaded Document", "email_log": "Email Log", "letterhead": "Letterhead", "business_case": "Business Case"}
	if v := labels[module]; v != "" {
		return v
	}
	return human(module)
}

func human(s string) string {
	s = strings.ReplaceAll(s, "_", " ")
	if s == "" {
		return s
	}
	parts := strings.Fields(s)
	for i, p := range parts {
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, " ")
}

func zipFile(src, dst string) error {
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	zw := zip.NewWriter(out)
	defer zw.Close()
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	hdr, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	hdr.Name = filepath.Base(src)
	hdr.Method = zip.Deflate
	w, err := zw.CreateHeader(hdr)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, f)
	return err
}

const indexHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Zenith Eclipse ERP - Letterhead & Invoices</title>
<style>
:root{--bg:#0f172a;--panel:#111827;--panel2:#1f2937;--card:#ffffff;--muted:#64748b;--text:#0f172a;--brand:#003366;--line:#e5e7eb;--good:#16a34a;--bad:#dc2626;--warn:#f59e0b}
*{box-sizing:border-box}body{margin:0;font-family:Inter,Segoe UI,Arial,sans-serif;background:#eef2f7;color:var(--text)}button,input,select,textarea{font:inherit}button{cursor:pointer}.hidden{display:none!important}
.login{min-height:100vh;display:flex;align-items:center;justify-content:center;background:linear-gradient(135deg,#0f172a,#003366);padding:16px}.login-card{background:white;width:min(460px,92vw);padding:34px;border-radius:22px;box-shadow:0 24px 80px #0007}.login-card h1{margin:0;font-size:28px}.login-card p{color:var(--muted);line-height:1.5}.field{display:flex;flex-direction:column;gap:6px;margin:12px 0}.field label{font-size:13px;font-weight:700;color:#334155}.field input,.field select,.field textarea{border:1px solid #cbd5e1;border-radius:12px;padding:11px 12px;background:white;min-width:0}.field textarea{min-height:92px}.btn{border:0;border-radius:12px;background:var(--brand);color:white;padding:11px 14px;font-weight:800}.btn.secondary{background:#e2e8f0;color:#0f172a}.btn.danger{background:var(--bad)}.btn.good{background:var(--good)}.btn.warn{background:var(--warn);color:#111827}.btn.small{padding:7px 10px;font-size:12px;border-radius:8px}.btn.full{width:100%}
.app{display:grid;grid-template-columns:285px minmax(0,1fr);min-height:100vh}.sidebar{background:#0f172a;color:white;padding:18px;overflow:auto}.brand{display:flex;gap:12px;align-items:center;margin-bottom:18px}.brand-mark{width:42px;height:42px;border-radius:13px;background:#003366;display:flex;align-items:center;justify-content:center;font-weight:900;flex:none}.brand-title{font-size:16px;font-weight:900}.brand-sub{font-size:12px;color:#bfdbfe}.nav-group{margin:18px 0 8px;color:#93c5fd;font-size:11px;font-weight:900;text-transform:uppercase;letter-spacing:.08em}.nav-btn{width:100%;text-align:left;border:0;background:transparent;color:#e5e7eb;padding:10px;border-radius:11px;margin:2px 0;display:flex;justify-content:space-between;gap:8px}.nav-btn:hover,.nav-btn.active{background:#1e293b;color:white}.nav-count{font-size:11px;background:#334155;border-radius:999px;padding:2px 7px;min-width:20px;text-align:center}
main{min-width:0}.topbar{min-height:72px;background:white;border-bottom:1px solid var(--line);display:flex;align-items:center;justify-content:space-between;padding:14px 22px;position:sticky;top:0;z-index:4;gap:12px}.topbar h2{margin:0;font-size:22px}.top-actions{display:flex;gap:8px;align-items:center;flex-wrap:wrap;justify-content:flex-end}.mobile-menu{display:none}.content{padding:22px;min-width:0}.cards{display:grid;grid-template-columns:repeat(4,minmax(150px,1fr));gap:14px;margin-bottom:18px}.card{background:white;border:1px solid var(--line);border-radius:18px;padding:16px;box-shadow:0 3px 14px #00000008}.card .label{color:var(--muted);font-size:12px;font-weight:800;text-transform:uppercase}.card .value{font-size:25px;font-weight:900;margin-top:8px;word-break:break-word}.panel{background:white;border:1px solid var(--line);border-radius:18px;box-shadow:0 3px 14px #00000008;overflow:hidden}.panel-head{padding:15px 16px;border-bottom:1px solid var(--line);display:flex;justify-content:space-between;align-items:center;gap:12px}.panel-head input{border:1px solid #cbd5e1;border-radius:10px;padding:9px 11px;min-width:260px}.table-wrap{overflow:auto;-webkit-overflow-scrolling:touch}table{width:100%;border-collapse:collapse;min-width:760px}th,td{padding:12px;border-bottom:1px solid var(--line);text-align:left;vertical-align:top;font-size:13px}th{font-size:11px;text-transform:uppercase;color:#64748b;background:#f8fafc;letter-spacing:.04em}tr:hover td{background:#f8fafc}.status{display:inline-block;padding:4px 8px;border-radius:999px;background:#e2e8f0;font-weight:800;font-size:11px}.status.Approved,.status.Accepted,.status.Paid,.status.Delivered,.status.Closed{background:#dcfce7;color:#166534}.status.Pending,.status.PendingApproval{background:#fef3c7;color:#92400e}.status.Rejected,.status.Cancelled{background:#fee2e2;color:#991b1b}.status.Booked,.status.InTransit{background:#dbeafe;color:#003366}.muted{color:var(--muted)}.actions-row{display:flex;gap:6px;flex-wrap:wrap}.modal{position:fixed;inset:0;background:#0007;display:flex;align-items:center;justify-content:center;z-index:20;padding:20px}.modal-card{background:white;max-width:980px;width:min(980px,96vw);max-height:92vh;overflow:auto;border-radius:20px;box-shadow:0 24px 80px #0008}.modal-head{padding:18px 20px;border-bottom:1px solid var(--line);display:flex;justify-content:space-between;align-items:center;gap:12px}.modal-body{padding:20px}.form-grid{display:grid;grid-template-columns:repeat(2,minmax(220px,1fr));gap:12px}.item-editor table{min-width:980px}.item-editor input{width:100%;border:1px solid var(--line);border-radius:8px;padding:7px 8px}.item-editor-wrap{grid-column:1/-1}.item-editor th,.item-editor td{padding:7px 8px}.item-editor .btn{padding:5px 8px}.span2{grid-column:1/-1}.toast{position:fixed;right:18px;bottom:18px;background:#111827;color:white;padding:13px 16px;border-radius:13px;box-shadow:0 14px 40px #0005;z-index:30}.flow{display:flex;gap:8px;flex-wrap:wrap}.timeline{padding:12px 16px;display:grid;gap:8px}.timeline-item{border:1px solid var(--line);border-radius:12px;padding:10px;background:#f8fafc}.control-strip{display:flex;gap:8px;flex-wrap:wrap;margin-top:14px}.control-strip span{background:#eef2ff;color:#172554;border:1px solid #dbeafe;border-radius:999px;padding:7px 10px;font-size:12px;font-weight:800}.split{display:grid;grid-template-columns:minmax(0,1.2fr) minmax(0,.8fr);gap:16px}.ai-box textarea{width:100%;min-height:220px;border:1px solid #cbd5e1;border-radius:14px;padding:12px}.pill{display:inline-block;background:#eff6ff;color:#003366;border-radius:999px;padding:4px 9px;font-size:12px;font-weight:800;margin:2px}.danger-text{color:#dc2626;font-weight:800}.small-note{font-size:12px;color:#64748b;line-height:1.5}.history-box{border:1px solid #e5e7eb;background:#f8fafc;border-radius:12px;padding:10px;font-size:12px;line-height:1.5}.module-desc{color:#64748b;line-height:1.5;margin:0 0 14px}.kpi-list{display:grid;gap:8px}.kpi-list div{display:flex;justify-content:space-between;border-bottom:1px dashed #e2e8f0;padding:8px 0;gap:10px}.kpi-list b{word-break:break-word;text-align:right}.upload-zone{border:2px dashed #cbd5e1;border-radius:16px;padding:16px;background:#f8fafc}.img-preview{max-width:180px;max-height:90px;object-fit:contain;border:1px solid #e2e8f0;border-radius:10px;background:#fff;margin-top:6px}.serial-grid{display:grid;grid-template-columns:repeat(3,minmax(140px,1fr));gap:10px}.rtl{direction:rtl}
@media(max-width:1100px){.cards{grid-template-columns:repeat(2,minmax(0,1fr))}.split{grid-template-columns:1fr}.form-grid{grid-template-columns:1fr}.span2{grid-column:auto}}
@media(max-width:820px){.app{display:block}.mobile-menu{display:inline-block}.sidebar{position:fixed;left:-310px;top:0;bottom:0;width:285px;z-index:25;transition:left .2s ease;box-shadow:18px 0 40px #0004}.sidebar.open{left:0}.topbar{align-items:flex-start;padding:10px 12px;min-height:64px}.topbar>div:first-child{min-width:0}.topbar h2{font-size:18px}.top-actions{justify-content:flex-start;overflow-x:auto;max-width:100%;width:100%;padding-bottom:2px}.content{padding:12px}.panel-head{display:block}.panel-head>div{display:flex;flex-wrap:wrap;gap:8px;margin-top:10px}.panel-head input{min-width:0;width:100%}.cards{grid-template-columns:1fr}.modal{padding:8px;align-items:flex-start;overflow:auto}.modal-card{width:100%;max-height:none;border-radius:14px}.modal-body{padding:14px}.actions-row .btn{flex:1 1 auto}.serial-grid{grid-template-columns:1fr}.kpi-list div{display:block}.kpi-list b{text-align:left}.login-card{padding:24px}}
@media(max-width:480px){html,body{max-width:100%;overflow-x:hidden}.btn{padding:9px 10px}.btn.small{font-size:11px;padding:7px 8px}.top-actions .btn{flex:1 0 auto}.brand-title{font-size:14px}.content{padding:10px}.card{padding:13px}.table-wrap{border-radius:12px}th,td{padding:9px;font-size:12px}.form-grid{gap:8px}}
</style>
</head>
<body>
<div id="login" class="login">
  <div class="login-card">
    <h1>Zenith Eclipse ERP</h1>
    <p>Local A-to-Z ERP MVP. Default first login: <b>admin</b> / <b>ChangeMe123!</b>. Change it in Users after login.</p>
    <div class="field"><label>Username</label><input id="loginUser" value="admin" autocomplete="username"></div>
    <div class="field"><label>Password</label><input id="loginPass" type="password" autocomplete="current-password" placeholder="Enter password"></div>
    <button class="btn" style="width:100%" onclick="login()">Login</button>
    <p id="loginMsg" class="danger-text"></p>
  </div>
</div>
<div id="app" class="app hidden">
  <aside id="sideBar" class="sidebar">
    <div class="brand"><div class="brand-mark">ZE</div><div><div class="brand-title">Zenith Eclipse ERP</div><div class="brand-sub">A-to-Z company control</div></div></div>
    <input id="navSearch" placeholder="Search modules" oninput="renderNav()" style="width:100%;border:0;border-radius:12px;padding:10px;background:#1e293b;color:white;margin-bottom:10px">
    <div id="nav"></div>
  </aside>
  <main>
    <div class="topbar">
      <div style="display:flex;gap:10px;align-items:flex-start"><button id="menuBtn" class="btn secondary small mobile-menu" onclick="toggleNav()">☰</button><div><h2 id="pageTitle">Dashboard</h2><div class="muted" id="pageSub">Loading...</div></div></div>
      <div class="top-actions">
        <button class="btn secondary small" onclick="openCompanyModal()">Company</button>
        <button class="btn secondary small" onclick="downloadBackup()">Backup</button>
        <button class="btn secondary small" onclick="openRestoreModal()">Restore</button>
        <button class="btn small" id="newBtn" onclick="openRecordModal()">New</button>
        <button class="btn danger small" onclick="logout()">Logout</button>
      </div>
    </div>
    <div class="content" id="content"></div>
  </main>
</div>
<div id="modal" class="modal hidden"></div>
<div id="toast" class="toast hidden"></div>
<script>
var state=null; var currentModule='dashboard'; var editingId=null; var searchTerm='';
var GROUPS=[
 {name:'Control',items:['dashboard','approvals','audit','ai_helper','users','settings']},
 {name:'Sales',items:['lead','customer','supplier','product','rfq','quotation','proforma_invoice','sales_invoice','sales_order','purchase_order']},
 {name:'Documents',items:['letterhead','contract','commercial_invoice','packing_list','bill_of_lading','delivery_note','handover_sheet','receipt_voucher','payment_voucher','document_upload']},
 {name:'Operations & Transport',items:['shipment','truck','driver','expense']},
 {name:'Admin, Legal & Compliance',items:['business_case','employee','task','compliance','bank_account','email_log']}
];
var CUSTOMER_FIELDS=['customerCode','customer','contactPerson','customerAddress','customerEmail','customerPhone','customerTaxNumber','paymentTerms','currency','creditLimit','outstandingBalance'];
var DOC_COMMON=['documentLanguage','transactionType'].concat(CUSTOMER_FIELDS);
var DOC_OPTIONS=['termsOption','termsTemplateId','termsTemplate','notesOption','bankDetailsOption','bankAccountId','bankDetails','companyLabelOption','stampOption','signatureOption','companyLabelText','signatureName','qrPosition'];
var DOC_FINANCE=['lineTotal','transportationCharges','serviceCharges','discount','taxRate','tax','subtotal','total'];
var LOGISTICS_DOC_OPTIONS=['showPriceTable'].concat(DOC_OPTIONS);
var MODULES={
 dashboard:{label:'Management Dashboard',desc:'Owner dashboard for cash, receivables, payables, shipments, approvals and profit.',virtual:true},
 approvals:{label:'Pending Approvals',desc:'All documents waiting for manager approval.',virtual:true},
 audit:{label:'Audit Log',desc:'Trace every login, create, edit, approval, cancellation and restore.',virtual:true},
 ai_helper:{label:'AI / OCR Helper',desc:'Paste text from invoice, BL, bank statement, scan or email to extract important fields.',virtual:true},
 users:{label:'Users & Roles',desc:'Create user accounts and change the default admin password.',virtual:true},
 settings:{label:'System Settings',desc:'Local storage, security notes, SMTP email, serial numbers and company profile.',virtual:true},
 customer:{label:'Customers',single:'Customer',fields:['name','customerCode','contactPerson','email','phone','mobile','address','country','taxNumber','trn','vatNumber','paymentTerms','creditLimit','outstandingBalance','currency','kycStatus','riskRating','notes']},
 supplier:{label:'Suppliers',single:'Supplier',fields:['name','email','phone','mobile','address','country','taxNumber','outstandingBalance','currency','kybStatus','riskRating','notes']},
 product:{label:'Products / Services',single:'Product or Service',fields:['name','sku','category','description','unit','costPrice','salePrice','currency','supplier']},
 lead:{label:'Leads',single:'Lead',fields:['customerName','contactPerson','email','mobile','source','expectedValue','currency','followUpDate','assignedTo','notes']},
 rfq:{label:'RFQ',single:'RFQ',fields:DOC_COMMON.concat(['supplier','itemsJSON','productDescription','serviceDescription','quantity','unit','unitPrice','hsCode','route','pol','pod','containerNumber','sealNumber','truckNumber','requiredDate','targetPrice','remarks'])},
 quotation:{label:'Quotations',single:'Quotation',fields:DOC_COMMON.concat(['validUntil','itemsJSON','productDescription','serviceDescription','quantity','unit','unitPrice','hsCode','amount','lineTotal','transportationCharges','serviceCharges','discount','taxRate','tax','subtotal','total','cost','route','pol','pod','containerNumber','sealNumber','truckNumber','incoterm','profitMargin']).concat(DOC_OPTIONS).concat(['remarks'])},
 proforma_invoice:{label:'Proforma Invoices',single:'Proforma Invoice',fields:DOC_COMMON.concat(['itemsJSON','productDescription','serviceDescription','quantity','unit','unitPrice','hsCode','amount','lineTotal','transportationCharges','serviceCharges','discount','taxRate','tax','subtotal','total','validUntil','route','pol','pod','containerNumber','sealNumber','incoterm']).concat(DOC_OPTIONS).concat(['remarks'])},
 sales_invoice:{label:'Sales Invoices',single:'Sales Invoice',fields:DOC_COMMON.concat(['itemsJSON','productDescription','serviceDescription','quantity','unit','unitPrice','hsCode','amount','lineTotal','transportationCharges','serviceCharges','discount','taxRate','tax','subtotal','total','invoiceDate','dueDate','route','pol','pod','containerNumber','sealNumber','incoterm']).concat(DOC_OPTIONS).concat(['remarks'])},
 sales_order:{label:'Sales Orders',single:'Sales Order',fields:DOC_COMMON.concat(['itemsJSON','productDescription','serviceDescription','quantity','unit','unitPrice','amount','deliveryDate','route','remarks'])},
 purchase_order:{label:'Purchase Orders',single:'Purchase Order',fields:['supplier','productDescription','quantity','amount','currency','deliveryDate','paymentTerms','remarks']},
 commercial_invoice:{label:'Commercial Invoices',single:'Commercial Invoice',fields:DOC_COMMON.concat(['supplier','itemsJSON','productDescription','serviceDescription','quantity','unit','unitPrice','hsCode','amount','lineTotal','transportationCharges','serviceCharges','discount','taxRate','tax','subtotal','total','cost','invoiceDate','dueDate','taxVat','route','pol','pod','containerNumber','sealNumber','incoterm']).concat(DOC_OPTIONS).concat(['remarks'])},
 packing_list:{label:'Packing Lists',single:'Packing List',fields:DOC_COMMON.concat(['itemsJSON','productDescription','quantity','packages','grossWeight','netWeight','containerNumber','sealNumber','route','showPriceTable']).concat(DOC_OPTIONS).concat(['remarks'])},
 bill_of_lading:{label:'BL / Bill of Lading',single:'Bill of Lading',fields:['itemsJSON','customerCode','customer','customerAddress','customerEmail','customerPhone','shipper','consignee','notifyParty','vesselVoyage','pol','pod','fpod','containerNumber','sealNumber','cargoDescription','grossWeight','netWeight','hsCode','freightTerms','releaseType','surrenderStatus','carrierAgent','showPriceTable','companyLabelOption','stampOption','signatureOption','companyLabelText','signatureName','remarks']},
 delivery_note:{label:'Delivery Notes',single:'Delivery Note',fields:DOC_COMMON.concat(['itemsJSON','shipmentJobNumber','truckNumber','driverName','driverMobile','containerNumber','sealNumber','productDescription','quantity','weight','loadingLocation','deliveryLocation','receiverName','receiverSignature','gpsLocation','deliveryPhotos','saleAmount','showPriceTable']).concat(DOC_OPTIONS).concat(['remarks'])},
 handover_sheet:{label:'Handover Sheets',single:'Handover Sheet',fields:DOC_COMMON.concat(['itemsJSON','shipmentJobNumber','warehouseName','truckNumber','driverName','driverMobile','containerNumber','sealNumber','productDescription','quantity','packages','grossWeight','netWeight','loadingLocation','deliveryLocation','receiverName','receiverSignature','gpsLocation','handoverDate']).concat(LOGISTICS_DOC_OPTIONS).concat(['remarks'])},
 receipt_voucher:{label:'Receipt Vouchers',single:'Receipt Voucher',fields:CUSTOMER_FIELDS.concat(['amount','bankOrCash','paymentMethod','referenceNo','receivedDate']).concat(DOC_OPTIONS).concat(['remarks'])},
 payment_voucher:{label:'Payment Vouchers',single:'Payment Voucher',fields:['supplier','amount','currency','bankOrCash','paymentMethod','referenceNo','paymentDate','approvalReason'].concat(DOC_OPTIONS).concat(['remarks'])},
 shipment:{label:'Shipments',single:'Shipment',fields:['customer','supplier','route','bookingNo','containerNumber','sealNumber','vesselVoyage','pol','pod','fpod','truckNumber','driverName','driverMobile','customsBayanNo','gatePassNo','portStatus','customsStatus','transitStatus','borderStatus','saleAmount','estimatedCost','currency','showPriceTable','eta','remarks']},
 truck:{label:'Trucks',single:'Truck',fields:['truckNumber','trailerNumber','owner','driverName','registrationExpiry','insuranceExpiry','status','notes']},
 driver:{label:'Drivers',single:'Driver',fields:['name','mobile','licenseNo','licenseExpiry','passportNo','nationality','assignedTruck','status','notes']},
 expense:{label:'Expenses',single:'Expense',fields:['department','expenseType','supplier','amount','currency','expenseDate','paymentStatus','approvalReason','remarks']},
 contract:{label:'Legal Contracts',single:'Contract',fields:CUSTOMER_FIELDS.concat(['documentLanguage','createBilingualDocument','primaryLanguage','secondLanguage','secondLanguageBody','contractTitle','partyName','toAddress','contractType','subject','date','startDate','expiryDate','value','riskFlag','approvalStatus','contractBody','terms','startNewPageAfterFirstPage','showWorkflowInFooter','bankDetailsOption','bankAccountId','bankDetails','stampOption','signatureOption','signatureName','versionNotes','remarks'])},
 employee:{label:'Employees / HR',single:'Employee',fields:['name','department','role','mobile','email','joinDate','visaExpiry','passportExpiry','leaveBalance','status','notes']},
 task:{label:'Tasks & Notices',single:'Task',fields:['title','department','assignedTo','priority','dueDate','status','details']},
 compliance:{label:'KYC / Compliance',single:'Compliance File',fields:['partyName','partyType','country','beneficialOwner','sourceOfFunds','sourceOfWealth','transactionPurpose','countryRisk','sanctionsScreening','riskRating','approvalStatus','remarks']},
 bank_account:{label:'Bank Accounts',single:'Bank Account',fields:['bankName','accountName','accountNumber','iban','swift','currency','openingBalance','currentBalance','lastReconciledDate','notes']},
 approval:{label:'Approval Request',single:'Approval Request',fields:['requestType','department','requestedBy','relatedDocument','amount','currency','reason','approver','decisionNotes']},
 document_upload:{label:'Uploaded Documents',single:'Uploaded Document',fields:['fileName','documentType','recordId','savedPath','sizeBytes','notes']},
 email_log:{label:'Email Sending History',single:'Email Log',fields:['to','cc','bcc','subject','documentNumber','customer','sentAt','attachment','body']},
 letterhead:{label:'Letterhead / Letters',single:'Letterhead Document',fields:CUSTOMER_FIELDS.concat(['documentLanguage','createBilingualDocument','primaryLanguage','secondLanguage','secondLanguageBody','title','subject','date','to','toAddress','body','startNewPageAfterFirstPage','showWorkflowInFooter','bankDetailsOption','bankAccountId','bankDetails','stampOption','signatureOption','signatureName','remarks'])},
 terms_template:{label:'Terms Templates',single:'Terms Template',fields:['name','templateText','language','status','notes']},
 business_case:{label:'Business Cases',single:'Business Case',fields:['title','customer','supplier','priority','owner','notes']}
};
var CONVERT={rfq:['quotation'],quotation:['proforma_invoice'],proforma_invoice:['sales_invoice'],sales_invoice:['commercial_invoice'],commercial_invoice:['packing_list'],sales_order:['purchase_order','shipment'],purchase_order:['payment_voucher'],shipment:['bill_of_lading','delivery_note'],bill_of_lading:['delivery_note'],delivery_note:['sales_invoice'],handover_sheet:['delivery_note']};
function $(id){return document.getElementById(id)}
function toast(msg){var t=$('toast');t.textContent=msg;t.classList.remove('hidden');setTimeout(function(){t.classList.add('hidden')},2600)}
function api(url,opts){opts=opts||{};opts.headers=opts.headers||{};if(opts.body && !(opts.body instanceof FormData)){opts.headers['Content-Type']='application/json'}return fetch(url,opts).then(function(r){return r.json().then(function(j){if(!r.ok||j.ok===false){throw new Error(j.error||'Request failed')}return j})})}
function login(){api('/api/login',{method:'POST',body:JSON.stringify({username:$('loginUser').value,password:$('loginPass').value})}).then(loadState).catch(function(e){$('loginMsg').textContent=e.message})}
function logout(){fetch('/api/logout',{method:'POST'}).then(function(){location.reload()})}
function loadState(){return api('/api/state').then(function(j){state=j;$('login').classList.add('hidden');$('app').classList.remove('hidden');renderNav();render()}).catch(function(){ $('login').classList.remove('hidden');$('app').classList.add('hidden') })}
function countModule(m){if(!state)return 0;if(m==='approvals')return state.records.filter(function(r){return r.status==='Pending Approval'}).length;if(MODULES[m]&&MODULES[m].virtual)return 0;return state.records.filter(function(r){return r.module===m}).length}
function toggleNav(){var s=$('sideBar');if(s)s.classList.toggle('open')}
function closeNavOnMobile(){var s=$('sideBar');if(s&&window.innerWidth<=820)s.classList.remove('open')}
function renderNav(){var q=($('navSearch').value||'').toLowerCase();var html='';GROUPS.forEach(function(g){var items=g.items.filter(function(k){return !q || (MODULES[k]&&MODULES[k].label.toLowerCase().indexOf(q)>=0)});if(!items.length)return;html+='<div class="nav-group">'+g.name+'</div>';items.forEach(function(k){var active=currentModule===k?' active':'';var cnt=countModule(k);html+='<button class="nav-btn'+active+'" onclick="go(\''+k+'\')"><span>'+MODULES[k].label+'</span><span class="nav-count">'+(cnt||'')+'</span></button>'})});$('nav').innerHTML=html}
function go(m){currentModule=m;searchTerm='';renderNav();render();closeNavOnMobile()}
function render(){if(!state)return;var mod=MODULES[currentModule];$('pageTitle').textContent=mod.label;$('pageSub').textContent=mod.desc||'';$('newBtn').style.display=(mod.virtual?'none':'inline-block');if(currentModule==='dashboard')return renderDashboard();if(currentModule==='approvals')return renderApprovals();if(currentModule==='audit')return renderAudit();if(currentModule==='ai_helper')return renderAI();if(currentModule==='users')return renderUsers();if(currentModule==='settings')return renderSettings();return renderModule(currentModule)}
function money(n){n=parseFloat(n||0);if(isNaN(n))n=0;return n.toLocaleString(undefined,{maximumFractionDigits:2})}
function num(v){var n=parseFloat((v||'').toString().replace(/,/g,''));return isNaN(n)?0:n}
function field(r,k){return (r.fields&&r.fields[k])||''}
function dashboardMetrics(){var rec=state.records||[];var receivables=0,payables=0,profit=0,cash=0;rec.forEach(function(r){if(r.module==='customer')receivables+=num(field(r,'outstandingBalance'));if(r.module==='supplier')payables+=num(field(r,'outstandingBalance'));if(r.module==='bank_account')cash+=num(field(r,'currentBalance'));var sale=num(field(r,'amount'))||num(field(r,'saleAmount'));var cost=num(field(r,'cost'))||num(field(r,'estimatedCost'));if(sale||cost)profit+=sale-cost});return {cash:cash,receivables:receivables,payables:payables,profit:profit,openShipments:rec.filter(function(r){return r.module==='shipment'&&['Closed','Delivered','Cancelled'].indexOf(r.status)<0}).length,pendingBL:rec.filter(function(r){return r.module==='bill_of_lading'&&r.status!=='Approved'}).length,pendingDN:rec.filter(function(r){return r.module==='delivery_note'&&r.status!=='Delivered'}).length,pendingApprovals:rec.filter(function(r){return r.status==='Pending Approval'}).length}}
function renderDashboard(){var m=dashboardMetrics();var latest=state.records.slice(0,14);var html='<div class="cards">'+card('Cash / Bank Balance',money(m.cash)+' '+state.company.baseCurrency)+card('Receivables',money(m.receivables)+' '+state.company.baseCurrency)+card('Payables',money(m.payables)+' '+state.company.baseCurrency)+card('Estimated Profit',money(m.profit)+' '+state.company.baseCurrency)+card('Open Shipments',m.openShipments)+card('Pending BL',m.pendingBL)+card('Pending Delivery Notes',m.pendingDN)+card('Pending Approvals',m.pendingApprovals)+'</div>';html+='<div class="panel"><div class="panel-head"><b>Latest Activity Documents</b><div><input placeholder="Search" oninput="searchTerm=this.value;renderDashboard()"> <button class="btn secondary small" onclick="go(&quot;settings&quot;)">Security / Serial Settings</button></div></div>'+tableRecords(latest.filter(matchesSearch),true)+'</div>';html+='<div class="control-strip"><span>✓ No permanent delete</span><span>✓ Audit log active</span><span>✓ Same job reference flow</span><span>✓ QR document verification</span></div>';$('content').innerHTML=html}
function card(label,value){return '<div class="card"><div class="label">'+label+'</div><div class="value">'+value+'</div></div>'}
function matchesSearch(r){if(!searchTerm)return true;var q=searchTerm.toLowerCase();return JSON.stringify(r).toLowerCase().indexOf(q)>=0}
function renderModule(module){var rec=state.records.filter(function(r){return r.module===module}).filter(matchesSearch);var html='<p class="module-desc">'+(MODULES[module].desc||'')+'</p><div class="panel"><div class="panel-head"><b>'+MODULES[module].label+' ('+rec.length+')</b><div><input placeholder="Search records" oninput="searchTerm=this.value;render()"> <button class="btn small" onclick="openRecordModal()">New '+(MODULES[module].single||'Record')+'</button></div></div>'+tableRecords(rec,false)+'</div>';$('content').innerHTML=html}
function tableRecords(rec,showModule){if(!rec.length)return '<div style="padding:22px" class="muted">No records yet.</div>';var html='<div class="table-wrap"><table><thead><tr><th>No</th>'+(showModule?'<th>Module</th>':'')+'<th>Job</th><th>Status</th><th>Main Info</th><th>Amount</th><th>Updated</th><th>Actions</th></tr></thead><tbody>';rec.forEach(function(r){var main=field(r,'name')||field(r,'customer')||field(r,'supplier')||field(r,'partyName')||field(r,'contractTitle')||field(r,'title')||field(r,'truckNumber')||field(r,'driverName')||field(r,'productDescription')||field(r,'route')||'';var amount=field(r,'amount')||field(r,'saleAmount')||field(r,'currentBalance')||field(r,'outstandingBalance')||'';var st=(r.status||'').replace(/ /g,'');html+='<tr><td><b>'+esc(r.number)+'</b><br><span class="muted">v'+r.version+'</span></td>'+(showModule?'<td>'+moduleName(r.module)+'</td>':'')+'<td>'+esc(r.jobRef||'')+'</td><td><span class="status '+st+'">'+esc(r.status)+'</span></td><td>'+esc(main)+'<br><span class="muted">'+esc(field(r,'containerNumber')||field(r,'email')||field(r,'route')||'')+'</span></td><td>'+esc(amount)+' '+esc(field(r,'currency')||'')+'</td><td>'+shortDate(r.updatedAt)+'<br><span class="muted">'+esc(r.updatedBy||'')+'</span></td><td>'+actions(r)+'</td></tr>'});html+='</tbody></table></div>';return html}
function actions(r){var html='<div class="actions-row"><button class="btn secondary small" onclick="openRecordModal(\''+r.id+'\')">Edit</button><button class="btn secondary small" onclick="window.open(\'/doc/'+r.id+'\',\'_blank\')">Preview</button><button class="btn secondary small" onclick="window.location=\'/export/'+r.id+'/pdf\'">PDF</button><button class="btn secondary small" onclick="window.location=\'/export/'+r.id+'/word\'">Word</button>';if(['quotation','proforma_invoice','sales_invoice','commercial_invoice','packing_list','bill_of_lading','delivery_note','handover_sheet','contract','letterhead','receipt_voucher','payment_voucher'].indexOf(r.module)>=0)html+='<button class="btn secondary small" onclick="openEmailModal(\''+r.id+'\')">Email</button>';if(r.status==='Pending Approval')html+='<button class="btn good small" onclick="setStatus(\''+r.id+'\',\'Approved\')">Approve</button><button class="btn warn small" onclick="setStatus(\''+r.id+'\',\'Rejected\')">Reject</button>';if(r.status!=='Cancelled')html+='<button class="btn danger small" onclick="setStatus(\''+r.id+'\',\'Cancelled\')">Cancel</button>';var targets=CONVERT[r.module]||[];targets.forEach(function(t){html+='<button class="btn small" onclick="convertRecord(\''+r.id+'\',\''+t+'\')">To '+moduleName(t)+'</button>'});html+='</div>';return html}
function moduleName(m){return (MODULES[m]&&MODULES[m].label)||m}
function shortDate(s){if(!s)return'';try{return new Date(s).toLocaleString()}catch(e){return s}}
function esc(s){return (s==null?'':String(s)).replace(/[&<>"']/g,function(c){return {'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]})}
function pretty(k){return k.replace(/_/g,' ').replace(/([A-Z])/g,' $1').replace(/^./,function(c){return c.toUpperCase()})}
function inputType(k){var l=k.toLowerCase();if(l.indexOf('date')>=0||l.indexOf('expiry')>=0||l==='eta'||l==='validuntil'||l==='duedate'||l==='receiveddate'||l==='paymentdate'||l==='followupdate'||l==='joindate')return 'date';if(l.indexOf('amount')>=0||l.indexOf('cost')>=0||l.indexOf('price')>=0||l.indexOf('balance')>=0||l.indexOf('quantity')>=0||l.indexOf('weight')>=0||l.indexOf('limit')>=0||l.indexOf('value')>=0||l.indexOf('tax')>=0)return 'number';if(l.indexOf('email')>=0)return 'email';return 'text'}
function closeModal(){var m=$('modal');if(m){m.classList.add('hidden');m.innerHTML=''}editingId=null}
function optionHTML(options,current,def){current=current||def||'';return options.map(function(o){return '<option value="'+esc(o)+'" '+(o===current?'selected':'')+'>'+esc(o)+'</option>'}).join('')}
function statusOptions(current){return optionHTML(['Draft','Open','Pending','Pending Approval','Approved','Accepted','Delivered','Paid','Closed','Rejected','Cancelled','Expired'],current,'Draft')}
function defaultStatusJS(module){if(['customer','supplier','product','bank_account','employee','driver','truck'].indexOf(module)>=0)return 'Open';if(['quotation','proforma_invoice','sales_invoice','commercial_invoice','packing_list','bill_of_lading','delivery_note','handover_sheet','contract','letterhead'].indexOf(module)>=0)return 'Draft';return 'Open'}
function currencyOptions(current){var list=((state.company&&state.company.currencyList)||'USD,AED,AFN,CNY,EUR,GBP,OMR').split(',').map(function(x){return x.trim()}).filter(Boolean);if(list.indexOf(current)<0&&current)list.unshift(current);return optionHTML(list,current,(state.company&&state.company.baseCurrency)||'USD')}
function bankAccountOptions(current){var rows=(state.records||[]).filter(function(r){return r.module==='bank_account'});var html='<option value="">Hide / no bank selected</option>';rows.forEach(function(r){var val=r.id;var label=(field(r,'bankName')||r.number)+' - '+(field(r,'accountName')||field(r,'currency')||'');html+='<option value="'+esc(val)+'" '+(val===current?'selected':'')+'>'+esc(label)+'</option>'});return html}
function termsTemplateOptions(current){var rows=(state.records||[]).filter(function(r){return r.module==='terms_template'});var html='<option value="">Default / manual terms</option>';rows.forEach(function(r){var val=r.id;var label=field(r,'name')||r.number;html+='<option value="'+esc(val)+'" '+(val===current?'selected':'')+'>'+esc(label)+'</option>'});return html}
function documentOptionChoices(k,current){var opts={
 companyLabelOption:['Hide','Placeholder','Use Company Label'],
 stampOption:['Hide','Placeholder','Use Company Stamp'],
 signatureOption:['Hide','Placeholder','Use Authorized Signature'],
 bankDetailsOption:['Hide','Selected Bank Account','Custom Bank Details'],
 showWorkflowInFooter:['Yes','No'],
 startNewPageAfterFirstPage:['No','Yes'],
 showPriceTable:['No','Yes'],
 termsOption:['Default Terms','Custom Terms','Select Terms Template','Hide Terms','Remove Terms'],
 notesOption:['Hide Notes','Show Notes'],
 qrPosition:['Footer Right','Footer Center','Hide QR']
};var def={companyLabelOption:'Hide',stampOption:'Placeholder',signatureOption:'Placeholder',bankDetailsOption:'Hide',showWorkflowInFooter:'Yes',startNewPageAfterFirstPage:'No',showPriceTable:'No',termsOption:'Default Terms',notesOption:'Hide Notes',qrPosition:'Footer Right'};return optionHTML(opts[k]||['Yes','No'],current,def[k]||'')}
function applyTransactionType(){var el=$('f_transactionType');var tx=(el?el.value:'Product + Transportation').toLowerCase();var showProduct=tx.indexOf('product')>=0||tx.indexOf('goods')>=0||tx.indexOf('+')>=0;var showService=tx.indexOf('service')>=0;var showTransport=tx.indexOf('transport')>=0||tx.indexOf('logistics')>=0||tx.indexOf('+')>=0;document.querySelectorAll('[data-rel]').forEach(function(w){var rel=w.getAttribute('data-rel');var show=true;if(rel==='product')show=showProduct;if(rel==='service')show=showService;if(rel==='transport')show=showTransport;w.style.display=show?'':'none'})}
function recordHistory(rec){var html='<div class="field span2"><label>Version History</label><div class="history-box">';(rec.history||[]).slice().reverse().slice(0,6).forEach(function(h){html+='<div><b>v'+esc(h.version)+'</b> '+esc(h.time||'')+' — '+esc(h.reason||'Updated')+'</div>'});if(!(rec.history||[]).length)html+='<div class="muted">No previous version history.</div>';return html+'</div></div>'}
function openRecordModal(id){
 editingId=id||null;var mod=MODULES[currentModule];var rec=editingId?state.records.find(function(x){return x.id===editingId}):null;if(rec)mod=MODULES[rec.module];var module=rec?rec.module:currentModule;var fields=mod.fields||[];
 var html='<div class="modal-card"><div class="modal-head"><b>'+(rec?'Edit ':'New ')+(mod.single||'Record')+'</b><button class="btn secondary small" onclick="closeModal()">Close</button></div><div class="modal-body"><div class="form-grid">';
 if(rec){html+='<div class="field"><label>Document Number</label><input id="f_number" value="'+esc(rec.number)+'"></div><div class="field"><label>Job Reference</label><input id="f_jobRef" value="'+esc(rec.jobRef||'')+'"></div><div class="field"><label>Status</label><select id="f_status">'+statusOptions(rec.status)+'</select></div><div class="field"><label>Reason for change</label><input id="changeReason" placeholder="Required for sensitive changes"></div>'}else{html+='<div class="field"><label>Job Reference (optional)</label><input id="f_jobRef" placeholder="Auto-generated for business documents"></div><div class="field"><label>Status</label><select id="f_status">'+statusOptions(defaultStatusJS(currentModule))+'</select></div>'}
 fields.forEach(function(k){if(k==='itemsJSON')return;var val=rec?field(rec,k):'';var low=k.toLowerCase();var span=(low.indexOf('description')>=0||low.indexOf('remarks')>=0||low.indexOf('notes')>=0||low.indexOf('details')>=0||low.indexOf('body')>=0||low.indexOf('terms')>=0)?' span2':'';var rel=fieldRelation(k);html+='<div class="field'+span+'" data-field="'+esc(k)+'" data-rel="'+rel+'"><label>'+pretty(k)+'</label>';
  if(k==='transactionType'){html+='<select id="f_'+k+'" onchange="applyTransactionType()">'+transactionTypeOptions(val)+'</select>'}
  else if(k==='documentLanguage'||k==='primaryLanguage'||k==='secondLanguage'){html+='<select id="f_'+k+'">'+languageOptions(val,k==='secondLanguage'?'Russian':'English')+'</select>'}
  else if(k==='createBilingualDocument'){html+='<select id="f_'+k+'">'+bilingualOptions(val)+'</select>'}
  else if(isCustomerField(k)){html+=customerInputHTML(k,val)}
  else if(k==='bankAccountId'){html+='<select id="f_'+k+'">'+bankAccountOptions(val)+'</select>'}
  else if(k==='termsTemplateId'){html+='<select id="f_'+k+'">'+termsTemplateOptions(val)+'</select>'}
  else if(k==='companyLabelOption'||k==='stampOption'||k==='signatureOption'||k==='bankDetailsOption'||k==='showWorkflowInFooter'||k==='startNewPageAfterFirstPage'||k==='showPriceTable'||k==='termsOption'||k==='notesOption'||k==='qrPosition'){html+='<select id="f_'+k+'">'+documentOptionChoices(k,val)+'</select>'}
  else if(span){html+='<textarea id="f_'+k+'">'+esc(val)+'</textarea>'}
  else if(k==='currency'){html+='<select id="f_'+k+'">'+currencyOptions(val)+'</select>'}
  else{var calcAttr=(calcField(k)?' oninput="recalculateRecordTotals()" onchange="recalculateRecordTotals()"':'');html+='<input type="'+inputType(k)+'" id="f_'+k+'" value="'+esc(val)+'"'+calcAttr+'>'}
  html+='</div>'});
 html+='</div>'+itemRowsHTML(rec,module)+customerDatalistHTML()+'<div style="margin-top:18px;display:flex;gap:8px;flex-wrap:wrap"><button class="btn" onclick="saveRecord()">Save</button><button class="btn secondary" onclick="closeModal()">Cancel</button></div><p class="small-note">Customer fields auto-fill after selecting a customer. Transaction Type hides fields that are not related to Product/Goods, Service, Transportation/Logistics, or Product + Transportation.</p>';if(rec)html+=recordHistory(rec);html+='</div></div>';$('modal').innerHTML=html;$('modal').classList.remove('hidden');applyTransactionType();recalculateRecordTotals()
}

function itemModule(module){return ['rfq','quotation','proforma_invoice','sales_invoice','sales_order','purchase_order','commercial_invoice','packing_list','bill_of_lading','delivery_note','handover_sheet'].indexOf(module)>=0}
function commercialItemModule(module){return ['rfq','quotation','proforma_invoice','sales_invoice','sales_order','purchase_order','commercial_invoice'].indexOf(module)>=0}
function itemRowsFromRecord(rec,module){var rows=[];if(rec&&field(rec,'itemsJSON')){try{rows=JSON.parse(field(rec,'itemsJSON'))||[]}catch(e){rows=[]}}if(!rows.length&&rec){var desc=field(rec,'productDescription')||field(rec,'cargoDescription')||field(rec,'description');var qty=field(rec,'quantity')||field(rec,'qty');var unitPrice=field(rec,'unitPrice')||field(rec,'price')||field(rec,'rate');var total=field(rec,'lineTotal')||field(rec,'amount')||field(rec,'saleAmount');if(desc||qty||unitPrice||total){rows=[{description:desc,hsCode:field(rec,'hsCode'),unit:field(rec,'unit')||'Unit',quantity:qty,unitPrice:unitPrice,total:total,netWeight:field(rec,'netWeight'),grossWeight:field(rec,'grossWeight'),packages:field(rec,'packages')}]} }if(!rows.length)rows=[{description:'',hsCode:'',unit:'Unit',quantity:'1',unitPrice:'',total:'',netWeight:'',grossWeight:'',packages:''}];return rows}
function itemRowsHTML(rec,module){if(!itemModule(module))return '';var rows=itemRowsFromRecord(rec,module);var showPrice=commercialItemModule(module);var html='<div class="field span2 item-editor-wrap"><label>Product / Service Rows</label><input type="hidden" id="f_itemsJSON" value="'+esc(JSON.stringify(rows))+'"><div class="table-wrap item-editor"><table id="itemRowsTable"><thead><tr><th>Description</th><th>HS Code</th><th>Unit</th><th>Qty</th>'+(showPrice?'<th>Unit Price</th><th>Total</th>':'')+'<th>Net Wt</th><th>Gross Wt</th><th>Packages</th><th></th></tr></thead><tbody>';rows.forEach(function(r){html+=itemRowHTML(r,showPrice)});html+='</tbody></table></div><div style="margin-top:8px"><button type="button" class="btn secondary small" onclick="addProductRow()">Add Product Row</button></div><p class="small-note">Each row calculates Quantity × Unit Price = Total. If rows exceed one A4 page, the document table continues on the next A4 page.</p></div>';return html}
function itemRowHTML(r,showPrice){r=r||{};return '<tr class="item-row"><td><input data-item="description" value="'+esc(r.description||r.productDescription||r.name||'')+'"></td><td><input data-item="hsCode" value="'+esc(r.hsCode||r.HSCode||'')+'"></td><td><input data-item="unit" value="'+esc(r.unit||r.uom||'Unit')+'"></td><td><input type="number" step="any" data-item="quantity" value="'+esc(r.quantity||r.qty||'1')+'" oninput="syncItemRows()"></td>'+(showPrice?'<td><input type="number" step="any" data-item="unitPrice" value="'+esc(r.unitPrice||r.price||r.rate||'')+'" oninput="syncItemRows()"></td><td><input type="number" step="any" data-item="total" value="'+esc(r.total||r.amount||r.lineTotal||'')+'" readonly></td>':'')+'<td><input type="number" step="any" data-item="netWeight" value="'+esc(r.netWeight||'')+'"></td><td><input type="number" step="any" data-item="grossWeight" value="'+esc(r.grossWeight||'')+'"></td><td><input type="number" step="any" data-item="packages" value="'+esc(r.packages||r.cartons||'')+'"></td><td><button type="button" class="btn danger small" onclick="removeProductRow(this)">×</button></td></tr>'}
function addProductRow(){var t=$('itemRowsTable');if(!t)return;var show=t.querySelector('thead').textContent.indexOf('Unit Price')>=0;t.querySelector('tbody').insertAdjacentHTML('beforeend',itemRowHTML({unit:'Unit',quantity:'1'},show));syncItemRows()}
function removeProductRow(btn){var tbody=$('itemRowsTable').querySelector('tbody');if(tbody.children.length>1){btn.closest('tr').remove()}else{btn.closest('tr').querySelectorAll('input').forEach(function(i){i.value=(i.dataset.item==='unit'?'Unit':(i.dataset.item==='quantity'?'1':''))})}syncItemRows()}
function readItemRows(){var t=$('itemRowsTable');if(!t)return null;var rows=[];t.querySelectorAll('tbody tr').forEach(function(tr){var r={};tr.querySelectorAll('input[data-item]').forEach(function(inp){r[inp.dataset.item]=inp.value});var q=num(r.quantity)||0;var p=num(r.unitPrice)||0;if(p&&q){r.total=(q*p).toFixed(2);var totalInput=tr.querySelector('input[data-item="total"]');if(totalInput)totalInput.value=r.total}else if(r.total){var totalInput=tr.querySelector('input[data-item="total"]');if(totalInput)totalInput.value=r.total}if(Object.keys(r).some(function(k){return String(r[k]||'').trim()!==''&&!(k==='unit'&&r[k]==='Unit')&&!(k==='quantity'&&r[k]==='1')}))rows.push(r)});if(!rows.length)rows=[{description:'',unit:'Unit',quantity:'1'}];return rows}
function syncItemRows(){var rows=readItemRows();if(!rows)return;var hidden=$('f_itemsJSON');if(hidden)hidden.value=JSON.stringify(rows);var first=rows[0]||{};var setPlain=function(k,v){var el=$('f_'+k);if(el)el.value=v||''};setPlain('productDescription',first.description||'');setPlain('hsCode',first.hsCode||'');setPlain('unit',first.unit||'');setPlain('quantity',first.quantity||'');setPlain('unitPrice',first.unitPrice||'');var sum=0;rows.forEach(function(r){var q=num(r.quantity)||0;var p=num(r.unitPrice)||0;var t=(q&&p)?q*p:num(r.total);sum+=t||0});if(sum>0){setCalcValue('lineTotal',sum);setCalcValue('amount',sum)}recalculateRecordTotals(true)}
function fieldRelation(k){var l=k.toLowerCase();if(['productdescription','quantity','unit','unitprice','hscode','grossweight','netweight','packages','weight'].indexOf(l)>=0)return 'product';if(['servicedescription'].indexOf(l)>=0)return 'service';if(['route','pol','pod','fpod','containernumber','sealnumber','trucknumber','drivername','drivermobile','loadinglocation','deliverylocation','shipmentjobnumber','vesselvoyage'].indexOf(l)>=0)return 'transport';return 'common'}
function languageOptions(current,def){var opts=['English','Russian','Chinese','Arabic'];current=current||def||'English';return opts.map(function(o){return '<option '+(o===current?'selected':'')+'>'+o+'</option>'}).join('')}
function bilingualOptions(current){var opts=['No','Create bilingual document'];current=current||'No';return opts.map(function(o){return '<option '+(o===current?'selected':'')+'>'+o+'</option>'}).join('')}
function transactionTypeOptions(current){var opts=['Product/Goods','Service','Transportation/Logistics','Product + Transportation'];current=current||'Product + Transportation';return opts.map(function(o){return '<option '+(o===current?'selected':'')+'>'+o+'</option>'}).join('')}
function customerRecords(){return (state.records||[]).filter(function(r){return r.module==='customer'})}
function isCustomerField(k){return ['customer','customerName','buyer','partyName','to','consignee','shipper','notifyParty'].indexOf(k)>=0}
function cleanEmail(v){v=String(v||'').trim();if(v.indexOf('@')>=0){var p=v.split('@');p[1]=(p[1]||'').replace(/,/g,'.');return p[0]+'@'+p.slice(1).join('@')}return v}
function isValidEmail(v){v=String(v||'').trim();return !v || /^[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}$/.test(v)}
function validateEmailInputs(){var bad=[];document.querySelectorAll('input[id^="f_"]').forEach(function(el){var k=el.id.slice(2).toLowerCase();if(k.indexOf('email')>=0&&el.value){if(el.value.indexOf(',')>=0)bad.push(el.id+': use dot, not comma');else if(!isValidEmail(el.value))bad.push(el.id+': invalid email format')}});if(bad.length){toast(bad.join(' | '));return false}return true}
function customerDatalistHTML(){var html='<datalist id="customerList">';customerRecords().forEach(function(c){var name=field(c,'name');var code=field(c,'customerCode')||field(c,'code')||c.number;var email=cleanEmail(field(c,'email'));if(name)html+='<option value="'+esc(name)+'">'+esc((code||'')+(email?' / '+email:''))+'</option>';if(code)html+='<option value="'+esc(code)+'">'+esc(name||'')+'</option>';if(email)html+='<option value="'+esc(email)+'">'+esc(name||'')+'</option>'});return html+'</datalist>'}
function customerInputHTML(k,val){return '<input list="customerList" id="f_'+k+'" value="'+esc(val)+'" oninput="fillCustomerFields(this.value,false)" onchange="fillCustomerFields(this.value,true)"><button type="button" class="btn secondary small" style="margin-top:6px" onclick="fillCustomerFields(document.getElementById(\'f_'+k+'\').value,true)">Fill Customer Data</button>'}
function normText(v){return (v||'').toString().toLowerCase().replace(/[^a-z0-9]+/g,'').trim()}
function codeFromName(n){return String(n||'').split(/\s+/).map(function(w){return w[0]||''}).join('').toUpperCase().slice(0,8)||'CUS'}
function findCustomerMatch(q){var n=normText(q);if(!n)return null;var list=customerRecords();var exact=list.find(function(c){return [field(c,'name'),field(c,'customerCode'),field(c,'code'),c.number,c.id,cleanEmail(field(c,'email')),field(c,'phone'),field(c,'mobile')].some(function(v){return normText(v)===n})});if(exact)return exact;return list.find(function(c){return [field(c,'name'),field(c,'customerCode'),field(c,'code'),c.number,cleanEmail(field(c,'email'))].some(function(v){var x=normText(v);return x && n.length>=3 && (x.indexOf(n)>=0||n.indexOf(x)>=0)})})||null}
function fillCustomerFields(name,force){var c=findCustomerMatch(name);if(!c)return;var set=function(k,v){var el=$('f_'+k);if(el&&(force||!el.value))el.value=v||''};var cname=field(c,'name');var code=field(c,'customerCode')||field(c,'code')||codeFromName(cname);var email=cleanEmail(field(c,'email')||field(c,'contactEmail'));var phone=field(c,'phone')||field(c,'mobile')||field(c,'contactPhone');var trn=field(c,'taxNumber')||field(c,'trn')||field(c,'vatNumber');var contact=field(c,'contactPerson')||field(c,'contact')||field(c,'attention');var address=field(c,'address')||field(c,'billingAddress')||field(c,'deliveryAddress')||field(c,'country');set('customerCode',code);['customer','customerName','buyer','partyName','to'].forEach(function(k){set(k,cname)});set('customerAddress',address);set('toAddress',address);set('partyAddress',address);set('contactPerson',contact);set('contact',contact);set('customerEmail',email);set('email',email);set('customerPhone',phone);set('phone',phone);set('customerTaxNumber',trn);set('taxNumber',trn);set('paymentTerms',field(c,'paymentTerms'));set('currency',field(c,'currency'));set('creditLimit',field(c,'creditLimit'));set('outstandingBalance',field(c,'outstandingBalance')||field(c,'balance')||field(c,'currentBalance'));toast('Customer details filled');recalculateRecordTotals()}
function calcField(k){return ['quantity','qty','unitPrice','price','rate','lineTotal','amount','saleAmount','transportationCharges','transportCharges','serviceCharges','discount','taxRate','tax','vat','taxVat','subtotal','total'].indexOf(k)>=0}
function setCalcValue(k,v){var el=$('f_'+k);if(el)el.value=(Math.round((v||0)*100)/100).toFixed(2)}
function recalculateRecordTotals(skipSync){if(!skipSync&&$('itemRowsTable')){syncItemRows();return}var line=0;if($('itemRowsTable')){var rows=readItemRows()||[];rows.forEach(function(r){var q=num(r.quantity)||0;var p=num(r.unitPrice)||0;var t=(q&&p)?q*p:num(r.total);line+=t||0})}else{var q=num(($('f_quantity')||{}).value)||num(($('f_qty')||{}).value)||1;var p=num(($('f_unitPrice')||{}).value)||num(($('f_price')||{}).value)||num(($('f_rate')||{}).value);line=q*p;var existingAmount=num(($('f_amount')||{}).value)||num(($('f_saleAmount')||{}).value);if(line>0){setCalcValue('lineTotal',line);setCalcValue('amount',line)}else{line=existingAmount}}var trans=num(($('f_transportationCharges')||{}).value)||num(($('f_transportCharges')||{}).value)||num(($('f_transportCharge')||{}).value)||num(($('f_freightCharges')||{}).value);var serv=num(($('f_serviceCharges')||{}).value)||num(($('f_servicesCharges')||{}).value)||num(($('f_extraCharges')||{}).value)||num(($('f_otherCharges')||{}).value);var subtotal=line+trans+serv;var discount=num(($('f_discount')||{}).value);var rate=num(($('f_taxRate')||{}).value)||num(($('f_vatRate')||{}).value);var tax=num(($('f_tax')||{}).value)||num(($('f_taxVat')||{}).value)||num(($('f_vat')||{}).value);if(rate){tax=(subtotal-discount)*rate/100}var total=Math.max(0,subtotal-discount+tax);if(line>0){setCalcValue('lineTotal',line);setCalcValue('amount',line)}setCalcValue('subtotal',subtotal);setCalcValue('tax',tax);setCalcValue('taxVat',tax);setCalcValue('vat',tax);setCalcValue('total',total)}
function saveRecord(){recalculateRecordTotals();var ce=$('f_customer')||$('f_customerName')||$('f_partyName')||$('f_to')||$('f_buyer');if(ce)fillCustomerFields(ce.value,true);if(!validateEmailInputs())return;var rec=editingId?state.records.find(function(x){return x.id===editingId}):null;var module=rec?rec.module:currentModule;var mod=MODULES[module];var fields={};(mod.fields||[]).forEach(function(k){var el=$('f_'+k);if(el)fields[k]=el.value});if($('f_itemsJSON'))fields.itemsJSON=$('f_itemsJSON').value;var status=$('f_status')?$('f_status').value:'';var jobRef=$('f_jobRef')?$('f_jobRef').value:'';if(rec){api('/api/record/update',{method:'POST',body:JSON.stringify({id:rec.id,number:($('f_number')?$('f_number').value:''),fields:fields,status:status,reason:($('changeReason')?$('changeReason').value:'Updated from UI')})}).then(function(){toast('Saved');closeModal();loadState()}).catch(function(e){toast(e.message)})}else{api('/api/record',{method:'POST',body:JSON.stringify({module:module,fields:fields,status:status,jobRef:jobRef})}).then(function(){toast('Created');closeModal();loadState()}).catch(function(e){toast(e.message)})}}
function setStatus(id,status){var reason=prompt('Reason for '+status+':')||status;api('/api/record/status',{method:'POST',body:JSON.stringify({id:id,status:status,reason:reason})}).then(function(){toast(status);loadState()}).catch(function(e){toast(e.message)})}
function convertRecord(id,target){api('/api/record/convert',{method:'POST',body:JSON.stringify({sourceId:id,targetModule:target})}).then(function(j){toast('Created '+j.record.number);currentModule=target;loadState()}).catch(function(e){toast(e.message)})}
function renderApprovals(){var rec=state.records.filter(function(r){return r.status==='Pending Approval'}).filter(matchesSearch);var html='<div class="panel"><div class="panel-head"><b>Pending Approval Queue</b><input placeholder="Search approvals" oninput="searchTerm=this.value;renderApprovals()"></div>'+tableRecords(rec,true)+'</div>';$('content').innerHTML=html}
function renderAudit(){var rows=state.audit.filter(function(a){return !searchTerm||JSON.stringify(a).toLowerCase().indexOf(searchTerm.toLowerCase())>=0});var html='<div class="panel"><div class="panel-head"><b>Audit Log ('+rows.length+')</b><input placeholder="Search audit" oninput="searchTerm=this.value;renderAudit()"></div><div class="table-wrap"><table><thead><tr><th>Time</th><th>User</th><th>Action</th><th>Module</th><th>Number</th><th>Details</th></tr></thead><tbody>';rows.forEach(function(a){html+='<tr><td>'+shortDate(a.time)+'</td><td>'+esc(a.user)+'</td><td><b>'+esc(a.action)+'</b></td><td>'+esc(a.module)+'</td><td>'+esc(a.number)+'</td><td>'+esc(a.details)+'</td></tr>'});html+='</tbody></table></div></div>';$('content').innerHTML=html}
function renderAI(){var html='<div class="split"><div class="panel ai-box"><div class="panel-head"><b>Document Text Extractor</b></div><div style="padding:16px"><p class="small-note">Paste text copied from a PDF, scan OCR, BL, invoice, bank statement or email. This MVP extracts common fields locally and does not approve payments.</p><textarea id="aiText" placeholder="Paste document text here..."></textarea><br><br><button class="btn" onclick="extractAI()">Extract Fields</button></div></div><div class="panel"><div class="panel-head"><b>Extracted Fields</b></div><div id="aiResult" style="padding:16px" class="muted">No extraction yet.</div></div></div>';$('content').innerHTML=html}
function extractAI(){api('/api/ai/extract',{method:'POST',body:JSON.stringify({text:$('aiText').value})}).then(function(j){var html='';Object.keys(j.extracted).forEach(function(k){html+='<div class="pill">'+pretty(k)+': '+esc(j.extracted[k])+'</div>'});if(!html)html='<p class="muted">No common fields found.</p>';$('aiResult').innerHTML=html+'<p class="small-note">'+esc((j.notes||[]).join(' '))+'</p>'}).catch(function(e){toast(e.message)})}
function renderUsers(){var html='<div class="split"><div class="panel"><div class="panel-head"><b>User Accounts</b></div><div class="table-wrap"><table><thead><tr><th>User</th><th>Name</th><th>Role</th><th>Department</th><th>Status</th></tr></thead><tbody>';state.users.forEach(function(u){html+='<tr><td>'+esc(u.username)+'</td><td>'+esc(u.displayName)+'</td><td>'+esc(u.role)+'</td><td>'+esc(u.department)+'</td><td>'+(u.active?'Active':'Inactive')+'</td></tr>'});html+='</tbody></table></div></div><div class="panel"><div class="panel-head"><b>Create User / Change Password</b></div><div style="padding:16px"><h3>Create User</h3><div class="field"><label>Username</label><input id="newUsername"></div><div class="field"><label>Display Name</label><input id="newDisplay"></div><div class="field"><label>Role</label><select id="newRole"><option>Owner/Admin</option><option>Manager</option><option>Accounting</option><option>Sales</option><option>Transport</option><option>Legal</option><option>Employee</option></select></div><div class="field"><label>Department</label><input id="newDept"></div><div class="field"><label>Password</label><input id="newPassword" type="password"></div><button class="btn" onclick="createUser()">Create User</button><hr><h3>Change My Password</h3><div class="field"><label>Old Password</label><input id="oldPass" type="password"></div><div class="field"><label>New Password</label><input id="newPass" type="password"></div><button class="btn danger" onclick="changePassword()">Change Password</button></div></div></div>';$('content').innerHTML=html}
function createUser(){api('/api/user',{method:'POST',body:JSON.stringify({username:$('newUsername').value,displayName:$('newDisplay').value,role:$('newRole').value,department:$('newDept').value,password:$('newPassword').value})}).then(function(){toast('User created');loadState()}).catch(function(e){toast(e.message)})}
function changePassword(){api('/api/user/password',{method:'POST',body:JSON.stringify({oldPassword:$('oldPass').value,newPassword:$('newPass').value})}).then(function(){toast('Password changed')}).catch(function(e){toast(e.message)})}
function openEmailModal(id){var r=state.records.find(function(x){return x.id===id});if(!r){toast('Document not found');return}var to=field(r,'customerEmail')||field(r,'email')||'';if(!to){var cname=(field(r,'customer')||field(r,'partyName')||field(r,'to')||'').toLowerCase();var c=customerRecords().find(function(x){return (field(x,'name')||'').toLowerCase()===cname});if(c)to=field(c,'email')}var subject=r.number+' - '+moduleName(r.module);var body='Dear Customer,\n\nPlease find attached '+r.number+'.\n\nRegards,';var html='<div class="modal-card"><div class="modal-head"><b>Send Document by Email</b><button class="btn secondary small" onclick="closeModal()">Close</button></div><div class="modal-body"><div class="form-grid"><div class="field"><label>To</label><input id="emailTo" value="'+esc(to)+'" placeholder="customer@example.com"></div><div class="field"><label>CC</label><input id="emailCc"></div><div class="field"><label>BCC</label><input id="emailBcc"></div><div class="field"><label>Subject</label><input id="emailSubject" value="'+esc(subject)+'"></div><div class="field span2"><label>Body</label><textarea id="emailBody">'+esc(body)+'</textarea></div><div class="field span2"><label><input type="checkbox" id="emailAttach" checked> Attach document PDF when available</label><p class="small-note">The system will attach an A4 PDF generated from the same print template when Edge/Chrome is available; otherwise it attaches the printable HTML.</p></div></div><br><button class="btn" onclick="sendEmail(\''+r.id+'\')">Send Email</button> <button class="btn secondary" onclick="closeModal()">Cancel</button></div></div>';$('modal').innerHTML=html;$('modal').classList.remove('hidden')}
function sendEmail(id){var payload={recordId:id,to:$('emailTo').value,cc:$('emailCc').value,bcc:$('emailBcc').value,subject:$('emailSubject').value,body:$('emailBody').value,attach:$('emailAttach').checked};api('/api/email/send',{method:'POST',body:JSON.stringify(payload)}).then(function(){toast('Email sent');closeModal();loadState()}).catch(function(e){toast(e.message)})}
function renderSettings(){
 var s=state.settings||{};var serial=state.serial||{};var prefixes=state.prefixes||{};var keysMap={};Object.keys(prefixes).forEach(function(k){keysMap[k]=1});Object.keys(serial).forEach(function(k){keysMap[k]=1});var serialKeys=Object.keys(keysMap).sort();
 var html='<div class="split"><div class="panel"><div class="panel-head"><b>System Info</b></div><div style="padding:16px" class="kpi-list"><div><span>App</span><b>'+esc(state.appName)+'</b></div><div><span>Version</span><b>'+esc(state.version)+'</b></div><div><span>Local Data Folder</span><b>'+esc(state.dataDir)+'</b></div><div><span>Records</span><b>'+state.records.length+'</b></div><div><span>Audit Entries</span><b>'+state.audit.length+'</b></div><div><span>Base Currency</span><b>'+esc(state.company.baseCurrency)+'</b></div></div><div style="padding:16px" class="small-note"><b>Security:</b> local login lockout, stronger password policy, HttpOnly session cookie, no permanent delete, audit log for email, serial and document changes.</div></div>';
 html+='<div class="panel"><div class="panel-head"><b>Company Profile & Images</b></div><div style="padding:16px"><p><b>'+esc(state.company.name)+'</b></p><p>'+esc(state.company.address)+'</p><p>'+esc(state.company.phone)+' | '+esc(state.company.email)+'</p><p class="small-note">Bank details are selected from Bank Accounts inside each document. QR verification uses the Company Verification Base URL.</p><p>Tax/VAT: '+esc(state.company.taxNumber)+'</p><button class="btn" onclick="openCompanyModal()">Edit Company / Stamp / Logo</button> <button class="btn secondary" onclick="window.open(&quot;/letterhead&quot;,&quot;_blank&quot;)">Open Letterhead</button></div></div></div>';
 html+='<div class="panel" style="margin-top:16px"><div class="panel-head"><b>Serial Number & Job Reference Settings</b><button class="btn small" onclick="saveSerialSettings()">Save Serial Settings</button></div><div style="padding:16px"><p class="small-note">Workflow example: Job Reference ZE-DESIGN-2026-0001, Quotation ZE-QTN-2026-0001, PI ZE-PI-2026-0001, SI ZE-SI-2026-0001, CI ZE-CI-2026-0001, PL ZE-PL-2026-0001. Tokens: {PREFIX}, {CUSTOMER}, {YEAR}, {YY}, {SEQ}, {JOBSEQ}, {MODULE}, {REV}.</p><div class="serial-grid"><div class="field"><label>Serial Format</label><input id="serialFormat" value="'+esc(s.serialFormat||'{PREFIX}-{MODULE}-{YEAR}-{JOBSEQ}')+'"></div><div class="field"><label>Serial Year</label><input id="serialYear" type="number" value="'+esc(s.serialYear||new Date().getFullYear())+'"></div><div class="field"><label>Serial Padding</label><input id="serialPadding" type="number" value="'+esc(s.serialPadding||5)+'"></div><div class="field"><label>Revision</label><input id="serialRevision" value="'+esc(s.serialRevision||'R0')+'"></div><div class="field"><label>Job Prefix</label><input id="jobPrefix" value="'+esc(s.jobPrefix||'ZEN')+'"></div><div class="field"><label>Job Padding</label><input id="jobPadding" type="number" value="'+esc(s.jobPadding||5)+'"></div></div><h3>Current Counters</h3><div class="table-wrap"><table><thead><tr><th>Module</th><th>Code</th><th>Current Counter</th></tr></thead><tbody>';
 serialKeys.forEach(function(k){html+='<tr><td>'+esc(moduleName(k))+'</td><td>'+esc(prefixes[k]||'')+'</td><td><input data-serial-key="'+esc(k)+'" type="number" min="0" value="'+esc(serial[k]||0)+'" style="max-width:120px"></td></tr>'});
 html+='</tbody></table></div></div></div>';
 html+='<div class="panel" style="margin-top:16px"><div class="panel-head"><b>Full Backup / Restore / Auto Backup</b><button class="btn small" onclick="saveBackupSettings()">Save Backup Settings</button></div><div style="padding:16px"><p class="small-note">Full backup includes database, documents, uploaded files, templates, QR verification records, user accounts and settings. Restore requires admin confirmation and replaces current data.</p><div class="serial-grid"><div class="field"><label>Auto Backup Frequency</label><select id="autoBackupFrequency"><option '+((s.autoBackupFrequency||'Off')==='Off'?'selected':'')+'>Off</option><option '+(s.autoBackupFrequency==='Daily'?'selected':'')+'>Daily</option><option '+(s.autoBackupFrequency==='Weekly'?'selected':'')+'>Weekly</option><option '+(s.autoBackupFrequency==='Monthly'?'selected':'')+'>Monthly</option></select></div><div class="field"><label>Local Backup Folder</label><input id="backupLocalPath" value="'+esc(s.backupLocalPath||'')+'" placeholder="Default app backups folder"></div><div class="field"><label>Cloud / Server Folder</label><input id="backupCloudPath" value="'+esc(s.backupCloudPath||'')+'" placeholder="Mounted drive or server path"></div><div class="field"><label>Retention Count</label><input id="backupRetentionCount" type="number" value="'+esc(s.backupRetentionCount||'10')+'"></div></div><div class="toolbar"><button class="btn" onclick="downloadFullBackup()">Download Full Backup ZIP</button><button class="btn secondary" onclick="runLocalBackup()">Create Local Backup Now</button><button class="btn danger" onclick="openRestoreModal()">Restore Backup</button></div><p class="small-note">Last auto backup: '+esc(s.lastAutoBackupAt||'Not yet')+'</p></div></div>';
 html+='<div class="panel" style="margin-top:16px"><div class="panel-head"><b>Email / SMTP Settings</b><button class="btn small" onclick="saveEmailSettings()">Save Email Settings</button></div><div style="padding:16px"><p class="small-note">Supports company SMTP, Microsoft 365, Google Workspace app password, or company mail server. Password stays in the local ERP data file; keep your computer secure.</p><div class="serial-grid"><div class="field"><label>SMTP Host</label><input id="smtpHost" value="'+esc(s.smtpHost||'')+'" placeholder="smtp.office365.com"></div><div class="field"><label>SMTP Port</label><input id="smtpPort" value="'+esc(s.smtpPort||'587')+'"></div><div class="field"><label>Security</label><select id="smtpSecurity"><option '+((s.smtpSecurity||'starttls')==='starttls'?'selected':'')+'>starttls</option><option '+(s.smtpSecurity==='ssl'?'selected':'')+'>ssl</option><option '+(s.smtpSecurity==='none'?'selected':'')+'>none</option></select></div><div class="field"><label>SMTP Username</label><input id="smtpUser" value="'+esc(s.smtpUser||'')+'"></div><div class="field"><label>SMTP Password</label><input id="smtpPassword" type="password" placeholder="'+(s.smtpPasswordSet==='yes'?'Saved, leave blank to keep':'Enter password/app password')+'"></div><div class="field"><label>From Email</label><input id="smtpFrom" value="'+esc(s.smtpFrom||state.company.email||'')+'"></div><div class="field"><label>From Name</label><input id="smtpFromName" value="'+esc(s.smtpFromName||state.company.name||'')+'"></div><div class="field span2"><label>Email Signature</label><textarea id="emailSignature">'+esc(s.emailSignature||'')+'</textarea></div><div class="toolbar"><button class="btn secondary" onclick="testSMTPConnection()">Test Connection</button><button class="btn secondary" onclick="sendSMTPTestEmail()">Send Test Email</button></div><p class="small-note" id="smtpTestResult"></p></div></div>';

  $('content').innerHTML=html
}
function testSMTPConnection(){saveEmailSettings();setTimeout(function(){api('/api/email/test-connection',{method:'POST',body:JSON.stringify({})}).then(function(j){toast(j.message||'SMTP connection ok');var el=$('smtpTestResult');if(el)el.textContent=j.message||'SMTP connection ok'}).catch(function(e){toast(e.message);var el=$('smtpTestResult');if(el)el.textContent=e.message})},300)}
function sendSMTPTestEmail(){var to=prompt('Send test email to:', $('smtpFrom')?$('smtpFrom').value:'');if(to===null)return;api('/api/email/send-test',{method:'POST',body:JSON.stringify({to:to})}).then(function(j){toast(j.message||'Test email sent');var el=$('smtpTestResult');if(el)el.textContent=j.message||'Test email sent'}).catch(function(e){toast(e.message);var el=$('smtpTestResult');if(el)el.textContent=e.message})}
function saveBackupSettings(){var settings={autoBackupFrequency:$('autoBackupFrequency').value,backupLocalPath:$('backupLocalPath').value,backupCloudPath:$('backupCloudPath').value,backupRetentionCount:$('backupRetentionCount').value};api('/api/backup/settings',{method:'POST',body:JSON.stringify(settings)}).then(function(){toast('Backup settings saved');loadState()}).catch(function(e){toast(e.message)})}
function saveEmailSettings(){var settings={smtpHost:$('smtpHost').value,smtpPort:$('smtpPort').value,smtpSecurity:$('smtpSecurity').value,smtpUser:$('smtpUser').value,smtpPassword:$('smtpPassword').value,smtpFrom:$('smtpFrom').value,smtpFromName:$('smtpFromName').value,emailSignature:$('emailSignature').value};api('/api/email/settings',{method:'POST',body:JSON.stringify(settings)}).then(function(){toast('Email settings saved');loadState()}).catch(function(e){toast(e.message)})}
function openCompanyModal(){
 var c=state.company;var keys=['name','legalName','logoText','stampText','slogan','address','city','country','phone','email','website','verificationBaseURL','taxNumber','baseCurrency','defaultNotes','defaultTerms','currencyList','prefix'];
 var html='<div class="modal-card"><div class="modal-head"><b>Company Settings, Stamp, Label and Signature</b><button class="btn secondary small" onclick="closeModal()">Close</button></div><div class="modal-body"><div class="form-grid">';
 keys.forEach(function(k){var span=(k==='address'||k==='defaultNotes'||k==='defaultTerms'||k==='slogan')?' span2':'';var value=esc(c[k]||'');if(k==='address'||k==='defaultNotes'||k==='defaultTerms'||k==='slogan'){html+='<div class="field'+span+'"><label>'+pretty(k)+'</label><textarea id="c_'+k+'">'+value+'</textarea></div>'}else{html+='<div class="field'+span+'"><label>'+pretty(k)+'</label><input id="c_'+k+'" value="'+value+'"></div>'}});
 html+=companyImageField('logoData','Company Logo')+companyImageField('stampData','Company Stamp')+companyImageField('labelData','Company Label Image')+companyImageField('signatureData','Authorized Signature Image');
 html+='</div><br><button class="btn" onclick="saveCompany()">Save Company</button> <button class="btn secondary" onclick="closeModal()">Cancel</button></div></div>';$('modal').innerHTML=html;$('modal').classList.remove('hidden')
}
function companyImageField(key,label){var data=(state.company&&state.company[key])||'';var img=data?'<img id="preview_'+key+'" class="img-preview" src="'+esc(data)+'">':'<img id="preview_'+key+'" class="img-preview hidden">';return '<div class="field"><label>'+label+'</label><input type="file" accept="image/*" onchange="readCompanyImage(this,\''+key+'\')"><input type="hidden" id="c_'+key+'" value="'+esc(data)+'">'+img+'<button type="button" class="btn secondary small" onclick="clearCompanyImage(\''+key+'\')">Clear</button></div>'}
function readCompanyImage(input,key){var f=input.files&&input.files[0];if(!f)return;if(f.size>2500000){toast('Image too large. Use under 2.5 MB.');input.value='';return}var r=new FileReader();r.onload=function(){var data=r.result;$('c_'+key).value=data;var img=$('preview_'+key);if(img){img.src=data;img.classList.remove('hidden')}};r.readAsDataURL(f)}
function clearCompanyImage(key){var el=$('c_'+key);if(el)el.value='';var img=$('preview_'+key);if(img){img.removeAttribute('src');img.classList.add('hidden')}}
function saveCompany(){var c=Object.assign({},state.company);['name','legalName','logoText','stampText','slogan','address','city','country','phone','email','website','verificationBaseURL','taxNumber','baseCurrency','bankName','bankAccount','bankIban','bankSwift','defaultNotes','defaultTerms','currencyList','prefix','logoData','stampData','labelData','signatureData'].forEach(function(k){var el=$('c_'+k);if(el)c[k]=el.value});c.whatsApp='';api('/api/company',{method:'POST',body:JSON.stringify(c)}).then(function(){toast('Company saved');closeModal();loadState()}).catch(function(e){toast(e.message)})}
function saveSerialSettings(){var serial={};document.querySelectorAll('[data-serial-key]').forEach(function(el){serial[el.getAttribute('data-serial-key')]=parseInt(el.value||'0',10)||0});var settings={serialFormat:$('serialFormat').value,serialYear:$('serialYear').value,serialPadding:$('serialPadding').value,serialRevision:$('serialRevision').value,jobPrefix:$('jobPrefix').value,jobPadding:$('jobPadding').value};api('/api/serial',{method:'POST',body:JSON.stringify({serial:serial,settings:settings})}).then(function(){toast('Serial settings saved');loadState()}).catch(function(e){toast(e.message)})}
function downloadBackup(){downloadFullBackup()}
function downloadFullBackup(){window.location='/api/backup/full'}
function runLocalBackup(){api('/api/backup/run',{method:'POST',body:JSON.stringify({})}).then(function(j){toast('Backup created: '+(j.path||''));loadState()}).catch(function(e){toast(e.message)})}
function openRestoreModal(){var html='<div class="modal-card"><div class="modal-head"><b>Restore Backup</b><button class="btn secondary small" onclick="closeModal()">Close</button></div><div class="modal-body"><p class="danger-text">Restoring replaces current ERP database and uploaded files. Admin confirmation is required.</p><input type="file" id="restoreFile" accept=".zip,.json,application/zip,application/json"><br><br><button class="btn danger" onclick="restoreBackup()">Restore Selected Backup</button></div></div>';$('modal').innerHTML=html;$('modal').classList.remove('hidden')}
function restoreBackup(){var f=$('restoreFile').files[0];if(!f){toast('Choose backup file');return}if(!confirm('This will replace current ERP data and uploads. Continue restore?'))return;f.arrayBuffer().then(function(buf){return fetch('/api/restore',{method:'POST',headers:{'Content-Type':'application/octet-stream'},body:buf})}).then(function(r){return r.json().then(function(j){if(!r.ok||j.ok===false)throw new Error(j.error||'Restore failed');return j})}).then(function(){toast('Restored');closeModal();loadState()}).catch(function(e){toast(e.message)})}
window.addEventListener('keydown',function(e){if(e.key==='Escape'){closeModal();var s=$('sideBar');if(s)s.classList.remove('open')}});
window.addEventListener('resize',function(){if(window.innerWidth>820){var s=$('sideBar');if(s)s.classList.remove('open')}});
loadState();
</script>
</body>
</html>`
