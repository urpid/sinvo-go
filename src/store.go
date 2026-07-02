package sinvo

import (
	"crypto/rand"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

func (a *App) initDatabase() error {
	if _, err := a.db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return err
	}
	if _, err := a.db.Exec("PRAGMA busy_timeout = 5000"); err != nil {
		return err
	}

	statements := []string{
		`CREATE TABLE IF NOT EXISTS settings (
				key TEXT PRIMARY KEY,
				value TEXT NOT NULL
			)`,
		`CREATE TABLE IF NOT EXISTS templates (
				id TEXT PRIMARY KEY,
				name TEXT NOT NULL,
				html TEXT NOT NULL,
				is_default INTEGER NOT NULL DEFAULT 0,
				template_type TEXT NOT NULL DEFAULT 'local',
				created_at TEXT NOT NULL
			)`,
		`CREATE TABLE IF NOT EXISTS customers (
				id TEXT PRIMARY KEY,
				name TEXT NOT NULL,
			contact_name TEXT,
			email TEXT,
			phone TEXT,
			address TEXT,
			city TEXT,
			postal_code TEXT,
			country_code TEXT,
			tax_id TEXT,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS products (
				id TEXT PRIMARY KEY,
				name TEXT NOT NULL,
				description TEXT,
				unit_price NUMERIC NOT NULL DEFAULT 0,
				sku TEXT,
				unit TEXT DEFAULT 'Stk',
				category TEXT,
				tax_definition_id TEXT,
				is_active INTEGER NOT NULL DEFAULT 1,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,
		`CREATE TABLE IF NOT EXISTS tax_definitions (
					id TEXT PRIMARY KEY,
					code TEXT UNIQUE,
					name TEXT,
					percent NUMERIC NOT NULL,
					country_code TEXT,
					is_default INTEGER NOT NULL DEFAULT 0,
					created_at TEXT NOT NULL,
					updated_at TEXT NOT NULL
				)`,
		`CREATE TABLE IF NOT EXISTS product_categories (
				id TEXT PRIMARY KEY,
				code TEXT UNIQUE NOT NULL,
				name TEXT NOT NULL,
				sort_order INTEGER NOT NULL DEFAULT 0,
				is_builtin INTEGER NOT NULL DEFAULT 0,
				created_at TEXT NOT NULL
			)`,
		`CREATE TABLE IF NOT EXISTS product_units (
				id TEXT PRIMARY KEY,
				code TEXT UNIQUE NOT NULL,
				name TEXT NOT NULL,
				sort_order INTEGER NOT NULL DEFAULT 0,
				is_builtin INTEGER NOT NULL DEFAULT 0,
				created_at TEXT NOT NULL
			)`,
		`CREATE TABLE IF NOT EXISTS invoices (
				id TEXT PRIMARY KEY,
				invoice_number TEXT UNIQUE NOT NULL,
				customer_id TEXT NOT NULL REFERENCES customers(id),
				issue_date TEXT NOT NULL,
			due_date TEXT,
			currency TEXT NOT NULL DEFAULT 'EUR',
			status TEXT NOT NULL CHECK(status IN ('draft', 'sent', 'paid', 'voided')) DEFAULT 'draft',
				subtotal NUMERIC NOT NULL DEFAULT 0,
				discount_amount NUMERIC NOT NULL DEFAULT 0,
				discount_percentage NUMERIC NOT NULL DEFAULT 0,
				tax_mode TEXT NOT NULL DEFAULT 'invoice',
				tax_rate NUMERIC NOT NULL DEFAULT 0,
				tax_definition_id TEXT,
				tax_amount NUMERIC NOT NULL DEFAULT 0,
				total NUMERIC NOT NULL DEFAULT 0,
			prices_include_tax INTEGER NOT NULL DEFAULT 0,
			rounding_mode TEXT NOT NULL DEFAULT 'line',
			payment_terms TEXT,
			notes TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS invoice_items (
			id TEXT PRIMARY KEY,
			invoice_id TEXT NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
			product_id TEXT REFERENCES products(id),
			description TEXT NOT NULL,
			quantity NUMERIC NOT NULL,
				unit TEXT,
				unit_price NUMERIC NOT NULL,
				tax_rate NUMERIC NOT NULL DEFAULT 0,
				tax_definition_id TEXT,
				line_total NUMERIC NOT NULL,
				notes TEXT,
				sort_order INTEGER NOT NULL DEFAULT 0
			)`,
		`CREATE TABLE IF NOT EXISTS invoice_status_history (
			id TEXT PRIMARY KEY,
			invoice_id TEXT NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
			status TEXT NOT NULL,
			changed_at TEXT NOT NULL,
			payment_method TEXT,
			note TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_customers_created ON customers(created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_products_active ON products(is_active)`,
		`CREATE INDEX IF NOT EXISTS idx_tax_definitions_code ON tax_definitions(code)`,
		`CREATE INDEX IF NOT EXISTS idx_product_categories_code ON product_categories(code)`,
		`CREATE INDEX IF NOT EXISTS idx_product_units_code ON product_units(code)`,
		`CREATE INDEX IF NOT EXISTS idx_templates_default ON templates(is_default)`,
		`CREATE INDEX IF NOT EXISTS idx_invoices_number ON invoices(invoice_number)`,
		`CREATE INDEX IF NOT EXISTS idx_invoices_customer ON invoices(customer_id)`,
		`CREATE INDEX IF NOT EXISTS idx_invoices_status ON invoices(status)`,
		`CREATE INDEX IF NOT EXISTS idx_invoice_items_invoice ON invoice_items(invoice_id, sort_order)`,
		`CREATE INDEX IF NOT EXISTS idx_invoice_status_history_invoice ON invoice_status_history(invoice_id, changed_at)`,
	}
	for _, stmt := range statements {
		if _, err := a.db.Exec(stmt); err != nil {
			return err
		}
	}
	if err := a.ensureSchemaUpgrades(); err != nil {
		return err
	}

	defaults := map[string]string{
		"companyName":                  "Meine Firma",
		"companyAddress":               "",
		"companyCity":                  "",
		"companyPostalCode":            "",
		"companyEmail":                 "",
		"companyPhone":                 "",
		"companyTaxId":                 "",
		"companyCountryCode":           "DE",
		"currency":                     "EUR",
		"logo":                         "",
		"highlight":                    "#1d6f8f",
		"templateId":                   "minimalist-clean",
		"paymentTerms":                 "Zahlbar innerhalb von 14 Tagen",
		"paymentMethods":               "Überweisung",
		"bankAccount":                  "",
		"defaultNotes":                 "",
		"locale":                       "de",
		"dateFormat":                   "DD.MM.YYYY",
		"numberFormat":                 "comma",
		"postalCityFormat":             "postal-city",
		"invoicePrefix":                "RE",
		"invoiceIncludeYear":           "true",
		"invoiceNumberPadding":         "4",
		"invoiceNumberPattern":         "",
		"invoiceNumberingEnabled":      "true",
		"xmlProfileId":                 "ubl21",
		"embedXmlInPdf":                "false",
		"embedXmlInHtml":               "false",
		"allowProtectedInvoiceChanges": "false",
	}
	for key, value := range defaults {
		if _, err := a.db.Exec("INSERT OR IGNORE INTO settings (key, value) VALUES (?, ?)", key, value); err != nil {
			return err
		}
	}
	if _, err := a.db.Exec("UPDATE settings SET value = 'de' WHERE key = 'locale' AND lower(value) NOT IN ('de', 'en')"); err != nil {
		return err
	}
	if err := a.seedProductOptions(); err != nil {
		return err
	}
	if err := a.seedTemplates(); err != nil {
		return err
	}
	return nil
}

func (a *App) ensureSchemaUpgrades() error {
	for _, col := range []struct {
		table string
		name  string
		def   string
	}{
		{"products", "tax_definition_id", "TEXT"},
		{"invoices", "tax_mode", "TEXT NOT NULL DEFAULT 'invoice'"},
		{"invoices", "tax_definition_id", "TEXT"},
		{"invoice_items", "tax_rate", "NUMERIC NOT NULL DEFAULT 0"},
		{"invoice_items", "tax_definition_id", "TEXT"},
		{"tax_definitions", "is_default", "INTEGER NOT NULL DEFAULT 0"},
		{"templates", "template_type", "TEXT NOT NULL DEFAULT 'local'"},
	} {
		if err := a.addColumnIfMissing(col.table, col.name, col.def); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) addColumnIfMissing(table, column, definition string) error {
	rows, err := a.db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return err
		}
		if name == column {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = a.db.Exec("ALTER TABLE " + table + " ADD COLUMN " + column + " " + definition)
	return err
}

func (a *App) seedProductOptions() error {
	now := nowText()
	categories := []ProductOption{
		{ID: "service", Code: "service", Name: "Service", SortOrder: 1, IsBuiltin: true},
		{ID: "goods", Code: "goods", Name: "Goods", SortOrder: 2, IsBuiltin: true},
		{ID: "subscription", Code: "subscription", Name: "Subscription", SortOrder: 3, IsBuiltin: true},
		{ID: "other", Code: "other", Name: "Other", SortOrder: 4, IsBuiltin: true},
	}
	for _, item := range categories {
		if _, err := a.db.Exec(`INSERT OR IGNORE INTO product_categories
			(id, code, name, sort_order, is_builtin, created_at)
			VALUES (?, ?, ?, ?, ?, ?)`, item.ID, item.Code, item.Name, item.SortOrder, boolInt(item.IsBuiltin), now); err != nil {
			return err
		}
	}
	units := []ProductOption{
		{ID: "piece", Code: "piece", Name: "Piece", SortOrder: 1, IsBuiltin: true},
		{ID: "hour", Code: "hour", Name: "Hour", SortOrder: 2, IsBuiltin: true},
		{ID: "day", Code: "day", Name: "Day", SortOrder: 3, IsBuiltin: true},
		{ID: "kg", Code: "kg", Name: "Kilogram", SortOrder: 4, IsBuiltin: true},
		{ID: "m", Code: "m", Name: "Meter", SortOrder: 5, IsBuiltin: true},
		{ID: "lump_sum", Code: "lump_sum", Name: "Lump Sum", SortOrder: 6, IsBuiltin: true},
		{ID: "std", Code: "Std", Name: "Stunde", SortOrder: 7, IsBuiltin: true},
		{ID: "stk", Code: "Stk", Name: "Stueck", SortOrder: 8, IsBuiltin: true},
	}
	for _, item := range units {
		if _, err := a.db.Exec(`INSERT OR IGNORE INTO product_units
			(id, code, name, sort_order, is_builtin, created_at)
			VALUES (?, ?, ?, ?, ?, ?)`, item.ID, item.Code, item.Name, item.SortOrder, boolInt(item.IsBuiltin), now); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) settingsMap() (map[string]string, error) {
	rows, err := a.db.Query("SELECT key, value FROM settings ORDER BY key")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	settings := map[string]string{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		switch key {
		case "taxLabel", "defaultTaxRate", "defaultPricesIncludeTax", "defaultRoundingMode":
			continue
		}
		settings[key] = value
	}
	return settings, rows.Err()
}

func (a *App) updateSettings(data map[string]string) (map[string]string, error) {
	tx, err := a.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	selectedTemplateID := ""
	for key, raw := range data {
		value := strings.TrimSpace(raw)
		switch key {
		case "taxLabel", "defaultTaxRate", "defaultPricesIncludeTax", "defaultRoundingMode":
			continue
		case "templateId":
			selectedTemplateID = normalizeTemplateID(value)
			value = selectedTemplateID
		case "invoiceIncludeYear", "invoiceNumberingEnabled", "allowProtectedInvoiceChanges", "embedXmlInPdf", "embedXmlInHtml":
			value = boolText(value)
		case "locale":
			if strings.ToLower(value) == "en" {
				value = "en"
			} else {
				value = "de"
			}
		case "xmlProfileId":
			value = normalizeXMLProfileID(value)
		case "invoiceNumberPadding":
			n, err := strconv.Atoi(value)
			if err != nil || n < 2 || n > 8 {
				value = "4"
			}
		case "numberFormat":
			if strings.ToLower(value) != "period" {
				value = "comma"
			} else {
				value = "period"
			}
		case "dateFormat":
			if value != "YYYY-MM-DD" {
				value = "DD.MM.YYYY"
			}
		case "postalCityFormat":
			if value != "city-postal" && value != "postal-city" {
				value = "auto"
			}
		}
		if _, err := tx.Exec("INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value", key, value); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	if selectedTemplateID != "" {
		_ = a.setTemplateDefault(selectedTemplateID)
	}
	return a.settingsMap()
}

func (a *App) listCustomers() ([]Customer, error) {
	rows, err := a.db.Query(`SELECT id, name, contact_name, email, phone, address, city, postal_code, country_code, tax_id, created_at
		FROM customers ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []Customer{}
	for rows.Next() {
		c, err := scanCustomer(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, c)
	}
	return items, rows.Err()
}

func (a *App) getCustomer(id string) (Customer, error) {
	row := a.db.QueryRow(`SELECT id, name, contact_name, email, phone, address, city, postal_code, country_code, tax_id, created_at
		FROM customers WHERE id = ?`, id)
	return scanCustomer(row)
}

func (a *App) saveCustomer(input Customer) (Customer, error) {
	input.Name = trim(input.Name)
	if input.Name == "" {
		return Customer{}, errors.New("customer name is required")
	}
	if input.ID == "" {
		input.ID = newID()
		input.CreatedAt = nowText()
		_, err := a.db.Exec(`INSERT INTO customers
			(id, name, contact_name, email, phone, address, city, postal_code, country_code, tax_id, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			input.ID, input.Name, nullable(input.ContactName), nullable(input.Email), nullable(input.Phone),
			nullable(input.Address), nullable(input.City), nullable(input.PostalCode), nullable(input.CountryCode),
			nullable(input.TaxID), input.CreatedAt)
		if err != nil {
			return Customer{}, err
		}
		return a.getCustomer(input.ID)
	}
	if _, err := a.getCustomer(input.ID); err != nil {
		return Customer{}, err
	}
	_, err := a.db.Exec(`UPDATE customers SET
		name = ?, contact_name = ?, email = ?, phone = ?, address = ?, city = ?, postal_code = ?, country_code = ?, tax_id = ?
		WHERE id = ?`,
		input.Name, nullable(input.ContactName), nullable(input.Email), nullable(input.Phone), nullable(input.Address),
		nullable(input.City), nullable(input.PostalCode), nullable(input.CountryCode), nullable(input.TaxID), input.ID)
	if err != nil {
		return Customer{}, err
	}
	return a.getCustomer(input.ID)
}

func (a *App) deleteCustomer(id string) error {
	var count int
	if err := a.db.QueryRow("SELECT COUNT(*) FROM invoices WHERE customer_id = ?", id).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return errors.New("customer is used by invoices")
	}
	res, err := a.db.Exec("DELETE FROM customers WHERE id = ?", id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.New("customer not found")
	}
	return nil
}

func scanCustomer(scanner interface{ Scan(dest ...any) error }) (Customer, error) {
	var c Customer
	var contact, email, phone, address, city, postal, country, tax sql.NullString
	err := scanner.Scan(&c.ID, &c.Name, &contact, &email, &phone, &address, &city, &postal, &country, &tax, &c.CreatedAt)
	if err != nil {
		return Customer{}, err
	}
	c.ContactName = contact.String
	c.Email = email.String
	c.Phone = phone.String
	c.Address = address.String
	c.City = city.String
	c.PostalCode = postal.String
	c.CountryCode = country.String
	c.TaxID = tax.String
	return c, nil
}

func (a *App) listProducts(includeInactive bool) ([]Product, error) {
	query := `SELECT id, name, description, unit_price, sku, unit, category, tax_definition_id, is_active, created_at, updated_at FROM products`
	if !includeInactive {
		query += " WHERE is_active = 1"
	}
	query += " ORDER BY name ASC"

	rows, err := a.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	products := []Product{}
	for rows.Next() {
		p, err := scanProduct(rows)
		if err != nil {
			return nil, err
		}
		products = append(products, p)
	}
	return products, rows.Err()
}

func (a *App) getProduct(id string) (Product, error) {
	row := a.db.QueryRow(`SELECT id, name, description, unit_price, sku, unit, category, tax_definition_id, is_active, created_at, updated_at
		FROM products WHERE id = ?`, id)
	return scanProduct(row)
}

func (a *App) saveProduct(input Product) (Product, error) {
	input.Name = trim(input.Name)
	if input.Name == "" {
		return Product{}, errors.New("product name is required")
	}
	if input.Unit == "" {
		input.Unit = "Stk"
	}
	if input.UnitPrice < 0 {
		return Product{}, errors.New("unit price must not be negative")
	}
	now := nowText()
	if input.ID == "" {
		input.ID = newID()
		_, err := a.db.Exec(`INSERT INTO products
			(id, name, description, unit_price, sku, unit, category, tax_definition_id, is_active, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`,
			input.ID, input.Name, nullable(input.Description), input.UnitPrice, nullable(input.SKU),
			nullable(input.Unit), nullable(input.Category), nullable(input.TaxDefinitionID), now, now)
		if err != nil {
			return Product{}, err
		}
		return a.getProduct(input.ID)
	}
	if _, err := a.getProduct(input.ID); err != nil {
		return Product{}, err
	}
	_, err := a.db.Exec(`UPDATE products SET
		name = ?, description = ?, unit_price = ?, sku = ?, unit = ?, category = ?, tax_definition_id = ?, updated_at = ?
		WHERE id = ?`,
		input.Name, nullable(input.Description), input.UnitPrice, nullable(input.SKU), nullable(input.Unit),
		nullable(input.Category), nullable(input.TaxDefinitionID), now, input.ID)
	if err != nil {
		return Product{}, err
	}
	return a.getProduct(input.ID)
}

func (a *App) setProductActive(id string, active bool) (Product, error) {
	res, err := a.db.Exec("UPDATE products SET is_active = ?, updated_at = ? WHERE id = ?", boolInt(active), nowText(), id)
	if err != nil {
		return Product{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return Product{}, errors.New("product not found")
	}
	return a.getProduct(id)
}

func scanProduct(scanner interface{ Scan(dest ...any) error }) (Product, error) {
	var p Product
	var desc, sku, unit, category, taxDefinitionID sql.NullString
	var active int
	err := scanner.Scan(&p.ID, &p.Name, &desc, &p.UnitPrice, &sku, &unit, &category, &taxDefinitionID, &active, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return Product{}, err
	}
	p.Description = desc.String
	p.SKU = sku.String
	p.Unit = unit.String
	p.Category = category.String
	p.TaxDefinitionID = taxDefinitionID.String
	p.IsActive = active == 1
	return p, nil
}

func (a *App) listTaxDefinitions() ([]TaxDefinition, error) {
	rows, err := a.db.Query(`SELECT id, code, name, percent, country_code, is_default, created_at, updated_at
				FROM tax_definitions ORDER BY name ASC, percent ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []TaxDefinition{}
	for rows.Next() {
		item, err := scanTaxDefinition(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (a *App) getTaxDefinition(id string) (TaxDefinition, error) {
	row := a.db.QueryRow(`SELECT id, code, name, percent, country_code, is_default, created_at, updated_at
				FROM tax_definitions WHERE id = ?`, id)
	return scanTaxDefinition(row)
}

func (a *App) saveTaxDefinition(input TaxDefinition) (TaxDefinition, error) {
	if input.Percent < 0 {
		return TaxDefinition{}, errors.New("tax percent must not be negative")
	}
	if trim(input.Name) == "" && trim(input.Code) == "" {
		return TaxDefinition{}, errors.New("tax name or code is required")
	}
	now := nowText()
	if input.ID != "" {
		if _, err := a.getTaxDefinition(input.ID); err != nil {
			return TaxDefinition{}, err
		}
	}
	tx, err := a.db.Begin()
	if err != nil {
		return TaxDefinition{}, err
	}
	defer tx.Rollback()
	if input.ID == "" {
		input.ID = newID()
		_, err = tx.Exec(`INSERT INTO tax_definitions
				(id, code, name, percent, country_code, is_default, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			input.ID, nullable(input.Code), nullable(input.Name), input.Percent, nullable(input.CountryCode), boolInt(input.IsDefault), now, now)
		if err != nil {
			return TaxDefinition{}, err
		}
	} else {
		_, err = tx.Exec(`UPDATE tax_definitions SET
			code = ?, name = ?, percent = ?, country_code = ?, is_default = ?, updated_at = ?
			WHERE id = ?`,
			nullable(input.Code), nullable(input.Name), input.Percent, nullable(input.CountryCode), boolInt(input.IsDefault), now, input.ID)
	}
	if err != nil {
		return TaxDefinition{}, err
	}
	if input.IsDefault {
		if _, err := tx.Exec("UPDATE tax_definitions SET is_default = 0 WHERE id <> ?", input.ID); err != nil {
			return TaxDefinition{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return TaxDefinition{}, err
	}
	return a.getTaxDefinition(input.ID)
}

func (a *App) deleteTaxDefinition(id string) error {
	var count int
	if err := a.db.QueryRow(`SELECT
		(SELECT COUNT(*) FROM products WHERE tax_definition_id = ?) +
		(SELECT COUNT(*) FROM invoices WHERE tax_definition_id = ?) +
		(SELECT COUNT(*) FROM invoice_items WHERE tax_definition_id = ?)`, id, id, id).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return errors.New("tax definition is in use")
	}
	res, err := a.db.Exec("DELETE FROM tax_definitions WHERE id = ?", id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.New("tax definition not found")
	}
	return nil
}

func scanTaxDefinition(scanner interface{ Scan(dest ...any) error }) (TaxDefinition, error) {
	var item TaxDefinition
	var code, name, countryCode sql.NullString
	var isDefault int
	err := scanner.Scan(&item.ID, &code, &name, &item.Percent, &countryCode, &isDefault, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return TaxDefinition{}, err
	}
	item.Code = code.String
	item.Name = name.String
	item.CountryCode = countryCode.String
	item.IsDefault = isDefault == 1
	return item, nil
}

func (a *App) listProductOptions(kind string) ([]ProductOption, error) {
	table, err := productOptionTable(kind)
	if err != nil {
		return nil, err
	}
	rows, err := a.db.Query(`SELECT id, code, name, sort_order, is_builtin, created_at FROM ` + table + ` ORDER BY sort_order ASC, name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []ProductOption{}
	for rows.Next() {
		item, err := scanProductOption(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (a *App) saveProductOption(kind string, input ProductOption) (ProductOption, error) {
	table, err := productOptionTable(kind)
	if err != nil {
		return ProductOption{}, err
	}
	input.Code = trim(input.Code)
	input.Name = trim(input.Name)
	if input.Code == "" || input.Name == "" {
		return ProductOption{}, errors.New("code and name are required")
	}
	if input.ID == "" {
		input.ID = newID()
		_, err := a.db.Exec(`INSERT INTO `+table+`
			(id, code, name, sort_order, is_builtin, created_at)
			VALUES (?, ?, ?, ?, 0, ?)`, input.ID, input.Code, input.Name, input.SortOrder, nowText())
		if err != nil {
			return ProductOption{}, err
		}
		return a.getProductOption(kind, input.ID)
	}
	existing, err := a.getProductOption(kind, input.ID)
	if err != nil {
		return ProductOption{}, err
	}
	_, err = a.db.Exec(`UPDATE `+table+` SET code = ?, name = ?, sort_order = ? WHERE id = ?`,
		input.Code, input.Name, input.SortOrder, input.ID)
	if err != nil {
		return ProductOption{}, err
	}
	existing.Code = input.Code
	existing.Name = input.Name
	existing.SortOrder = input.SortOrder
	return existing, nil
}

func (a *App) getProductOption(kind, id string) (ProductOption, error) {
	table, err := productOptionTable(kind)
	if err != nil {
		return ProductOption{}, err
	}
	row := a.db.QueryRow(`SELECT id, code, name, sort_order, is_builtin, created_at FROM `+table+` WHERE id = ?`, id)
	return scanProductOption(row)
}

func (a *App) deleteProductOption(kind, id string) error {
	table, err := productOptionTable(kind)
	if err != nil {
		return err
	}
	item, err := a.getProductOption(kind, id)
	if err != nil {
		return err
	}
	if item.IsBuiltin {
		return errors.New("built-in option cannot be deleted")
	}
	column := "category"
	if kind == "units" {
		column = "unit"
	}
	var count int
	if err := a.db.QueryRow("SELECT COUNT(*) FROM products WHERE "+column+" = ?", item.Code).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return errors.New("option is used by products")
	}
	_, err = a.db.Exec("DELETE FROM "+table+" WHERE id = ?", id)
	return err
}

func productOptionTable(kind string) (string, error) {
	switch kind {
	case "categories":
		return "product_categories", nil
	case "units":
		return "product_units", nil
	default:
		return "", errors.New("unknown product option kind")
	}
}

func scanProductOption(scanner interface{ Scan(dest ...any) error }) (ProductOption, error) {
	var item ProductOption
	var builtin int
	err := scanner.Scan(&item.ID, &item.Code, &item.Name, &item.SortOrder, &builtin, &item.CreatedAt)
	if err != nil {
		return ProductOption{}, err
	}
	item.IsBuiltin = builtin == 1
	return item, nil
}

func (a *App) listInvoices() ([]Invoice, error) {
	rows, err := a.db.Query(`SELECT i.id, i.invoice_number, i.customer_id, c.name, i.issue_date, i.due_date, i.currency, i.status,
		i.subtotal, i.discount_amount, i.discount_percentage, i.tax_mode, i.tax_rate, i.tax_definition_id, i.tax_amount, i.total,
		i.prices_include_tax, i.rounding_mode, i.payment_terms, i.notes, i.created_at, i.updated_at
		FROM invoices i
		LEFT JOIN customers c ON c.id = i.customer_id
		ORDER BY i.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	invoices := []Invoice{}
	for rows.Next() {
		inv, err := scanInvoice(rows)
		if err != nil {
			return nil, err
		}
		invoices = append(invoices, applyDerivedStatus(inv))
	}
	return invoices, rows.Err()
}

func (a *App) getInvoice(id string) (InvoiceWithDetails, error) {
	row := a.db.QueryRow(`SELECT i.id, i.invoice_number, i.customer_id, c.name, i.issue_date, i.due_date, i.currency, i.status,
		i.subtotal, i.discount_amount, i.discount_percentage, i.tax_mode, i.tax_rate, i.tax_definition_id, i.tax_amount, i.total,
		i.prices_include_tax, i.rounding_mode, i.payment_terms, i.notes, i.created_at, i.updated_at
		FROM invoices i
		LEFT JOIN customers c ON c.id = i.customer_id
		WHERE i.id = ?`, id)
	inv, err := scanInvoice(row)
	if err != nil {
		return InvoiceWithDetails{}, err
	}
	inv = applyDerivedStatus(inv)

	customer, err := a.getCustomer(inv.CustomerID)
	if err != nil {
		return InvoiceWithDetails{}, err
	}
	items, err := a.invoiceItems(inv.ID)
	if err != nil {
		return InvoiceWithDetails{}, err
	}
	history, err := a.statusHistory(inv.ID)
	if err != nil {
		return InvoiceWithDetails{}, err
	}
	settings, _ := a.settingsMap()
	return InvoiceWithDetails{
		Invoice:       inv,
		Customer:      customer,
		Items:         items,
		StatusHistory: history,
		Settings:      settings,
	}, nil
}

func (a *App) invoiceItems(invoiceID string) ([]InvoiceItem, error) {
	rows, err := a.db.Query(`SELECT id, invoice_id, product_id, description, quantity, unit, unit_price, tax_rate, tax_definition_id, line_total, notes, sort_order
		FROM invoice_items WHERE invoice_id = ? ORDER BY sort_order`, invoiceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []InvoiceItem{}
	for rows.Next() {
		var it InvoiceItem
		var productID, unit, taxDefinitionID, notes sql.NullString
		if err := rows.Scan(&it.ID, &it.InvoiceID, &productID, &it.Description, &it.Quantity, &unit, &it.UnitPrice, &it.TaxRate, &taxDefinitionID, &it.LineTotal, &notes, &it.SortOrder); err != nil {
			return nil, err
		}
		it.ProductID = productID.String
		it.Unit = unit.String
		it.TaxDefinitionID = taxDefinitionID.String
		it.Notes = notes.String
		items = append(items, it)
	}
	return items, rows.Err()
}

func (a *App) statusHistory(invoiceID string) ([]StatusHistoryEntry, error) {
	rows, err := a.db.Query(`SELECT id, invoice_id, status, changed_at, payment_method, note
		FROM invoice_status_history WHERE invoice_id = ? ORDER BY changed_at ASC`, invoiceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []StatusHistoryEntry{}
	for rows.Next() {
		var h StatusHistoryEntry
		var payment, note sql.NullString
		if err := rows.Scan(&h.ID, &h.InvoiceID, &h.Status, &h.ChangedAt, &payment, &note); err != nil {
			return nil, err
		}
		h.PaymentMethod = payment.String
		h.Note = note.String
		items = append(items, h)
	}
	return items, rows.Err()
}

func scanInvoice(scanner interface{ Scan(dest ...any) error }) (Invoice, error) {
	var inv Invoice
	var customerName, due, taxMode, taxDefinitionID, terms, notes sql.NullString
	var includeTax int
	err := scanner.Scan(&inv.ID, &inv.InvoiceNumber, &inv.CustomerID, &customerName, &inv.IssueDate, &due, &inv.Currency, &inv.Status,
		&inv.Subtotal, &inv.DiscountAmount, &inv.DiscountPercentage, &taxMode, &inv.TaxRate, &taxDefinitionID, &inv.TaxAmount, &inv.Total,
		&includeTax, &inv.RoundingMode, &terms, &notes, &inv.CreatedAt, &inv.UpdatedAt)
	if err != nil {
		return Invoice{}, err
	}
	inv.CustomerName = customerName.String
	inv.DueDate = due.String
	inv.TaxMode = taxMode.String
	if inv.TaxMode == "" {
		inv.TaxMode = "invoice"
	}
	inv.TaxDefinitionID = taxDefinitionID.String
	inv.PricesIncludeTax = includeTax == 1
	inv.PaymentTerms = terms.String
	inv.Notes = notes.String
	return inv, nil
}

func (a *App) saveInvoice(input InvoiceInput, id string) (InvoiceWithDetails, error) {
	settings, err := a.settingsMap()
	if err != nil {
		return InvoiceWithDetails{}, err
	}
	if _, err := a.getCustomer(input.CustomerID); err != nil {
		return InvoiceWithDetails{}, errors.New("customer not found")
	}
	items, err := normalizeItems(input.Items)
	if err != nil {
		return InvoiceWithDetails{}, err
	}
	if len(items) == 0 {
		return InvoiceWithDetails{}, errors.New("at least one invoice item is required")
	}

	if input.IssueDate == "" {
		input.IssueDate = todayText()
	}
	if !validDate(input.IssueDate) {
		return InvoiceWithDetails{}, errors.New("issue date is invalid")
	}
	if input.DueDate != "" && !validDate(input.DueDate) {
		return InvoiceWithDetails{}, errors.New("due date is invalid")
	}
	if input.Currency == "" {
		input.Currency = setting(settings, "currency", "EUR")
	}
	if input.TaxMode != "line" {
		input.TaxMode = "invoice"
	}
	if input.TaxDefinitionID != "" && input.TaxRate == 0 {
		if percent, ok := a.taxPercent(input.TaxDefinitionID); ok {
			input.TaxRate = percent
		}
	}
	for i := range items {
		if items[i].TaxDefinitionID != "" && items[i].TaxRate == 0 {
			if percent, ok := a.taxPercent(items[i].TaxDefinitionID); ok {
				items[i].TaxRate = percent
			}
		}
	}
	if input.PaymentTerms == "" {
		input.PaymentTerms = setting(settings, "paymentTerms", "")
	}
	if input.RoundingMode != "total" {
		input.RoundingMode = "line"
	}
	totals := calculateTotals(items, input.DiscountPercentage, input.DiscountAmount, input.TaxRate, input.TaxMode, input.PricesIncludeTax, input.RoundingMode)

	if id == "" {
		return a.createInvoice(input, items, totals)
	}
	return a.updateInvoice(id, input, items, totals)
}

func (a *App) createInvoice(input InvoiceInput, items []InvoiceItem, totals calculatedTotals) (InvoiceWithDetails, error) {
	if trim(input.InvoiceNumber) == "" {
		input.InvoiceNumber = generateDraftInvoiceNumber()
	} else if a.invoiceNumberExists(input.InvoiceNumber, "") {
		return InvoiceWithDetails{}, errors.New("invoice number already exists")
	}

	tx, err := a.db.Begin()
	if err != nil {
		return InvoiceWithDetails{}, err
	}
	defer tx.Rollback()

	id := newID()
	now := nowText()
	_, err = tx.Exec(`INSERT INTO invoices
		(id, invoice_number, customer_id, issue_date, due_date, currency, status, subtotal, discount_amount, discount_percentage,
		 tax_mode, tax_rate, tax_definition_id, tax_amount, total, prices_include_tax, rounding_mode, payment_terms, notes, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, 'draft', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, trim(input.InvoiceNumber), input.CustomerID, input.IssueDate, nullable(input.DueDate), input.Currency,
		totals.Subtotal, totals.DiscountAmount, input.DiscountPercentage, input.TaxMode, input.TaxRate, nullable(input.TaxDefinitionID), totals.TaxAmount, totals.Total,
		boolInt(input.PricesIncludeTax), input.RoundingMode, nullable(input.PaymentTerms), nullable(input.Notes), now, now)
	if err != nil {
		return InvoiceWithDetails{}, err
	}
	if err := insertInvoiceItems(tx, id, items); err != nil {
		return InvoiceWithDetails{}, err
	}
	if err := recordStatusChange(tx, id, "draft", "", ""); err != nil {
		return InvoiceWithDetails{}, err
	}
	if err := tx.Commit(); err != nil {
		return InvoiceWithDetails{}, err
	}
	return a.getInvoice(id)
}

func (a *App) updateInvoice(id string, input InvoiceInput, items []InvoiceItem, totals calculatedTotals) (InvoiceWithDetails, error) {
	existing, err := a.getInvoice(id)
	if err != nil {
		return InvoiceWithDetails{}, err
	}
	if existing.Status != "draft" {
		settings, err := a.settingsMap()
		if err != nil {
			return InvoiceWithDetails{}, err
		}
		if setting(settings, "allowProtectedInvoiceChanges", "false") != "true" || existing.Status == "voided" {
			return InvoiceWithDetails{}, errors.New("only draft invoices can be changed")
		}
	}
	if trim(input.InvoiceNumber) == "" {
		input.InvoiceNumber = existing.InvoiceNumber
	}
	if input.InvoiceNumber != existing.InvoiceNumber && a.invoiceNumberExists(input.InvoiceNumber, id) {
		return InvoiceWithDetails{}, errors.New("invoice number already exists")
	}

	tx, err := a.db.Begin()
	if err != nil {
		return InvoiceWithDetails{}, err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`UPDATE invoices SET
		invoice_number = ?, customer_id = ?, issue_date = ?, due_date = ?, currency = ?,
		subtotal = ?, discount_amount = ?, discount_percentage = ?, tax_mode = ?, tax_rate = ?, tax_definition_id = ?, tax_amount = ?, total = ?,
		prices_include_tax = ?, rounding_mode = ?, payment_terms = ?, notes = ?, updated_at = ?
		WHERE id = ?`,
		trim(input.InvoiceNumber), input.CustomerID, input.IssueDate, nullable(input.DueDate), input.Currency,
		totals.Subtotal, totals.DiscountAmount, input.DiscountPercentage, input.TaxMode, input.TaxRate, nullable(input.TaxDefinitionID), totals.TaxAmount, totals.Total,
		boolInt(input.PricesIncludeTax), input.RoundingMode, nullable(input.PaymentTerms), nullable(input.Notes), nowText(), id)
	if err != nil {
		return InvoiceWithDetails{}, err
	}
	if _, err := tx.Exec("DELETE FROM invoice_items WHERE invoice_id = ?", id); err != nil {
		return InvoiceWithDetails{}, err
	}
	if err := insertInvoiceItems(tx, id, items); err != nil {
		return InvoiceWithDetails{}, err
	}
	if err := tx.Commit(); err != nil {
		return InvoiceWithDetails{}, err
	}
	return a.getInvoice(id)
}

func insertInvoiceItems(tx *sql.Tx, invoiceID string, items []InvoiceItem) error {
	for i, item := range items {
		_, err := tx.Exec(`INSERT INTO invoice_items
			(id, invoice_id, product_id, description, quantity, unit, unit_price, tax_rate, tax_definition_id, line_total, notes, sort_order)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			newID(), invoiceID, nullable(item.ProductID), item.Description, item.Quantity, nullable(item.Unit),
			item.UnitPrice, item.TaxRate, nullable(item.TaxDefinitionID), item.LineTotal, nullable(item.Notes), i)
		if err != nil {
			return err
		}
	}
	return nil
}

func (a *App) sendInvoice(id string) (InvoiceWithDetails, error) {
	inv, err := a.getInvoice(id)
	if err != nil {
		return InvoiceWithDetails{}, err
	}
	if inv.Status != "draft" {
		return InvoiceWithDetails{}, errors.New("only draft invoices can be sent")
	}
	if inv.Customer.Name == "" || len(inv.Items) == 0 || inv.Currency == "" || inv.IssueDate == "" {
		return InvoiceWithDetails{}, errors.New("invoice is missing required data")
	}
	if err := a.createBackup("before-send"); err != nil {
		return InvoiceWithDetails{}, err
	}

	number := inv.InvoiceNumber
	settings, err := a.settingsMap()
	if err != nil {
		return InvoiceWithDetails{}, err
	}
	if strings.HasPrefix(number, "DRAFT-") && setting(settings, "invoiceNumberingEnabled", "true") != "false" {
		number, err = a.nextInvoiceNumber()
		if err != nil {
			return InvoiceWithDetails{}, err
		}
	}

	tx, err := a.db.Begin()
	if err != nil {
		return InvoiceWithDetails{}, err
	}
	defer tx.Rollback()

	if _, err := tx.Exec("UPDATE invoices SET status = 'sent', invoice_number = ?, updated_at = ? WHERE id = ?", number, nowText(), id); err != nil {
		return InvoiceWithDetails{}, err
	}
	if err := recordStatusChange(tx, id, "sent", "", ""); err != nil {
		return InvoiceWithDetails{}, err
	}
	if err := tx.Commit(); err != nil {
		return InvoiceWithDetails{}, err
	}
	return a.getInvoice(id)
}

func (a *App) markInvoicePaid(id string, paymentMethod string) (InvoiceWithDetails, error) {
	inv, err := a.getInvoice(id)
	if err != nil {
		return InvoiceWithDetails{}, err
	}
	if inv.Status != "sent" && inv.Status != "overdue" {
		return InvoiceWithDetails{}, errors.New("only sent or overdue invoices can be marked paid")
	}
	return a.setInvoiceStatus(id, "paid", paymentMethod, "")
}

func (a *App) voidInvoice(id string) (InvoiceWithDetails, error) {
	inv, err := a.getInvoice(id)
	if err != nil {
		return InvoiceWithDetails{}, err
	}
	if inv.Status == "draft" {
		return InvoiceWithDetails{}, errors.New("draft invoices must be deleted instead")
	}
	if inv.Status == "paid" {
		return InvoiceWithDetails{}, errors.New("paid invoices cannot be voided")
	}
	if inv.Status == "voided" {
		return InvoiceWithDetails{}, errors.New("invoice is already voided")
	}
	return a.setInvoiceStatus(id, "voided", "", "")
}

func (a *App) setInvoiceStatus(id, status, paymentMethod, note string) (InvoiceWithDetails, error) {
	tx, err := a.db.Begin()
	if err != nil {
		return InvoiceWithDetails{}, err
	}
	defer tx.Rollback()

	if _, err := tx.Exec("UPDATE invoices SET status = ?, updated_at = ? WHERE id = ?", status, nowText(), id); err != nil {
		return InvoiceWithDetails{}, err
	}
	if err := recordStatusChange(tx, id, status, paymentMethod, note); err != nil {
		return InvoiceWithDetails{}, err
	}
	if err := tx.Commit(); err != nil {
		return InvoiceWithDetails{}, err
	}
	return a.getInvoice(id)
}

func (a *App) duplicateInvoice(id string) (InvoiceWithDetails, error) {
	original, err := a.getInvoice(id)
	if err != nil {
		return InvoiceWithDetails{}, err
	}
	input := InvoiceInput{
		CustomerID:         original.CustomerID,
		IssueDate:          todayText(),
		DueDate:            original.DueDate,
		Currency:           original.Currency,
		TaxMode:            original.TaxMode,
		DiscountAmount:     original.DiscountAmount,
		DiscountPercentage: original.DiscountPercentage,
		TaxRate:            original.TaxRate,
		TaxDefinitionID:    original.TaxDefinitionID,
		PricesIncludeTax:   original.PricesIncludeTax,
		RoundingMode:       original.RoundingMode,
		PaymentTerms:       original.PaymentTerms,
		Notes:              original.Notes,
		Items:              original.Items,
	}
	return a.saveInvoice(input, "")
}

func (a *App) deleteInvoice(id string) error {
	inv, err := a.getInvoice(id)
	if err != nil {
		return err
	}
	if inv.Status != "draft" && inv.Status != "voided" {
		settings, err := a.settingsMap()
		if err != nil {
			return err
		}
		if setting(settings, "allowProtectedInvoiceChanges", "false") != "true" {
			return errors.New("only draft or voided invoices can be deleted")
		}
	}
	_, err = a.db.Exec("DELETE FROM invoices WHERE id = ?", id)
	return err
}

func recordStatusChange(tx *sql.Tx, invoiceID, status, paymentMethod, note string) error {
	_, err := tx.Exec(`INSERT INTO invoice_status_history (id, invoice_id, status, changed_at, payment_method, note)
		VALUES (?, ?, ?, ?, ?, ?)`, newID(), invoiceID, status, nowText(), nullable(paymentMethod), nullable(note))
	return err
}

func (a *App) invoiceNumberExists(number, exceptID string) bool {
	var count int
	if exceptID == "" {
		_ = a.db.QueryRow("SELECT COUNT(*) FROM invoices WHERE invoice_number = ?", trim(number)).Scan(&count)
	} else {
		_ = a.db.QueryRow("SELECT COUNT(*) FROM invoices WHERE invoice_number = ? AND id <> ?", trim(number), exceptID).Scan(&count)
	}
	return count > 0
}

func normalizeItems(input []InvoiceItem) ([]InvoiceItem, error) {
	items := make([]InvoiceItem, 0, len(input))
	for i, item := range input {
		item.Description = trim(item.Description)
		item.ProductID = trim(item.ProductID)
		item.Unit = trim(item.Unit)
		item.Notes = trim(item.Notes)
		item.TaxDefinitionID = trim(item.TaxDefinitionID)
		if item.Description == "" {
			return nil, fmt.Errorf("item %d description is required", i+1)
		}
		if item.Quantity <= 0 {
			return nil, fmt.Errorf("item %d quantity must be greater than zero", i+1)
		}
		if item.UnitPrice < 0 {
			return nil, fmt.Errorf("item %d unit price must not be negative", i+1)
		}
		item.LineTotal = r2(item.Quantity * item.UnitPrice)
		item.SortOrder = i
		items = append(items, item)
	}
	return items, nil
}

func calculateTotals(items []InvoiceItem, discountPercentage, discountAmount, taxRate float64, taxMode string, pricesIncludeTax bool, roundingMode string) calculatedTotals {
	rate := math.Max(0, taxRate) / 100
	lineGrosses := make([]float64, len(items))
	subtotal := 0.0
	for i, item := range items {
		lineGrosses[i] = item.Quantity * item.UnitPrice
		subtotal += lineGrosses[i]
	}

	finalDiscount := discountAmount
	if discountPercentage > 0 {
		finalDiscount = subtotal * (discountPercentage / 100)
	}
	finalDiscount = math.Min(math.Max(finalDiscount, 0), subtotal)

	taxAmount := 0.0
	total := 0.0

	if roundingMode == "line" && subtotal > 0 {
		lineDiscounts := make([]float64, len(lineGrosses))
		distributed := 0.0
		for i, gross := range lineGrosses {
			if i == len(lineGrosses)-1 {
				lineDiscounts[i] = r2(finalDiscount - distributed)
			} else {
				discount := r2(finalDiscount * (gross / subtotal))
				lineDiscounts[i] = discount
				distributed += discount
			}
		}
		for i, gross := range lineGrosses {
			afterDiscount := math.Max(0, gross-lineDiscounts[i])
			lineRate := rate
			if taxMode == "line" {
				lineRate = math.Max(0, items[i].TaxRate) / 100
			}
			if pricesIncludeTax {
				net := afterDiscount
				if lineRate > 0 {
					net = afterDiscount / (1 + lineRate)
				}
				taxAmount += r2(afterDiscount - net)
				total += r2(afterDiscount)
			} else {
				tax := r2(afterDiscount * lineRate)
				taxAmount += tax
				total += r2(afterDiscount + tax)
			}
		}
	} else {
		afterDiscount := subtotal - finalDiscount
		if taxMode == "line" && subtotal > 0 {
			for i, gross := range lineGrosses {
				share := gross / subtotal
				lineAfterDiscount := math.Max(0, afterDiscount*share)
				lineRate := math.Max(0, items[i].TaxRate) / 100
				if pricesIncludeTax {
					net := lineAfterDiscount
					if lineRate > 0 {
						net = lineAfterDiscount / (1 + lineRate)
					}
					taxAmount += lineAfterDiscount - net
					total += lineAfterDiscount
				} else {
					taxAmount += lineAfterDiscount * lineRate
					total += lineAfterDiscount + lineAfterDiscount*lineRate
				}
			}
			taxAmount = r2(taxAmount)
			total = r2(total)
		} else if pricesIncludeTax {
			net := afterDiscount
			if rate > 0 {
				net = afterDiscount / (1 + rate)
			}
			taxAmount = r2(afterDiscount - net)
			total = r2(afterDiscount)
		} else {
			taxAmount = r2(afterDiscount * rate)
			total = r2(afterDiscount + taxAmount)
		}
	}

	return calculatedTotals{
		Subtotal:       r2(subtotal),
		DiscountAmount: r2(finalDiscount),
		TaxAmount:      r2(taxAmount),
		Total:          r2(total),
	}
}

func (a *App) taxPercent(id string) (float64, bool) {
	if strings.TrimSpace(id) == "" {
		return 0, false
	}
	var percent float64
	err := a.db.QueryRow("SELECT percent FROM tax_definitions WHERE id = ?", id).Scan(&percent)
	return percent, err == nil
}

func applyDerivedStatus(inv Invoice) Invoice {
	if inv.Status != "sent" || inv.DueDate == "" {
		return inv
	}
	due, err := time.Parse("2006-01-02", inv.DueDate)
	if err != nil {
		return inv
	}
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if due.Before(today) {
		inv.Status = "overdue"
	}
	return inv
}

func (a *App) nextInvoiceNumber() (string, error) {
	settings, err := a.settingsMap()
	if err != nil {
		return "", err
	}
	pattern := trim(settings["invoiceNumberPattern"])
	if pattern != "" {
		expanded := expandNumberPattern(pattern)
		if !strings.Contains(pattern, "{SEQ}") {
			return expanded, nil
		}
		prefix := strings.Split(expanded, "{SEQ}")[0]
		next := a.findMaxSequence(prefix) + 1
		return strings.ReplaceAll(expanded, "{SEQ}", fmt.Sprintf("%03d", next)), nil
	}

	prefix := setting(settings, "invoicePrefix", "RE")
	includeYear := setting(settings, "invoiceIncludeYear", "true") != "false"
	padding := intSetting(settings, "invoiceNumberPadding", 4)
	base := prefix + "-"
	if includeYear {
		base += strconv.Itoa(time.Now().Year()) + "-"
	}
	next := a.findMaxSequence(base) + 1
	return base + fmt.Sprintf("%0*d", padding, next), nil
}

func (a *App) findMaxSequence(prefix string) int {
	rows, err := a.db.Query("SELECT invoice_number FROM invoices WHERE invoice_number LIKE ?", prefix+"%")
	if err != nil {
		return 0
	}
	defer rows.Close()

	re := regexp.MustCompile("^" + regexp.QuoteMeta(prefix) + `(\d+).*?$`)
	maxSeq := 0
	for rows.Next() {
		var number string
		if err := rows.Scan(&number); err != nil {
			continue
		}
		match := re.FindStringSubmatch(number)
		if len(match) == 2 {
			if n, err := strconv.Atoi(match[1]); err == nil && n > maxSeq {
				maxSeq = n
			}
		}
	}
	return maxSeq
}

func generateDraftInvoiceNumber() string {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("DRAFT-%d", time.Now().UnixNano())
	}
	var out strings.Builder
	for _, v := range b {
		out.WriteByte(alphabet[int(v)%len(alphabet)])
	}
	return "DRAFT-" + out.String()
}

func expandNumberPattern(pattern string) string {
	now := time.Now()
	yyyy := strconv.Itoa(now.Year())
	yy := yyyy[2:]
	mm := fmt.Sprintf("%02d", int(now.Month()))
	dd := fmt.Sprintf("%02d", now.Day())
	out := strings.NewReplacer(
		"{YYYY}", yyyy,
		"{YY}", yy,
		"{MM}", mm,
		"{DD}", dd,
		"{DATE}", yyyy+mm+dd,
	).Replace(pattern)
	for strings.Contains(out, "{RAND4}") {
		out = strings.Replace(out, "{RAND4}", randomCode(4), 1)
	}
	return out
}

func randomCode(length int) string {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return strings.Repeat("X", length)
	}
	var out strings.Builder
	for _, v := range b {
		out.WriteByte(alphabet[int(v)%len(alphabet)])
	}
	return out.String()
}

func (a *App) dashboard() (Dashboard, error) {
	invoices, err := a.listInvoices()
	if err != nil {
		return Dashboard{}, err
	}
	customers, err := a.listCustomers()
	if err != nil {
		return Dashboard{}, err
	}
	products, err := a.listProducts(true)
	if err != nil {
		return Dashboard{}, err
	}
	settings, _ := a.settingsMap()

	statusCounts := map[string]int{"draft": 0, "sent": 0, "overdue": 0, "paid": 0, "voided": 0}
	money := map[string]float64{"billed": 0, "outstanding": 0, "paid": 0}
	for _, inv := range invoices {
		statusCounts[inv.Status]++
		if inv.Status != "voided" && inv.Status != "draft" {
			money["billed"] = r2(money["billed"] + inv.Total)
		}
		if inv.Status == "sent" || inv.Status == "overdue" {
			money["outstanding"] = r2(money["outstanding"] + inv.Total)
		}
		if inv.Status == "paid" {
			money["paid"] = r2(money["paid"] + inv.Total)
		}
	}
	recent := invoices
	if len(recent) > 8 {
		recent = recent[:8]
	}
	return Dashboard{
		Counts: map[string]int{
			"invoices":  len(invoices),
			"customers": len(customers),
			"products":  len(products),
		},
		StatusCounts: statusCounts,
		Money:        money,
		Recent:       recent,
		Currency:     setting(settings, "currency", "EUR"),
	}, nil
}

func (a *App) createBackup(reason string) error {
	filename := fmt.Sprintf("sinvo-go-%s-%s.sqlite", time.Now().Format("20060102-150405-000000000"), safeFileName(reason))
	path := filepath.Join(a.paths.BackupsDir, filename)
	_, err := a.db.Exec("VACUUM INTO " + sqlQuote(path))
	return err
}

func (a *App) exportInvoice(id, format string) (string, string, string, error) {
	inv, err := a.getInvoice(id)
	if err != nil {
		return "", "", "", err
	}
	base := safeFileName(inv.InvoiceNumber)
	if base == "" {
		base = inv.ID
	}
	switch format {
	case "json":
		body, err := json.MarshalIndent(inv, "", "  ")
		if err != nil {
			return "", "", "", err
		}
		path := filepath.Join(a.paths.ExportsDir, base+".json")
		if err := os.WriteFile(path, body, 0644); err != nil {
			return "", "", "", err
		}
		return path, "application/json", base + ".json", nil
	case "csv":
		path := filepath.Join(a.paths.ExportsDir, base+".csv")
		file, err := os.Create(path)
		if err != nil {
			return "", "", "", err
		}
		writer := csv.NewWriter(file)
		_ = writer.Write([]string{"invoice_number", "customer", "issue_date", "description", "quantity", "unit", "unit_price", "line_total"})
		for _, item := range inv.Items {
			_ = writer.Write([]string{
				inv.InvoiceNumber,
				inv.Customer.Name,
				inv.IssueDate,
				item.Description,
				strconv.FormatFloat(item.Quantity, 'f', 2, 64),
				item.Unit,
				strconv.FormatFloat(item.UnitPrice, 'f', 2, 64),
				strconv.FormatFloat(item.LineTotal, 'f', 2, 64),
			})
		}
		writer.Flush()
		closeErr := file.Close()
		if writer.Error() != nil {
			return "", "", "", writer.Error()
		}
		if closeErr != nil {
			return "", "", "", closeErr
		}
		return path, "text/csv", base + ".csv", nil
	case "html":
		html, err := a.renderInvoiceHTML(inv)
		if err != nil {
			return "", "", "", err
		}
		path := filepath.Join(a.paths.ExportsDir, base+".html")
		if err := os.WriteFile(path, []byte(html), 0644); err != nil {
			return "", "", "", err
		}
		return path, "text/html; charset=utf-8", base + ".html", nil
	case "xml":
		xml, profile := renderInvoiceXML(inv)
		path := filepath.Join(a.paths.ExportsDir, base+"-"+profile.ID+".xml")
		if err := os.WriteFile(path, []byte(xml), 0644); err != nil {
			return "", "", "", err
		}
		return path, "application/xml; charset=utf-8", base + "-" + profile.ID + ".xml", nil
	case "pdf":
		body, err := a.renderInvoicePDF(inv)
		if err != nil {
			return "", "", "", err
		}
		path := filepath.Join(a.paths.ExportsDir, base+".pdf")
		if err := os.WriteFile(path, body, 0644); err != nil {
			return "", "", "", err
		}
		return path, "application/pdf", base + ".pdf", nil
	default:
		return "", "", "", errors.New("unsupported export format")
	}
}

func renderInvoiceSimpleHTML(inv InvoiceWithDetails) string {
	var rows strings.Builder
	for _, item := range inv.Items {
		rows.WriteString(fmt.Sprintf("<tr><td>%s</td><td class='num'>%.2f</td><td>%s</td><td class='num'>%.2f</td><td class='num'>%.2f</td></tr>",
			htmlEscape(item.Description), item.Quantity, htmlEscape(item.Unit), item.UnitPrice, item.LineTotal))
	}
	settings := inv.Settings
	return fmt.Sprintf(`<!doctype html>
<html lang="de">
<head>
<meta charset="utf-8">
<title>Rechnung %s</title>
<style>
body{font-family:Arial,sans-serif;margin:40px;color:#1f2933}
h1{font-size:28px;margin:0 0 24px}
.top{display:flex;justify-content:space-between;gap:32px;margin-bottom:36px}
.box{white-space:pre-line}
table{border-collapse:collapse;width:100%%;margin-top:24px}
th,td{border-bottom:1px solid #d7dde5;padding:8px;text-align:left}
th{background:#f2f5f8}
.num{text-align:right}
.totals{margin-left:auto;width:320px}
</style>
</head>
<body>
<div class="top">
<div><h1>Rechnung %s</h1><div class="box">%s<br>%s %s</div></div>
<div class="box"><strong>%s</strong><br>%s<br>%s %s<br>%s</div>
</div>
<p><strong>Datum:</strong> %s<br><strong>Faellig:</strong> %s<br><strong>Status:</strong> %s</p>
<table><thead><tr><th>Beschreibung</th><th class="num">Menge</th><th>Einheit</th><th class="num">Preis</th><th class="num">Summe</th></tr></thead><tbody>%s</tbody></table>
<table class="totals">
<tr><td>Zwischensumme</td><td class="num">%.2f %s</td></tr>
<tr><td>Rabatt</td><td class="num">%.2f %s</td></tr>
<tr><td>Steuer %.2f%%</td><td class="num">%.2f %s</td></tr>
<tr><th>Gesamt</th><th class="num">%.2f %s</th></tr>
</table>
<p>%s</p>
</body>
</html>`,
		htmlEscape(inv.InvoiceNumber),
		htmlEscape(inv.InvoiceNumber),
		htmlEscape(setting(settings, "companyName", "")),
		htmlEscape(setting(settings, "companyPostalCode", "")),
		htmlEscape(setting(settings, "companyCity", "")),
		htmlEscape(inv.Customer.Name),
		htmlEscape(inv.Customer.Address),
		htmlEscape(inv.Customer.PostalCode),
		htmlEscape(inv.Customer.City),
		htmlEscape(inv.Customer.Email),
		htmlEscape(inv.IssueDate),
		htmlEscape(inv.DueDate),
		htmlEscape(inv.Status),
		rows.String(),
		inv.Subtotal, htmlEscape(inv.Currency),
		inv.DiscountAmount, htmlEscape(inv.Currency),
		inv.TaxRate, inv.TaxAmount, htmlEscape(inv.Currency),
		inv.Total, htmlEscape(inv.Currency),
		htmlEscape(inv.PaymentTerms),
	)
}

func r2(n float64) float64 {
	return math.Round(n*100) / 100
}

func validDate(s string) bool {
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

func boolText(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "y", "on":
		return "true"
	default:
		return "false"
	}
}

func setting(settings map[string]string, key, fallback string) string {
	if value := strings.TrimSpace(settings[key]); value != "" {
		return value
	}
	return fallback
}

func floatSetting(settings map[string]string, key string, fallback float64) float64 {
	value, err := strconv.ParseFloat(strings.ReplaceAll(settings[key], ",", "."), 64)
	if err != nil {
		return fallback
	}
	return value
}

func intSetting(settings map[string]string, key string, fallback int) int {
	value, err := strconv.Atoi(settings[key])
	if err != nil {
		return fallback
	}
	return value
}

func sqlQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func safeFileName(s string) string {
	re := regexp.MustCompile(`[^A-Za-z0-9._-]+`)
	return strings.Trim(re.ReplaceAllString(s, "_"), "_")
}

func htmlEscape(s string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&#34;", "'", "&#39;")
	return replacer.Replace(s)
}

func sortSettingsKeys(settings map[string]string) []string {
	keys := make([]string, 0, len(settings))
	for key := range settings {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
