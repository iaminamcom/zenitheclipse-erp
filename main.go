package main

import (
	"archive/zip"
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"embed"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed web/* web/assets/*
var embeddedFiles embed.FS

const (
	appName         = "Zenith Eclipse ERP Ultimate"
	currentVersion  = "3.1.0-docker"
	defaultUsername = "admin"
	defaultPassword = "admin123"
)

type Record map[string]any

type User struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Username     string `json:"username"`
	FullName     string `json:"fullName"`
	Email        string `json:"email"`
	Phone        string `json:"phone"`
	Salt         string `json:"salt,omitempty"`
	PasswordHash string `json:"passwordHash,omitempty"`
	Role         string `json:"role"`
	Status       string `json:"status"`
	CreatedAt    string `json:"createdAt"`
	ApprovedBy   string `json:"approvedBy,omitempty"`
	ApprovedAt   string `json:"approvedAt,omitempty"`
	LastLoginAt  string `json:"lastLoginAt,omitempty"`
}

type DB struct {
	Version   string   `json:"version"`
	Company   Record   `json:"company"`
	Settings  Record   `json:"settings"`
	Customers []Record `json:"customers"`
	Suppliers []Record `json:"suppliers"`
	Products  []Record `json:"products"`
	Cases     []Record `json:"cases"`
	Documents []Record `json:"documents"`
	Payments  []Record `json:"payments"`
	Expenses  []Record `json:"expenses"`
	Shipments []Record `json:"shipments"`
	Accounts  []Record `json:"accounts"`
	Locks     []Record `json:"locks"`
	AuditLogs []Record `json:"auditLogs"`
	Users     []User   `json:"users"`
}

type PublicData struct {
	Version   string   `json:"version"`
	Company   Record   `json:"company"`
	Settings  Record   `json:"settings"`
	Customers []Record `json:"customers"`
	Suppliers []Record `json:"suppliers"`
	Products  []Record `json:"products"`
	Cases     []Record `json:"cases"`
	Documents []Record `json:"documents"`
	Payments  []Record `json:"payments"`
	Expenses  []Record `json:"expenses"`
	Shipments []Record `json:"shipments"`
	Accounts  []Record `json:"accounts"`
	Locks     []Record `json:"locks"`
	Audit     []Record `json:"audit"`
	AuditLogs []Record `json:"auditLogs"`
}

type Session struct {
	Username string
	Expires  time.Time
}

type Server struct {
	mu       sync.Mutex
	db       DB
	dbPath   string
	sessions map[string]Session
}

func main() {
	srv, err := newServer()
	if err != nil {
		log.Fatal(err)
	}

	staticFS, err := fs.Sub(embeddedFiles, "web")
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	mux.HandleFunc("/api/register", srv.handleRegister)
	mux.HandleFunc("/api/login", srv.handleLogin)
	mux.HandleFunc("/api/logout", srv.requireAuth(srv.handleLogout))
	mux.HandleFunc("/api/me", srv.handleMe)
	mux.HandleFunc("/api/data", srv.requireAuth(srv.handleData))
	mux.HandleFunc("/api/users", srv.requireAdmin(srv.handleUsers))
	mux.HandleFunc("/api/users/update", srv.requireAdmin(srv.handleUserUpdate))
	mux.HandleFunc("/api/change-password", srv.requireAuth(srv.handleChangePassword))
	mux.HandleFunc("/api/backup", srv.requireAuth(srv.handleBackup))
	mux.HandleFunc("/api/restore", srv.requireAdmin(srv.handleRestore))
	mux.HandleFunc("/api/export/csv", srv.requireAuth(srv.handleCSVExport))
	mux.HandleFunc("/api/export/xlsx", srv.requireAuth(srv.handleXLSXExport))
	mux.HandleFunc("/document/", srv.requireAuth(srv.handleDocumentPrint))
	mux.HandleFunc("/verify/", srv.handleVerify)
	mux.Handle("/", http.FileServer(http.FS(staticFS)))

	ln, err := srv.listen()
	if err != nil {
		log.Fatal(err)
	}
	addr := ln.Addr().String()
	url := srv.publicURL(addr)

	fmt.Println(appName)
	fmt.Println("Address:", url)
	fmt.Println("Default admin:", defaultUsername, "/", displayAdminPasswordHint())
	fmt.Println("Data file:", srv.dbPath)
	fmt.Println("For server use: set ZENITH_ERP_ADDR=0.0.0.0:8080 and ZENITH_ERP_BROWSER=0")
	if os.Getenv("ZENITH_ERP_BROWSER") != "0" {
		openBrowser(url)
	}

	server := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second, MaxHeaderBytes: 1 << 20}
	if err := server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func (s *Server) listen() (net.Listener, error) {
	addr := strings.TrimSpace(os.Getenv("ZENITH_ERP_ADDR"))
	if addr != "" {
		return net.Listen("tcp", addr)
	}
	return listenFirstAvailable(6799, 6810)
}

func (s *Server) publicURL(addr string) string {
	if strings.HasPrefix(addr, "127.0.0.1:") || strings.HasPrefix(addr, "localhost:") || strings.HasPrefix(addr, "[::1]:") {
		return "http://" + addr
	}
	if strings.HasPrefix(addr, ":") {
		return "http://127.0.0.1" + addr
	}
	if strings.HasPrefix(addr, "0.0.0.0:") || strings.HasPrefix(addr, "[::]:") {
		parts := strings.Split(addr, ":")
		port := parts[len(parts)-1]
		return "http://YOUR-SERVER-IP:" + port
	}
	return "http://" + addr
}

func newServer() (*Server, error) {
	dir, err := appDataDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "data.json")
	srv := &Server{dbPath: path, sessions: map[string]Session{}}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		srv.db = defaultDB()
		if err := srv.saveLocked(); err != nil {
			return nil, err
		}
	} else if err := srv.load(); err != nil {
		return nil, err
	}
	return srv, nil
}

func appDataDir() (string, error) {
	if custom := strings.TrimSpace(os.Getenv("ZENITH_ERP_DATA")); custom != "" {
		return custom, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "ZenithEclipseERPPro"), nil
}

func initialAdminPassword() string {
	password := strings.TrimSpace(os.Getenv("ZENITH_ERP_ADMIN_PASSWORD"))
	if password == "" {
		return defaultPassword
	}
	return password
}

func displayAdminPasswordHint() string {
	if strings.TrimSpace(os.Getenv("ZENITH_ERP_ADMIN_PASSWORD")) != "" {
		return "from ZENITH_ERP_ADMIN_PASSWORD"
	}
	return defaultPassword
}

func cookieSecureEnabled(r *http.Request) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("ZENITH_ERP_COOKIE_SECURE")))
	switch v {
	case "1", "true", "yes", "on", "secure":
		return true
	case "0", "false", "no", "off", "insecure":
		return false
	}
	proto := strings.ToLower(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")))
	return proto == "https" || r.TLS != nil
}

func defaultDB() DB {
	now := time.Now().Format(time.RFC3339)
	year := time.Now().Year()
	base := "ZE-HRBTC-" + strconv.Itoa(year) + "-0001"
	salt := randomToken(16)
	return DB{
		Version: currentVersion,
		Company: Record{
			"name":             "ZENITH ECLIPSE CO",
			"legalName":        "ZENITH ECLIPSE CO",
			"slogan":           "NURTURING FIELDS OF TOMORROW WEAVING WORLDWIDE PATHWAYS OF PROSPERITY",
			"address":          "Citadel Tower, office 204, Business Bay, Dubai, UAE",
			"city":             "Dubai",
			"country":          "United Arab Emirates",
			"phone":            "+93 77 404 7259",
			"whatsApp":         "+93 77 404 7259",
			"email":            "sales@zenitheclipse.com",
			"website":          "http://www.zenitheclipse.com",
			"taxId":            "",
			"trn":              "",
			"bankName":         "",
			"bankAccount":      "",
			"bankIban":         "",
			"bankSwift":        "",
			"baseCurrency":     "USD",
			"currencyList":     "USD,AED,AFN,CNY,EUR,GBP,OMR",
			"prefix":           "ZE",
			"serialFormat":     "ZE-{CUSTOMER}-{YEAR}-{SEQ}",
			"serialSeq":        float64(2),
			"serialYear":       float64(year),
			"serialPadding":    float64(4),
			"autoApproveUsers": false,
			"logoUrl":          "/assets/logo.png",
			"leafUrl":          "/assets/leaf.png",
			"stampText":        "Authorized Signature",
			"defaultTerms":     "Payment terms are subject to final written confirmation. Bank charges are borne by the sender unless agreed otherwise. All documents remain valid only with the official serial number and verification code.",
			"defaultNotes":     "Thank you for your business.",
		},
		Settings: Record{
			"signupsEnabled":   true,
			"defaultRole":      "staff",
			"approvalRequired": true,
			"lockBeforeDate":   "",
			"revisionMode":     true,
			"serverMode":       true,
		},
		Customers: []Record{{"id": "cus-demo-1", "type": "customer", "code": "HRBTC", "name": "Haroon Rezwan and Bradaran Amar khil Trade Co", "contact": "MR. Abdul Qasum", "email": "", "phone": "", "address": "Shop# 19, Faisal Sharif Market, Mandawi, Kabul AFG", "city": "Kabul", "country": "Afghanistan", "currency": "USD", "taxId": "", "notes": "Sample customer from uploaded letterhead", "createdAt": now, "updatedAt": now}},
		Suppliers: []Record{{"id": "sup-demo-1", "type": "supplier", "code": "SUP", "name": "Demo Supplier Co.", "contact": "Sales Team", "email": "supplier@example.com", "phone": "+86 000 0000", "address": "Shenzhen", "city": "Shenzhen", "country": "China", "currency": "USD", "taxId": "", "notes": "", "createdAt": now, "updatedAt": now}},
		Products:  defaultProducts(now),
		Cases:     []Record{{"id": "case-demo-1", "baseNumber": base, "baseSerial": base, "title": "Demo trade and logistics deal", "customerId": "cus-demo-1", "supplierId": "", "owner": defaultUsername, "status": "draft", "priority": "normal", "createdAt": now, "updatedAt": now, "notes": "Sample same-serial business chain."}},
		Documents: []Record{{"id": "doc-demo-qtn", "caseId": "case-demo-1", "baseNumber": base, "baseSerial": base, "chainId": base, "type": "quotation", "dealMode": "combined", "number": base + "-QTN-R0", "revision": float64(0), "date": time.Now().Format("2006-01-02"), "validUntil": time.Now().AddDate(0, 0, 14).Format("2006-01-02"), "dueDate": "", "customerId": "cus-demo-1", "supplierId": "", "currency": "USD", "exchangeRate": float64(1), "status": "draft", "approvalStatus": "draft", "incoterm": "CIF", "pol": "Dubai", "pod": "Kabul", "containerNo": "", "sealNo": "", "blNo": "", "discount": float64(0), "taxRate": float64(0), "shipping": float64(0), "notes": "Demo quotation with both product and transportation lines", "terms": "Payment and delivery terms are subject to final confirmation.", "items": []Record{{"productId": "prd-fertilizer", "itemKind": "product", "category": "product", "description": "Agricultural Product / Fertilizer", "hsCode": "", "unit": "Bag", "qty": float64(1000), "unitCost": float64(0), "unitPrice": float64(0), "netWeight": float64(0), "grossWeight": float64(0), "packages": float64(1000)}, {"productId": "prd-sea-20", "itemKind": "transport", "category": "transport", "description": "Sea Freight 20FT Container", "unit": "Container", "qty": float64(1), "unitCost": float64(900), "unitPrice": float64(1200), "packages": float64(1)}}, "createdAt": now, "updatedAt": now, "createdBy": defaultUsername, "updatedBy": defaultUsername}},
		Payments:  []Record{},
		Expenses:  []Record{},
		Shipments: []Record{},
		Accounts:  []Record{{"id": "acc-cash-usd", "name": "Cash USD", "type": "cash", "currency": "USD", "balance": float64(0)}, {"id": "acc-bank-aed", "name": "Bank AED", "type": "bank", "currency": "AED", "balance": float64(0)}},
		Locks:     []Record{},
		AuditLogs: []Record{{"id": "aud-boot", "time": now, "at": now, "user": "system", "action": "System created", "entity": "database", "details": "Zenith Eclipse ERP Ultimate database initialized"}},
		Users:     []User{{ID: "admin", Name: "Administrator", Username: defaultUsername, FullName: "Administrator", Email: "", Salt: salt, PasswordHash: hashPassword(salt, initialAdminPassword()), Role: "admin", Status: "active", CreatedAt: now, ApprovedAt: now, ApprovedBy: "system"}},
	}
}

func defaultProducts(now string) []Record {
	return []Record{
		{"id": "prd-goods-general", "category": "product", "sku": "PRD-GEN", "name": "General Trading Product", "description": "Physical goods/product line with HS code, quantity, cartons and weight", "hsCode": "", "unit": "Unit", "costPrice": float64(0), "salePrice": float64(0), "currency": "USD", "taxRate": float64(0), "stockQty": float64(0), "minStock": float64(0), "warehouse": "Dubai Warehouse", "notes": "", "createdAt": now, "updatedAt": now},
		{"id": "prd-fertilizer", "category": "product", "sku": "PRD-AGR-001", "name": "Agricultural Product / Fertilizer", "description": "Sample product line for product sale quotations and invoices", "hsCode": "", "unit": "Bag", "costPrice": float64(0), "salePrice": float64(0), "currency": "USD", "taxRate": float64(0), "stockQty": float64(0), "minStock": float64(0), "warehouse": "Dubai Warehouse", "notes": "", "createdAt": now, "updatedAt": now},
		{"id": "prd-sea-20", "category": "transport", "sku": "TRN-SEA-20", "name": "Sea Freight 20FT Container", "description": "Sea freight service for one 20FT container", "hsCode": "", "unit": "Container", "costPrice": float64(900), "salePrice": float64(1200), "currency": "USD", "taxRate": float64(0), "stockQty": float64(0), "minStock": float64(0), "warehouse": "Logistics Desk", "notes": "", "createdAt": now, "updatedAt": now},
		{"id": "prd-truck", "category": "transport", "sku": "TRN-TRUCK", "name": "Truck Transportation", "description": "Road transportation / trucking service", "hsCode": "", "unit": "Trip", "costPrice": float64(0), "salePrice": float64(0), "currency": "USD", "taxRate": float64(0), "stockQty": float64(0), "minStock": float64(0), "warehouse": "Logistics Desk", "notes": "", "createdAt": now, "updatedAt": now},
		{"id": "prd-doc-clear", "category": "service", "sku": "SRV-DOC-CLR", "name": "Customs Clearance & Documentation", "description": "Customs documentation and clearance service", "hsCode": "", "unit": "Service", "costPrice": float64(80), "salePrice": float64(150), "currency": "USD", "taxRate": float64(0), "stockQty": float64(0), "minStock": float64(0), "warehouse": "Office", "notes": "", "createdAt": now, "updatedAt": now},
	}
}

func ensureDefaultCatalog(products []Record) []Record {
	now := time.Now().Format(time.RFC3339)
	seen := map[string]bool{}
	for i := range products {
		products[i]["category"] = normalizeLineKindGo(firstNonEmpty(rstr(products[i], "category"), inferLineKindGo(products[i])))
		if rstr(products[i], "currency") == "" {
			products[i]["currency"] = "USD"
		}
		if rstr(products[i], "unit") == "" {
			if rstr(products[i], "category") == "transport" {
				products[i]["unit"] = "Trip"
			} else if rstr(products[i], "category") == "service" {
				products[i]["unit"] = "Service"
			} else {
				products[i]["unit"] = "Unit"
			}
		}
		seen[strings.ToUpper(rstr(products[i], "sku"))] = true
	}
	for _, def := range defaultProducts(now) {
		if !seen[strings.ToUpper(rstr(def, "sku"))] {
			products = append(products, def)
		}
	}
	return products
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func (s *Server) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := os.ReadFile(s.dbPath)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, &s.db); err != nil {
		return err
	}
	s.normalizeLocked()
	return s.saveLocked()
}

func (s *Server) normalizeLocked() {
	if s.db.Version == "" {
		s.db.Version = currentVersion
	}
	// Mark the database as the latest app schema while preserving existing records.
	s.db.Version = currentVersion
	if s.db.Company == nil {
		s.db.Company = defaultDB().Company
	}
	if s.db.Settings == nil {
		s.db.Settings = defaultDB().Settings
	}
	if _, ok := s.db.Settings["signupsEnabled"]; !ok {
		s.db.Settings["signupsEnabled"] = true
	}
	if _, ok := s.db.Settings["defaultRole"]; !ok {
		s.db.Settings["defaultRole"] = "staff"
	}

	if _, ok := s.db.Company["prefix"]; !ok {
		s.db.Company["prefix"] = "ZE"
	}
	if _, ok := s.db.Company["serialFormat"]; !ok {
		s.db.Company["serialFormat"] = "ZE-{CUSTOMER}-{YEAR}-{SEQ}"
	}
	if _, ok := s.db.Company["serialSeq"]; !ok {
		s.db.Company["serialSeq"] = float64(1)
	}
	if _, ok := s.db.Company["serialYear"]; !ok {
		s.db.Company["serialYear"] = float64(time.Now().Year())
	}
	if _, ok := s.db.Company["serialPadding"]; !ok {
		s.db.Company["serialPadding"] = float64(4)
	}
	if _, ok := s.db.Company["currencyList"]; !ok {
		s.db.Company["currencyList"] = "USD,AED,AFN,CNY,EUR,GBP,OMR"
	}
	if _, ok := s.db.Company["logoUrl"]; !ok {
		s.db.Company["logoUrl"] = "/assets/logo.png"
	}
	if _, ok := s.db.Company["leafUrl"]; !ok {
		s.db.Company["leafUrl"] = "/assets/leaf.png"
	}
	for i := range s.db.Customers {
		if rstr(s.db.Customers[i], "code") == "" {
			s.db.Customers[i]["code"] = makeCode(rstr(s.db.Customers[i], "name"))
		}
	}
	for i := range s.db.Suppliers {
		if rstr(s.db.Suppliers[i], "code") == "" {
			s.db.Suppliers[i]["code"] = makeCode(rstr(s.db.Suppliers[i], "name"))
		}
	}
	s.db.Products = ensureDefaultCatalog(s.db.Products)
	for i := range s.db.Documents {
		items := itemsOf(s.db.Documents[i])
		for j := range items {
			items[j]["itemKind"] = normalizeLineKindGo(firstNonEmpty(rstr(items[j], "itemKind"), rstr(items[j], "category"), inferLineKindGo(items[j])))
			items[j]["category"] = rstr(items[j], "itemKind")
		}
		s.db.Documents[i]["items"] = items
		if rstr(s.db.Documents[i], "dealMode") == "" {
			s.db.Documents[i]["dealMode"] = inferDealModeGo(items)
		}
	}

	if len(s.db.Users) == 0 {
		salt := randomToken(16)
		now := time.Now().Format(time.RFC3339)
		s.db.Users = []User{{ID: "admin", Name: "Administrator", Username: defaultUsername, FullName: "Administrator", Salt: salt, PasswordHash: hashPassword(salt, initialAdminPassword()), Role: "admin", Status: "active", CreatedAt: now, ApprovedAt: now, ApprovedBy: "system"}}
	}
	for i := range s.db.Users {
		if s.db.Users[i].ID == "" {
			s.db.Users[i].ID = s.db.Users[i].Username
		}
		if s.db.Users[i].Name == "" {
			s.db.Users[i].Name = s.db.Users[i].FullName
		}
		if s.db.Users[i].FullName == "" {
			s.db.Users[i].FullName = s.db.Users[i].Name
		}
		if s.db.Users[i].Status == "" {
			s.db.Users[i].Status = "active"
		}
		if s.db.Users[i].Role == "" {
			s.db.Users[i].Role = "staff"
		}
	}
}

func (s *Server) saveLocked() error {
	b, err := json.MarshalIndent(s.db, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.dbPath + ".tmp"
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, s.dbPath)
}

func publicData(db DB) PublicData {
	return PublicData{Version: db.Version, Company: db.Company, Settings: db.Settings, Customers: db.Customers, Suppliers: db.Suppliers, Products: db.Products, Cases: db.Cases, Documents: db.Documents, Payments: db.Payments, Expenses: db.Expenses, Shipments: db.Shipments, Accounts: db.Accounts, Locks: db.Locks, Audit: db.AuditLogs, AuditLogs: db.AuditLogs}
}

func applyPublicData(db *DB, p PublicData) {
	if p.Version == "" {
		p.Version = db.Version
	}
	db.Version = p.Version
	if p.Company != nil {
		db.Company = p.Company
	}
	if p.Settings != nil {
		db.Settings = p.Settings
	}
	db.Customers = p.Customers
	db.Suppliers = p.Suppliers
	db.Products = p.Products
	db.Cases = p.Cases
	db.Documents = p.Documents
	db.Payments = p.Payments
	db.Expenses = p.Expenses
	db.Shipments = p.Shipments
	db.Accounts = p.Accounts
	db.Locks = p.Locks
	if len(p.Audit) > 0 {
		db.AuditLogs = p.Audit
	} else {
		db.AuditLogs = p.AuditLogs
	}
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Name     string `json:"name"`
		FullName string `json:"fullName"`
		Username string `json:"username"`
		Email    string `json:"email"`
		Phone    string `json:"phone"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, Record{"error": "invalid request"})
		return
	}
	req.Username = strings.ToLower(strings.TrimSpace(req.Username))
	req.Email = strings.TrimSpace(req.Email)
	req.FullName = strings.TrimSpace(req.FullName)
	if req.FullName == "" {
		req.FullName = strings.TrimSpace(req.Name)
	}
	req.Phone = strings.TrimSpace(req.Phone)
	if len(req.Username) < 3 || len(req.Password) < 6 {
		writeJSON(w, http.StatusBadRequest, Record{"error": "username must be 3+ characters and password 6+ characters"})
		return
	}
	if !validUsername(req.Username) {
		writeJSON(w, http.StatusBadRequest, Record{"error": "username can use letters, numbers, dot, dash and underscore only"})
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !boolVal(s.db.Settings["signupsEnabled"], true) {
		writeJSON(w, http.StatusForbidden, Record{"error": "self signup is disabled"})
		return
	}
	for _, u := range s.db.Users {
		if strings.EqualFold(u.Username, req.Username) {
			writeJSON(w, http.StatusConflict, Record{"error": "username already exists"})
			return
		}
	}
	role := strVal(s.db.Settings["defaultRole"])
	if role == "" {
		role = "staff"
	}
	status := "pending"

	if len(s.db.Users) == 0 {
		role = "admin"
		status = "active"
	}
	now := time.Now().Format(time.RFC3339)
	salt := randomToken(16)
	s.db.Users = append(s.db.Users, User{ID: req.Username, Name: req.FullName, Username: req.Username, FullName: req.FullName, Email: req.Email, Phone: req.Phone, Salt: salt, PasswordHash: hashPassword(salt, req.Password), Role: role, Status: status, CreatedAt: now})
	s.addAuditLocked("system", "Employee signup", "user", req.Username, "New account requested: "+req.Username)
	if err := s.saveLocked(); err != nil {
		writeJSON(w, http.StatusInternalServerError, Record{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, Record{"ok": true, "status": status})
}

func validUsername(s string) bool {
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, Record{"error": "invalid request"})
		return
	}
	username := strings.TrimSpace(req.Username)
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, u := range s.db.Users {
		if strings.EqualFold(u.Username, username) && u.PasswordHash == hashPassword(u.Salt, req.Password) {
			if !isActiveStatus(u.Status) {
				writeJSON(w, http.StatusForbidden, Record{"error": "account is " + u.Status + ". Admin must approve it first."})
				return
			}
			token := randomToken(32)
			expires := time.Now().Add(24 * time.Hour)
			s.sessions[token] = Session{Username: u.Username, Expires: expires}
			s.db.Users[i].LastLoginAt = time.Now().Format(time.RFC3339)
			s.addAuditLocked(u.Username, "Login", "user", u.Username, "User logged in")
			_ = s.saveLocked()
			http.SetCookie(w, &http.Cookie{Name: "zenith_session", Value: token, Path: "/", HttpOnly: true, Secure: cookieSecureEnabled(r), SameSite: http.SameSiteLaxMode, Expires: expires})
			writeJSON(w, http.StatusOK, Record{"ok": true, "user": sanitizeUser(s.db.Users[i])})
			return
		}
	}
	writeJSON(w, http.StatusUnauthorized, Record{"error": "wrong username or password"})
}

func isActiveStatus(status string) bool {
	status = strings.ToLower(strings.TrimSpace(status))
	return status == "" || status == "active" || status == "approved"
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie("zenith_session"); err == nil {
		s.mu.Lock()
		delete(s.sessions, c.Value)
		s.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: "zenith_session", Value: "", Path: "/", Secure: cookieSecureEnabled(r), Expires: time.Unix(0, 0), MaxAge: -1})
	writeJSON(w, http.StatusOK, Record{"ok": true})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	u, ok := s.currentUserObj(r)
	if !ok {
		s.mu.Lock()
		signups := boolVal(s.db.Settings["signupsEnabled"], true)
		s.mu.Unlock()
		writeJSON(w, http.StatusUnauthorized, Record{"authenticated": false, "signupsEnabled": signups})
		return
	}
	s.mu.Lock()
	signups := boolVal(s.db.Settings["signupsEnabled"], true)
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, Record{"authenticated": true, "user": sanitizeUser(u), "signupsEnabled": signups})
}

func (s *Server) handleData(w http.ResponseWriter, r *http.Request) {
	u, ok := s.currentUserObj(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, Record{"error": "not logged in"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.mu.Lock()
		data := publicData(s.db)
		s.mu.Unlock()
		writeJSON(w, http.StatusOK, data)
	case http.MethodPost, http.MethodPut:
		if strings.EqualFold(u.Role, "viewer") {
			writeJSON(w, http.StatusForbidden, Record{"error": "viewer role cannot save changes"})
			return
		}
		var p PublicData
		if err := json.NewDecoder(io.LimitReader(r.Body, 60<<20)).Decode(&p); err != nil {
			writeJSON(w, http.StatusBadRequest, Record{"error": "invalid JSON"})
			return
		}
		s.mu.Lock()
		applyPublicData(&s.db, p)
		s.normalizeLocked()
		s.addAuditLocked(u.Username, "Data saved", "database", "all", "Business data saved from browser")
		err := s.saveLocked()
		s.mu.Unlock()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, Record{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, Record{"ok": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleUsers(w http.ResponseWriter, r *http.Request) {
	admin, _ := s.currentUserObj(r)
	switch r.Method {
	case http.MethodGet:
		s.mu.Lock()
		out := make([]Record, 0, len(s.db.Users))
		for _, u := range s.db.Users {
			out = append(out, sanitizeUser(u))
		}
		s.mu.Unlock()
		writeJSON(w, http.StatusOK, out)
	case http.MethodPost, http.MethodPut:
		var req struct {
			Username string `json:"username"`
			Action   string `json:"action"`
			Role     string `json:"role"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, Record{"error": "invalid request"})
			return
		}
		req.Username = strings.TrimSpace(req.Username)
		req.Action = strings.ToLower(strings.TrimSpace(req.Action))
		req.Role = strings.ToLower(strings.TrimSpace(req.Role))
		s.mu.Lock()
		defer s.mu.Unlock()
		for i := range s.db.Users {
			if strings.EqualFold(s.db.Users[i].Username, req.Username) {
				switch req.Action {
				case "approve", "enable":
					s.db.Users[i].Status = "active"
					s.db.Users[i].ApprovedBy = admin.Username
					s.db.Users[i].ApprovedAt = time.Now().Format(time.RFC3339)
				case "disable":
					if strings.EqualFold(s.db.Users[i].Username, admin.Username) {
						writeJSON(w, http.StatusBadRequest, Record{"error": "you cannot disable your own account"})
						return
					}
					s.db.Users[i].Status = "disabled"
				case "reject":
					s.db.Users[i].Status = "rejected"
				case "role":
					if req.Role == "" {
						writeJSON(w, http.StatusBadRequest, Record{"error": "role required"})
						return
					}
					s.db.Users[i].Role = req.Role
				default:
					writeJSON(w, http.StatusBadRequest, Record{"error": "unknown action"})
					return
				}
				s.addAuditLocked(admin.Username, "User "+req.Action, "user", s.db.Users[i].Username, "Admin changed user account")
				if err := s.saveLocked(); err != nil {
					writeJSON(w, http.StatusInternalServerError, Record{"error": err.Error()})
					return
				}
				writeJSON(w, http.StatusOK, Record{"ok": true})
				return
			}
		}
		writeJSON(w, http.StatusNotFound, Record{"error": "user not found"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleUserUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	admin, _ := s.currentUserObj(r)
	var req struct {
		ID            string `json:"id"`
		Name          string `json:"name"`
		Email         string `json:"email"`
		Phone         string `json:"phone"`
		Role          string `json:"role"`
		Status        string `json:"status"`
		ResetPassword string `json:"resetPassword"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, Record{"error": "invalid request"})
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.db.Users {
		if s.db.Users[i].ID == req.ID || s.db.Users[i].Username == req.ID {
			if req.Name != "" {
				s.db.Users[i].Name = strings.TrimSpace(req.Name)
				s.db.Users[i].FullName = s.db.Users[i].Name
			}
			if req.Email != "" {
				s.db.Users[i].Email = strings.TrimSpace(req.Email)
			}
			if req.Phone != "" {
				s.db.Users[i].Phone = strings.TrimSpace(req.Phone)
			}
			if req.Role != "" {
				s.db.Users[i].Role = strings.ToLower(strings.TrimSpace(req.Role))
			}
			if req.Status != "" {
				if strings.EqualFold(s.db.Users[i].Username, admin.Username) && req.Status != "active" {
					writeJSON(w, http.StatusBadRequest, Record{"error": "you cannot suspend your own account"})
					return
				}
				s.db.Users[i].Status = strings.ToLower(strings.TrimSpace(req.Status))
				if s.db.Users[i].Status == "active" && s.db.Users[i].ApprovedAt == "" {
					s.db.Users[i].ApprovedAt = time.Now().Format(time.RFC3339)
					s.db.Users[i].ApprovedBy = admin.Username
				}
			}
			if req.ResetPassword != "" {
				if len(req.ResetPassword) < 6 {
					writeJSON(w, http.StatusBadRequest, Record{"error": "password must be at least 6 characters"})
					return
				}
				salt := randomToken(16)
				s.db.Users[i].Salt = salt
				s.db.Users[i].PasswordHash = hashPassword(salt, req.ResetPassword)
			}
			s.addAuditLocked(admin.Username, "User updated", "user", s.db.Users[i].Username, "Admin updated user account")
			if err := s.saveLocked(); err != nil {
				writeJSON(w, http.StatusInternalServerError, Record{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, sanitizeUser(s.db.Users[i]))
			return
		}
	}
	writeJSON(w, http.StatusNotFound, Record{"error": "user not found"})
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	u, ok := s.currentUserObj(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, Record{"error": "not logged in"})
		return
	}
	var req struct {
		OldPassword string `json:"oldPassword"`
		NewPassword string `json:"newPassword"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, Record{"error": "invalid request"})
		return
	}
	if len(req.NewPassword) < 6 {
		writeJSON(w, http.StatusBadRequest, Record{"error": "new password must be at least 6 characters"})
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, user := range s.db.Users {
		if user.Username == u.Username {
			if user.PasswordHash != hashPassword(user.Salt, req.OldPassword) {
				writeJSON(w, http.StatusUnauthorized, Record{"error": "old password is wrong"})
				return
			}
			salt := randomToken(16)
			s.db.Users[i].Salt = salt
			s.db.Users[i].PasswordHash = hashPassword(salt, req.NewPassword)
			s.addAuditLocked(u.Username, "Password changed", "user", u.Username, "User changed password")
			if err := s.saveLocked(); err != nil {
				writeJSON(w, http.StatusInternalServerError, Record{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, Record{"ok": true})
			return
		}
	}
	writeJSON(w, http.StatusNotFound, Record{"error": "user not found"})
}

func (s *Server) handleBackup(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	b, err := json.MarshalIndent(s.db, "", "  ")
	s.mu.Unlock()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, Record{"error": err.Error()})
		return
	}
	filename := "zenith-eclipse-erp-backup-" + time.Now().Format("2006-01-02-150405") + ".json"
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)
	_, _ = w.Write(b)
}

func (s *Server) handleRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	admin, _ := s.currentUserObj(r)
	b, err := io.ReadAll(io.LimitReader(r.Body, 60<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, Record{"error": "could not read upload"})
		return
	}
	var incoming DB
	if err := json.Unmarshal(b, &incoming); err != nil {
		var p PublicData
		if err2 := json.Unmarshal(b, &p); err2 != nil {
			writeJSON(w, http.StatusBadRequest, Record{"error": "backup is not valid JSON"})
			return
		}
		s.mu.Lock()
		applyPublicData(&s.db, p)
		s.normalizeLocked()
		s.addAuditLocked(admin.Username, "Restore", "database", "public", "Public data restored")
		err = s.saveLocked()
		s.mu.Unlock()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, Record{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, Record{"ok": true})
		return
	}
	s.mu.Lock()
	if len(incoming.Users) == 0 {
		incoming.Users = s.db.Users
	}
	s.db = incoming
	s.normalizeLocked()
	s.addAuditLocked(admin.Username, "Restore", "database", "full", "Full backup restored")
	err = s.saveLocked()
	s.mu.Unlock()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, Record{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, Record{"ok": true})
}

func (s *Server) handleCSVExport(w http.ResponseWriter, r *http.Request) {
	table := strings.ToLower(r.URL.Query().Get("table"))
	if table == "" {
		table = "documents"
	}
	s.mu.Lock()
	db := s.db
	s.mu.Unlock()
	var buf bytes.Buffer
	cw := csv.NewWriter(&buf)
	switch table {
	case "customers":
		_ = cw.Write([]string{"Name", "Contact", "Email", "Phone", "Country", "Currency", "Tax ID", "Notes"})
		for _, x := range db.Customers {
			_ = cw.Write([]string{rstr(x, "name"), rstr(x, "contact"), rstr(x, "email"), rstr(x, "phone"), rstr(x, "country"), rstr(x, "currency"), rstr(x, "taxId"), rstr(x, "notes")})
		}
	case "suppliers":
		_ = cw.Write([]string{"Name", "Contact", "Email", "Phone", "Country", "Currency", "Tax ID", "Notes"})
		for _, x := range db.Suppliers {
			_ = cw.Write([]string{rstr(x, "name"), rstr(x, "contact"), rstr(x, "email"), rstr(x, "phone"), rstr(x, "country"), rstr(x, "currency"), rstr(x, "taxId"), rstr(x, "notes")})
		}
	case "products":
		_ = cw.Write([]string{"Category", "SKU", "Name", "Description", "HS Code", "Unit", "Cost", "Sale", "Currency", "Stock", "Warehouse", "Notes"})
		for _, p := range db.Products {
			_ = cw.Write([]string{lineKindLabelGo(rstr(p, "category")), rstr(p, "sku"), rstr(p, "name"), rstr(p, "description"), rstr(p, "hsCode"), rstr(p, "unit"), fmt2(rnum(p, "costPrice")), fmt2(rnum(p, "salePrice")), rstr(p, "currency"), fmt2(rnum(p, "stockQty")), rstr(p, "warehouse"), rstr(p, "notes")})
		}
	case "payments":
		_ = cw.Write([]string{"Type", "Date", "Party", "Document", "Amount", "Currency", "Account", "Method", "Reference", "Notes"})
		for _, p := range db.Payments {
			_ = cw.Write([]string{rstr(p, "type"), rstr(p, "date"), partyName(db, rstr(p, "partyType"), rstr(p, "partyId")), docNumber(db, rstr(p, "documentId")), fmt2(rnum(p, "amount")), rstr(p, "currency"), rstr(p, "account"), rstr(p, "method"), rstr(p, "reference"), rstr(p, "notes")})
		}
	case "expenses":
		_ = cw.Write([]string{"Date", "Category", "Vendor", "Amount", "Currency", "Account", "Notes"})
		for _, e := range db.Expenses {
			_ = cw.Write([]string{rstr(e, "date"), rstr(e, "category"), rstr(e, "vendor"), fmt2(rnum(e, "amount")), rstr(e, "currency"), rstr(e, "account"), rstr(e, "notes")})
		}
	case "audit":
		_ = cw.Write([]string{"Time", "User", "Action", "Entity", "Details"})
		for _, a := range db.AuditLogs {
			_ = cw.Write([]string{rstr(a, "time"), rstr(a, "user"), rstr(a, "action"), rstr(a, "entity"), rstr(a, "details")})
		}
	default:
		_ = cw.Write([]string{"Type", "Number", "Base Serial", "Date", "Party", "Currency", "Subtotal", "Tax", "Total", "Status", "POL", "POD", "Container", "Seal", "BL"})
		for _, d := range db.Documents {
			t := totals(d)
			_ = cw.Write([]string{titleForDocType(rstr(d, "type")), rstr(d, "number"), rstr(d, "baseSerial"), rstr(d, "date"), partyName(db, "customer", rstr(d, "customerId")), rstr(d, "currency"), fmt2(t.Subtotal), fmt2(t.Tax), fmt2(t.Total), rstr(d, "status"), rstr(d, "pol"), rstr(d, "pod"), rstr(d, "containerNo"), rstr(d, "sealNo"), rstr(d, "blNo")})
		}
	}
	cw.Flush()
	filename := "zenith-" + table + "-" + time.Now().Format("2006-01-02") + ".csv"
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)
	_, _ = w.Write(buf.Bytes())
}

func (s *Server) handleXLSXExport(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	db := s.db
	s.mu.Unlock()
	b, err := buildWorkbook(db)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, Record{"error": err.Error()})
		return
	}
	filename := "zenith-eclipse-erp-ultimate-" + time.Now().Format("2006-01-02") + ".xlsx"
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)
	_, _ = w.Write(b)
}

func (s *Server) handleDocumentPrint(w http.ResponseWriter, r *http.Request) {
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/document/"), "/")
	s.mu.Lock()
	db := s.db
	s.mu.Unlock()
	var doc Record
	found := false
	for _, d := range db.Documents {
		if rstr(d, "id") == id {
			doc = d
			found = true
			break
		}
	}
	if !found {
		http.NotFound(w, r)
		return
	}
	html, err := renderPrintHTML(db, doc)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(html))
}

func (s *Server) handleVerify(w http.ResponseWriter, r *http.Request) {
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/verify/"), "/")
	s.mu.Lock()
	db := s.db
	s.mu.Unlock()
	var doc Record
	for _, d := range db.Documents {
		if rstr(d, "id") == id {
			doc = d
			break
		}
	}
	if doc == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<h1>Document not found</h1>"))
		return
	}
	t := totals(doc)
	party := partyName(db, "customer", rstr(doc, "customerId"))
	body := fmt.Sprintf(`<html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Verify %s</title><style>body{font-family:Arial,sans-serif;background:#f4f7fb;padding:40px}.card{max-width:640px;background:white;margin:auto;border-radius:22px;padding:28px;box-shadow:0 20px 60px #0001}.ok{font-size:44px;color:#059669}dt{font-weight:800;color:#475569}dd{margin:0 0 10px}</style></head><body><div class="card"><div class="ok">✓ Verified</div><h1>%s</h1><p>This document exists in Zenith Eclipse ERP Pro.</p><dl><dt>Type</dt><dd>%s</dd><dt>Customer</dt><dd>%s</dd><dt>Date</dt><dd>%s</dd><dt>Status</dt><dd>%s</dd><dt>Total</dt><dd>%s %s</dd></dl></div></body></html>`, template.HTMLEscapeString(rstr(doc, "number")), template.HTMLEscapeString(rstr(doc, "number")), template.HTMLEscapeString(titleForDocType(rstr(doc, "type"))), template.HTMLEscapeString(party), template.HTMLEscapeString(rstr(doc, "date")), template.HTMLEscapeString(rstr(doc, "status")), template.HTMLEscapeString(rstr(doc, "currency")), fmt2(t.Total))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(body))
}

func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := s.currentUserObj(r); !ok {
			writeJSON(w, http.StatusUnauthorized, Record{"error": "not authenticated"})
			return
		}
		next(w, r)
	}
}

func (s *Server) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, ok := s.currentUserObj(r)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, Record{"error": "not authenticated"})
			return
		}
		if !strings.EqualFold(u.Role, "admin") {
			writeJSON(w, http.StatusForbidden, Record{"error": "admin only"})
			return
		}
		next(w, r)
	}
}

func (s *Server) currentUserObj(r *http.Request) (User, bool) {
	c, err := r.Cookie("zenith_session")
	if err != nil || c.Value == "" {
		return User{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[c.Value]
	if !ok || time.Now().After(sess.Expires) {
		if ok {
			delete(s.sessions, c.Value)
		}
		return User{}, false
	}
	for _, u := range s.db.Users {
		if u.Username == sess.Username && isActiveStatus(u.Status) {
			return u, true
		}
	}
	return User{}, false
}

func sanitizeUser(u User) Record {
	name := u.Name
	if name == "" {
		name = u.FullName
	}
	id := u.ID
	if id == "" {
		id = u.Username
	}
	return Record{"id": id, "name": name, "fullName": u.FullName, "username": u.Username, "email": u.Email, "phone": u.Phone, "role": u.Role, "status": u.Status, "createdAt": u.CreatedAt, "approvedBy": u.ApprovedBy, "approvedAt": u.ApprovedAt, "lastLoginAt": u.LastLoginAt}
}

func (s *Server) addAuditLocked(user, action, entity, entityID, details string) {
	if len(s.db.AuditLogs) > 2000 {
		s.db.AuditLogs = s.db.AuditLogs[len(s.db.AuditLogs)-1500:]
	}
	s.db.AuditLogs = append(s.db.AuditLogs, Record{"id": "aud-" + randomToken(6), "time": time.Now().Format(time.RFC3339), "at": time.Now().Format(time.RFC3339), "user": user, "action": action, "entity": entity, "entityId": entityID, "details": details})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func hashPassword(salt, password string) string {
	h := sha256.Sum256([]byte(salt + ":" + password))
	return hex.EncodeToString(h[:])
}

func randomToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(b)
}

func listenFirstAvailable(start, end int) (net.Listener, error) {
	var last error
	for p := start; p <= end; p++ {
		ln, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(p))
		if err == nil {
			return ln, nil
		}
		last = err
	}
	return nil, last
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

func strVal(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		if math.Trunc(x) == x {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		if x {
			return "true"
		}
		return "false"
	case nil:
		return ""
	default:
		return fmt.Sprint(v)
	}
}

func boolVal(v any, def bool) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		if x == "" {
			return def
		}
		b, err := strconv.ParseBool(x)
		if err == nil {
			return b
		}
		return def
	case float64:
		return x != 0
	default:
		return def
	}
}

func makeCode(name string) string {
	name = strings.ToUpper(name)
	var b strings.Builder
	for _, r := range name {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			if b.Len() >= 5 {
				break
			}
		}
	}
	if b.Len() == 0 {
		return "GEN"
	}
	return b.String()
}

func rstr(r Record, key string) string { return strVal(r[key]) }
func rnum(r Record, key string) float64 {
	switch x := r[key].(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case string:
		f, _ := strconv.ParseFloat(strings.ReplaceAll(x, ",", ""), 64)
		return f
	default:
		return 0
	}
}

func itemsOf(doc Record) []Record {
	raw, ok := doc["items"]
	if !ok || raw == nil {
		return nil
	}
	switch arr := raw.(type) {
	case []Record:
		return arr
	case []any:
		out := make([]Record, 0, len(arr))
		for _, v := range arr {
			if m, ok := v.(map[string]any); ok {
				out = append(out, Record(m))
			}
		}
		return out
	default:
		return nil
	}
}

type Totals struct {
	Subtotal  float64
	Product   float64
	Transport float64
	Service   float64
	Taxable   float64
	Tax       float64
	Total     float64
}

func totals(d Record) Totals {
	out := Totals{}
	for _, it := range itemsOf(d) {
		line := cleanMoney(rnum(it, "qty") * rnum(it, "unitPrice"))
		out.Subtotal += line
		switch normalizeLineKindGo(firstNonEmpty(rstr(it, "itemKind"), rstr(it, "category"), inferLineKindGo(it))) {
		case "transport":
			out.Transport += line
		case "service", "charge":
			out.Service += line
		default:
			out.Product += line
		}
	}
	out.Subtotal = cleanMoney(out.Subtotal)
	out.Product = cleanMoney(out.Product)
	out.Transport = cleanMoney(out.Transport)
	out.Service = cleanMoney(out.Service)
	out.Taxable = out.Subtotal - rnum(d, "discount") + rnum(d, "shipping")
	if out.Taxable < 0 {
		out.Taxable = 0
	}
	out.Tax = cleanMoney(out.Taxable * rnum(d, "taxRate") / 100)
	out.Total = cleanMoney(out.Taxable + out.Tax)
	return out
}

func normalizeLineKindGo(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	v = strings.ReplaceAll(v, "transportation", "transport")
	v = strings.ReplaceAll(v, "freight", "transport")
	v = strings.ReplaceAll(v, "goods", "product")
	switch v {
	case "product", "transport", "service", "charge", "discount":
		return v
	default:
		return "product"
	}
}

func inferLineKindGo(it Record) string {
	text := strings.ToLower(rstr(it, "sku") + " " + rstr(it, "name") + " " + rstr(it, "description") + " " + rstr(it, "unit") + " " + rstr(it, "warehouse"))
	if strings.Contains(text, "freight") || strings.Contains(text, "transport") || strings.Contains(text, "truck") || strings.Contains(text, "container") || strings.Contains(text, "logistic") || strings.Contains(text, "shipping") || strings.Contains(text, "vessel") || strings.Contains(text, "voyage") || strings.Contains(text, "delivery") || strings.Contains(text, "port") {
		return "transport"
	}
	if strings.Contains(text, "customs") || strings.Contains(text, "clearance") || strings.Contains(text, "documentation") || strings.Contains(text, "handling") || strings.Contains(text, "service") {
		return "service"
	}
	if strings.Contains(text, "discount") || strings.Contains(text, "rebate") {
		return "discount"
	}
	return "product"
}

func lineKindLabelGo(v string) string {
	switch normalizeLineKindGo(v) {
	case "transport":
		return "Transportation"
	case "service":
		return "Service"
	case "charge":
		return "Other Charge"
	case "discount":
		return "Discount"
	default:
		return "Product"
	}
}

func inferDealModeGo(items []Record) string {
	hasProduct, hasTransport, hasService := false, false, false
	for _, it := range items {
		switch normalizeLineKindGo(firstNonEmpty(rstr(it, "itemKind"), rstr(it, "category"), inferLineKindGo(it))) {
		case "product":
			hasProduct = true
		case "transport":
			hasTransport = true
		default:
			hasService = true
		}
	}
	if hasProduct && (hasTransport || hasService) {
		return "combined"
	}
	if hasTransport {
		return "transport"
	}
	if hasProduct {
		return "product"
	}
	return "service"
}

func dealModeLabelGo(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "product":
		return "Product Sale"
	case "transport", "transportation":
		return "Transportation Only"
	case "service":
		return "Service Only"
	default:
		return "Product + Transportation"
	}
}

func cleanMoney(f float64) float64 {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0
	}
	return math.Round(f*100) / 100
}

func fmt2(f float64) string { return strconv.FormatFloat(cleanMoney(f), 'f', 2, 64) }

func partyName(db DB, typ, id string) string {
	list := db.Customers
	if typ == "supplier" {
		list = db.Suppliers
	}
	for _, p := range list {
		if rstr(p, "id") == id {
			return rstr(p, "name")
		}
	}
	return ""
}

func partyByID(db DB, typ, id string) Record {
	list := db.Customers
	if typ == "supplier" {
		list = db.Suppliers
	}
	for _, p := range list {
		if rstr(p, "id") == id {
			return p
		}
	}
	return Record{}
}

func docNumber(db DB, id string) string {
	for _, d := range db.Documents {
		if rstr(d, "id") == id {
			return rstr(d, "number")
		}
	}
	return ""
}

func titleForDocType(t string) string {
	switch strings.ToLower(t) {
	case "quotation":
		return "Quotation"
	case "pi":
		return "Proforma Invoice"
	case "invoice", "commercial":
		return "Commercial Invoice"
	case "packing":
		return "Packing List"
	case "agreement":
		return "Agreement"
	case "purchase":
		return "Purchase Order"
	case "delivery":
		return "Delivery Note"
	case "receipt":
		return "Receipt Voucher"
	default:
		return strings.Title(strings.ReplaceAll(t, "-", " "))
	}
}

type Sheet struct {
	Name string
	Rows [][]any
}

func buildWorkbook(db DB) ([]byte, error) {
	var sheets []Sheet
	customers := Sheet{Name: "Customers", Rows: [][]any{{"Name", "Contact", "Email", "Phone", "City", "Country", "Currency", "Tax ID", "Notes"}}}
	for _, c := range db.Customers {
		customers.Rows = append(customers.Rows, []any{rstr(c, "name"), rstr(c, "contact"), rstr(c, "email"), rstr(c, "phone"), rstr(c, "city"), rstr(c, "country"), rstr(c, "currency"), rstr(c, "taxId"), rstr(c, "notes")})
	}
	sheets = append(sheets, customers)
	suppliers := Sheet{Name: "Suppliers", Rows: [][]any{{"Name", "Contact", "Email", "Phone", "City", "Country", "Currency", "Tax ID", "Notes"}}}
	for _, c := range db.Suppliers {
		suppliers.Rows = append(suppliers.Rows, []any{rstr(c, "name"), rstr(c, "contact"), rstr(c, "email"), rstr(c, "phone"), rstr(c, "city"), rstr(c, "country"), rstr(c, "currency"), rstr(c, "taxId"), rstr(c, "notes")})
	}
	sheets = append(sheets, suppliers)
	products := Sheet{Name: "Products", Rows: [][]any{{"SKU", "Name", "Description", "HS Code", "Unit", "Cost", "Sale Price", "Currency", "Stock Qty", "Warehouse", "Notes"}}}
	for _, p := range db.Products {
		products.Rows = append(products.Rows, []any{rstr(p, "sku"), rstr(p, "name"), rstr(p, "description"), rstr(p, "hsCode"), rstr(p, "unit"), rnum(p, "costPrice"), rnum(p, "salePrice"), rstr(p, "currency"), rnum(p, "stockQty"), rstr(p, "warehouse"), rstr(p, "notes")})
	}
	sheets = append(sheets, products)
	docs := Sheet{Name: "Documents", Rows: [][]any{{"Type", "Number", "Base Serial", "Date", "Customer", "Supplier", "Currency", "Subtotal", "Discount", "Shipping", "Tax", "Total", "Status", "POL", "POD", "Container", "Seal", "BL", "Verification"}}}
	sort.Slice(db.Documents, func(i, j int) bool { return rstr(db.Documents[i], "date") < rstr(db.Documents[j], "date") })
	for _, d := range db.Documents {
		t := totals(d)
		docs.Rows = append(docs.Rows, []any{titleForDocType(rstr(d, "type")), rstr(d, "number"), rstr(d, "baseSerial"), rstr(d, "date"), partyName(db, "customer", rstr(d, "customerId")), partyName(db, "supplier", rstr(d, "supplierId")), rstr(d, "currency"), t.Subtotal, rnum(d, "discount"), rnum(d, "shipping"), t.Tax, t.Total, rstr(d, "status"), rstr(d, "pol"), rstr(d, "pod"), rstr(d, "containerNo"), rstr(d, "sealNo"), rstr(d, "blNo"), rstr(d, "verificationCode")})
	}
	sheets = append(sheets, docs)
	payments := Sheet{Name: "Payments", Rows: [][]any{{"Type", "Date", "Party Type", "Party", "Document", "Amount", "Currency", "Exchange Rate", "Account", "Method", "Reference", "Notes"}}}
	for _, p := range db.Payments {
		payments.Rows = append(payments.Rows, []any{rstr(p, "type"), rstr(p, "date"), rstr(p, "partyType"), partyName(db, rstr(p, "partyType"), rstr(p, "partyId")), docNumber(db, rstr(p, "documentId")), rnum(p, "amount"), rstr(p, "currency"), rnum(p, "exchangeRate"), rstr(p, "account"), rstr(p, "method"), rstr(p, "reference"), rstr(p, "notes")})
	}
	sheets = append(sheets, payments)
	expenses := Sheet{Name: "Expenses", Rows: [][]any{{"Date", "Category", "Vendor", "Amount", "Currency", "Exchange Rate", "Account", "Notes"}}}
	for _, e := range db.Expenses {
		expenses.Rows = append(expenses.Rows, []any{rstr(e, "date"), rstr(e, "category"), rstr(e, "vendor"), rnum(e, "amount"), rstr(e, "currency"), rnum(e, "exchangeRate"), rstr(e, "account"), rstr(e, "notes")})
	}
	sheets = append(sheets, expenses)
	audit := Sheet{Name: "Audit", Rows: [][]any{{"Time", "User", "Action", "Entity", "Details"}}}
	for _, a := range db.AuditLogs {
		audit.Rows = append(audit.Rows, []any{rstr(a, "time"), rstr(a, "user"), rstr(a, "action"), rstr(a, "entity"), rstr(a, "details")})
	}
	sheets = append(sheets, audit)
	return writeXLSX(sheets)
}

func writeXLSX(sheets []Sheet) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name, content string) error {
		f, err := zw.Create(name)
		if err != nil {
			return err
		}
		_, err = f.Write([]byte(content))
		return err
	}
	contentTypes := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/>`
	for i := range sheets {
		contentTypes += fmt.Sprintf(`<Override PartName="/xl/worksheets/sheet%d.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>`, i+1)
	}
	contentTypes += `</Types>`
	if err := add("[Content_Types].xml", contentTypes); err != nil {
		return nil, err
	}
	if err := add("_rels/.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`); err != nil {
		return nil, err
	}
	if err := add("xl/styles.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><fonts count="1"><font><sz val="11"/><name val="Calibri"/></font></fonts><fills count="1"><fill><patternFill patternType="none"/></fill></fills><borders count="1"><border><left/><right/><top/><bottom/><diagonal/></border></borders><cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs><cellXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/></cellXfs></styleSheet>`); err != nil {
		return nil, err
	}
	workbook := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets>`
	rels := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`
	for i, sh := range sheets {
		workbook += fmt.Sprintf(`<sheet name="%s" sheetId="%d" r:id="rId%d"/>`, xmlEscape(sanitizeSheetName(sh.Name)), i+1, i+1)
		rels += fmt.Sprintf(`<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet%d.xml"/>`, i+1, i+1)
	}
	rels += fmt.Sprintf(`<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>`, len(sheets)+1)
	workbook += `</sheets></workbook>`
	rels += `</Relationships>`
	if err := add("xl/workbook.xml", workbook); err != nil {
		return nil, err
	}
	if err := add("xl/_rels/workbook.xml.rels", rels); err != nil {
		return nil, err
	}
	for i, sh := range sheets {
		if err := add(fmt.Sprintf("xl/worksheets/sheet%d.xml", i+1), sheetXML(sh)); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func sanitizeSheetName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		s = "Sheet"
	}
	repl := strings.NewReplacer("[", "", "]", "", "*", "", "?", "", "/", "-", "\\", "-", ":", "-")
	s = repl.Replace(s)
	if len([]rune(s)) > 31 {
		s = string([]rune(s)[:31])
	}
	return s
}

func sheetXML(sh Sheet) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
	for r, row := range sh.Rows {
		rowNum := r + 1
		b.WriteString(fmt.Sprintf(`<row r="%d">`, rowNum))
		for c, val := range row {
			ref := colName(c+1) + strconv.Itoa(rowNum)
			switch v := val.(type) {
			case int:
				b.WriteString(fmt.Sprintf(`<c r="%s"><v>%d</v></c>`, ref, v))
			case float64:
				b.WriteString(fmt.Sprintf(`<c r="%s"><v>%s</v></c>`, ref, strconv.FormatFloat(cleanMoney(v), 'f', -1, 64)))
			default:
				b.WriteString(fmt.Sprintf(`<c r="%s" t="inlineStr"><is><t>%s</t></is></c>`, ref, xmlEscape(fmt.Sprint(v))))
			}
		}
		b.WriteString(`</row>`)
	}
	b.WriteString(`</sheetData></worksheet>`)
	return b.String()
}

func colName(n int) string {
	name := ""
	for n > 0 {
		n--
		name = string(rune('A'+(n%26))) + name
		n /= 26
	}
	return name
}

func xmlEscape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

func renderPrintHTML(db DB, doc Record) (string, error) {
	type PrintLine struct {
		No                                                          int
		Kind, Description, HSCode, Unit                             string
		Qty, UnitPrice, LineTotal, NetWeight, GrossWeight, Packages float64
	}
	type PrintData struct {
		Company      Record
		Party        Record
		Doc          Record
		Title        string
		DocLabel     string
		Lines        []PrintLine
		Totals       Totals
		Logo         string
		Leaf         string
		Verification string
		Matrix       template.HTML
		DateNow      string
	}
	lines := make([]PrintLine, 0)
	for i, it := range itemsOf(doc) {
		kind := normalizeLineKindGo(firstNonEmpty(rstr(it, "itemKind"), rstr(it, "category"), inferLineKindGo(it)))
		lines = append(lines, PrintLine{No: i + 1, Kind: lineKindLabelGo(kind), Description: rstr(it, "description"), HSCode: rstr(it, "hsCode"), Unit: rstr(it, "unit"), Qty: rnum(it, "qty"), UnitPrice: rnum(it, "unitPrice"), LineTotal: cleanMoney(rnum(it, "qty") * rnum(it, "unitPrice")), NetWeight: rnum(it, "netWeight"), GrossWeight: rnum(it, "grossWeight"), Packages: rnum(it, "packages")})
	}
	party := partyByID(db, "customer", rstr(doc, "customerId"))
	if rstr(doc, "supplierId") != "" && len(party) == 0 {
		party = partyByID(db, "supplier", rstr(doc, "supplierId"))
	}
	logo := rstr(db.Company, "logoData")
	if logo == "" {
		logo = rstr(db.Company, "logoUrl")
	}
	if logo == "" {
		logo = "/assets/zenith-logo.jpeg"
	}
	leaf := rstr(db.Company, "leafUrl")
	if leaf == "" {
		leaf = "/assets/maple-leaf.png"
	}
	ver := rstr(doc, "verificationCode")
	if ver == "" {
		ver = verificationCode(rstr(doc, "number") + rstr(doc, "date") + rstr(doc, "baseSerial"))
	}
	data := PrintData{Company: db.Company, Party: party, Doc: doc, Title: titleForDocType(rstr(doc, "type")), DocLabel: titleForDocType(rstr(doc, "type")) + "#", Lines: lines, Totals: totals(doc), Logo: logo, Leaf: leaf, Verification: ver, Matrix: verificationMatrix(ver), DateNow: time.Now().Format("2006-01-02")}
	funcs := template.FuncMap{"money": fmt2, "upper": strings.ToUpper, "yes": func(s string) bool { return strings.TrimSpace(s) != "" }, "rstr": rstr, "rnum": rnum, "dealModeLabel": dealModeLabelGo}
	t, err := template.New("print").Funcs(funcs).Parse(printTemplate)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func verificationCode(seed string) string {
	h := sha256.Sum256([]byte(seed))
	return strings.ToUpper(hex.EncodeToString(h[:])[:12])
}

func verificationMatrix(code string) template.HTML {
	h := sha256.Sum256([]byte(code))
	bits := ""
	for _, b := range h[:] {
		bits += fmt.Sprintf("%08b", b)
	}
	var sb strings.Builder
	sb.WriteString(`<div class="v-matrix">`)
	idx := 0
	for i := 0; i < 81; i++ {
		black := bits[idx%len(bits)] == '1'
		idx++
		cls := "w"
		if black {
			cls = "b"
		}
		sb.WriteString(`<span class="` + cls + `"></span>`)
	}
	sb.WriteString(`</div>`)
	return template.HTML(sb.String())
}

const printTemplate = `<!doctype html>
<html><head><meta charset="utf-8"><title>{{rstr .Doc "number"}} - {{.Title}}</title>
<style>
:root{font-family:Inter,Segoe UI,Arial,sans-serif;color:#111827}body{margin:0;background:#edf6fb}.no-print{position:fixed;right:24px;bottom:24px;background:#0b3b75;color:white;border:0;border-radius:999px;padding:12px 18px;font-weight:900;cursor:pointer;box-shadow:0 16px 36px rgba(15,23,42,.22);z-index:5}.sheet{width:210mm;min-height:297mm;margin:18px auto;background:white;border:10px solid transparent;background-image:linear-gradient(#fff,#fff),linear-gradient(135deg,#67e8f9,#0f5ea8 78%,#082f65);background-origin:border-box;background-clip:padding-box,border-box;border-radius:18px;box-shadow:0 30px 80px rgba(15,23,42,.18);position:relative;display:flex;flex-direction:column;overflow:hidden}.sheet:before{content:"";position:absolute;inset:0;background:radial-gradient(circle at 88% 5%,rgba(14,165,233,.12),transparent 24%),linear-gradient(90deg,rgba(103,232,249,.08),transparent 18%,transparent 82%,rgba(15,94,168,.06));pointer-events:none}.inner{padding:18px 20px 13px;display:flex;flex-direction:column;min-height:273mm;position:relative}.head{display:grid;grid-template-columns:1fr 245px;gap:24px;border-bottom:1px solid #dbeafe;padding-bottom:9px}.brand{display:grid;grid-template-columns:64px 1fr;gap:12px;align-items:start}.brand img{width:64px;height:64px;border-radius:50%;object-fit:cover;border:1px solid #e0f2fe}.brand h1{font-family:Georgia,serif;font-size:23px;line-height:1;margin:0 0 2px;letter-spacing:.03em;color:#111827}.brand .slogan{font-size:7px;font-weight:900;letter-spacing:.09em;text-transform:uppercase;margin:0;color:#334155}.to{display:grid;grid-template-columns:auto 1fr;gap:8px;margin-top:7px}.to .label{font-family:Georgia,serif;font-size:28px;font-weight:800}.to p{margin:1px 0;font-size:12px;line-height:1.28}.meta{text-align:right;padding-top:10px}.meta h2{margin:0 0 6px;font-size:14px;color:#0f172a}.meta .num{font-weight:950}.meta p{font-size:11px;margin:4px 0;color:#334155}.title{text-align:center;margin:16px 0 9px}.title h2{margin:0;text-transform:uppercase;letter-spacing:.16em;font-size:20px}.chips{display:flex;gap:8px;justify-content:center;flex-wrap:wrap;margin-top:7px}.chip{border:1px solid #bae6fd;background:#f0f9ff;border-radius:999px;padding:4px 10px;font-size:10px;font-weight:900;color:#0f3c68}.info{display:grid;grid-template-columns:1fr 1fr;gap:12px;margin:8px 0}.box{border:1px solid #dbeafe;border-radius:14px;padding:10px 12px;background:linear-gradient(180deg,#fff,#f8fbff)}.box h3{font-size:10px;color:#64748b;text-transform:uppercase;letter-spacing:.09em;margin:0 0 6px}.box p{font-size:11px;line-height:1.35;margin:3px 0}.doc-table{width:100%;border-collapse:separate;border-spacing:0;margin-top:10px;border:1px solid #dbeafe;border-radius:12px;overflow:hidden}.doc-table th{background:#0f5ea8;color:#fff;text-align:left;font-size:9px;padding:7px 6px;text-transform:uppercase;letter-spacing:.04em}.doc-table td{border-bottom:1px solid #e7eef5;font-size:9.5px;padding:7px 6px;vertical-align:top}.doc-table tr:last-child td{border-bottom:0}.right{text-align:right}.type-pill{display:inline-block;border-radius:999px;background:#eff6ff;color:#0f3c68;border:1px solid #bfdbfe;padding:2px 7px;font-size:8.5px;font-weight:900}.totals{margin-left:auto;width:315px;margin-top:12px}.row{display:flex;justify-content:space-between;border-bottom:1px solid #e7eef5;padding:6px 0;font-size:11px}.row strong{color:#0f172a}.grand{font-size:15px;font-weight:950;border-bottom:3px solid #0f5ea8}.terms{margin-top:12px;font-size:11px;color:#334155;white-space:pre-wrap;line-height:1.45}.signature{display:flex;justify-content:space-between;gap:22px;margin-top:28px}.sig{width:230px;text-align:center;border-top:1px solid #111827;padding-top:7px;font-size:11px}.grow{flex:1}.verify{display:flex;align-items:center;gap:10px;font-size:9px;color:#475569}.v-matrix{display:grid;grid-template-columns:repeat(9,5px);grid-template-rows:repeat(9,5px);gap:1px;background:#dbeafe;padding:3px;width:max-content}.v-matrix span{width:5px;height:5px;display:block}.v-matrix .b{background:#0f172a}.v-matrix .w{background:#fff}.foot{display:grid;grid-template-columns:48px 1.15fr 1.55fr 1fr 1.35fr;gap:8px;align-items:center;border-top:1px solid #dbeafe;padding-top:9px;font-size:10.2px;color:#334155}.foot img{max-width:30px;max-height:30px}.foot .leaf{width:44px;max-width:44px;max-height:44px}.foot a{color:#0f5ea8}.stamp{font-weight:950;color:#0f5ea8}@media print{body{background:#fff}.sheet{box-shadow:none;margin:0;border-width:10px;border-radius:0}.no-print{display:none}@page{size:A4;margin:0}}
</style></head><body><button class="no-print" onclick="window.print()">Print / Save PDF</button><main class="sheet"><div class="inner"><section class="head"><div><div class="brand"><img src="{{.Logo}}"><div><h1>{{rstr .Company "name"}}</h1><p class="slogan">{{rstr .Company "slogan"}}</p></div></div><div class="to"><div class="label">To:</div><div><p><strong>{{rstr .Party "name"}}</strong></p><p><strong>{{rstr .Party "contact"}}</strong></p><p>{{rstr .Party "address"}}</p></div></div></div><div class="meta"><h2>{{.DocLabel}} <span class="num">{{rstr .Doc "number"}}</span></h2><p>Date: {{rstr .Doc "date"}}</p><p>Status: {{rstr .Doc "status"}}</p><p>Base Serial: {{rstr .Doc "baseSerial"}}</p></div></section><section class="title"><h2>{{.Title}}</h2><div class="chips"><span class="chip">{{dealModeLabel (rstr .Doc "dealMode")}}</span><span class="chip">Verification {{.Verification}}</span></div></section><section class="info"><div class="box"><h3>Buyer / Customer</h3><p><strong>{{rstr .Party "name"}}</strong></p><p>{{rstr .Party "contact"}}</p><p>{{rstr .Party "phone"}} {{rstr .Party "email"}}</p><p>{{rstr .Party "city"}} {{rstr .Party "country"}}</p>{{if yes (rstr .Party "taxId")}}<p>Tax ID: {{rstr .Party "taxId"}}</p>{{end}}</div><div class="box"><h3>Product & Transportation Details</h3><p>Currency: <strong>{{rstr .Doc "currency"}}</strong></p>{{if yes (rstr .Doc "incoterm")}}<p>Incoterm: {{rstr .Doc "incoterm"}}</p>{{end}}<p>Route: {{rstr .Doc "pol"}} → {{rstr .Doc "pod"}}</p>{{if yes (rstr .Doc "containerNo")}}<p>Container: {{rstr .Doc "containerNo"}}</p>{{end}}{{if yes (rstr .Doc "sealNo")}}<p>Seal: {{rstr .Doc "sealNo"}}</p>{{end}}{{if yes (rstr .Doc "blNo")}}<p>BL No: {{rstr .Doc "blNo"}}</p>{{end}}{{if yes (rstr .Doc "vessel")}}<p>Vessel/Voyage: {{rstr .Doc "vessel"}} {{rstr .Doc "voyage"}}</p>{{end}}</div></section><table class="doc-table"><thead><tr><th>#</th><th>Type</th><th>Description</th><th>HS Code</th><th>Unit</th><th class="right">Qty</th><th class="right">Unit Price</th><th class="right">Total</th><th class="right">Net Wt</th><th class="right">Gross Wt</th><th class="right">Packages</th></tr></thead><tbody>{{range .Lines}}<tr><td>{{.No}}</td><td><span class="type-pill">{{.Kind}}</span></td><td>{{.Description}}</td><td>{{.HSCode}}</td><td>{{.Unit}}</td><td class="right">{{money .Qty}}</td><td class="right">{{money .UnitPrice}}</td><td class="right">{{money .LineTotal}}</td><td class="right">{{money .NetWeight}}</td><td class="right">{{money .GrossWeight}}</td><td class="right">{{money .Packages}}</td></tr>{{end}}</tbody></table><section class="totals"><div class="row"><span>Products</span><strong>{{money .Totals.Product}} {{rstr .Doc "currency"}}</strong></div><div class="row"><span>Transportation</span><strong>{{money .Totals.Transport}} {{rstr .Doc "currency"}}</strong></div><div class="row"><span>Services/Charges</span><strong>{{money .Totals.Service}} {{rstr .Doc "currency"}}</strong></div><div class="row"><span>Subtotal</span><strong>{{money .Totals.Subtotal}} {{rstr .Doc "currency"}}</strong></div><div class="row"><span>Discount</span><strong>{{money (rnum .Doc "discount")}} {{rstr .Doc "currency"}}</strong></div><div class="row"><span>Tax ({{money (rnum .Doc "taxRate")}}%)</span><strong>{{money .Totals.Tax}} {{rstr .Doc "currency"}}</strong></div><div class="row grand"><span>Total</span><strong>{{money .Totals.Total}} {{rstr .Doc "currency"}}</strong></div></section>{{if yes (rstr .Doc "notes")}}<section class="terms"><strong>Notes</strong><br>{{rstr .Doc "notes"}}</section>{{end}}{{if yes (rstr .Doc "terms")}}<section class="terms"><strong>Terms & Conditions</strong><br>{{rstr .Doc "terms"}}</section>{{end}}{{if yes (rstr .Company "bankName")}}<section class="terms"><strong>Bank Details</strong><br>Bank: {{rstr .Company "bankName"}}<br>Account: {{rstr .Company "bankAccount"}}<br>IBAN: {{rstr .Company "bankIban"}}<br>SWIFT: {{rstr .Company "bankSwift"}}</section>{{end}}<div class="grow"></div><section class="signature"><div class="verify">{{.Matrix}}<div><strong>Verification</strong><br>{{.Verification}}<br>Base serial: {{rstr .Doc "baseSerial"}}</div></div><div class="sig"><span class="stamp">{{rstr .Company "stampText"}}</span></div></section><footer class="foot"><img class="leaf" src="{{.Leaf}}"><div>✉ {{rstr .Company "email"}}</div><div>📍 {{rstr .Company "address"}}</div><div>☎ {{rstr .Company "phone"}}</div><div><strong>Find More at</strong><br><a>{{rstr .Company "website"}}</a></div></footer></div></main></body></html>`
