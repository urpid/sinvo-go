package sinvo

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

func (a *App) handleAPI(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api")
	parts := splitPath(path)

	switch {
	case path == "/instance" && r.Method == http.MethodGet:
		writeJSON(w, map[string]string{
			"app": AppID,
			"url": "http://" + r.Host,
		}, http.StatusOK)
	case path == "/shutdown" && r.Method == http.MethodPost:
		writeJSON(w, map[string]bool{"ok": true}, http.StatusOK)
		if a.shutdown != nil {
			go a.shutdown()
		}
	case path == "/dashboard" && r.Method == http.MethodGet:
		dashboard, err := a.dashboard()
		writeResult(w, dashboard, err)
	case path == "/settings" && r.Method == http.MethodGet:
		settings, err := a.settingsMap()
		writeResult(w, settings, err)
	case path == "/settings" && r.Method == http.MethodPut:
		var data map[string]string
		if err := readJSON(r, &data); err != nil {
			writeError(w, err, http.StatusBadRequest)
			return
		}
		settings, err := a.updateSettings(data)
		writeResult(w, settings, err)
	case path == "/settings/logo-upload" && r.Method == http.MethodPost:
		logo, err := a.uploadLogo(w, r)
		writeResult(w, map[string]string{"logo": logo}, err)
	case path == "/backup" && r.Method == http.MethodPost:
		writeResult(w, map[string]bool{"ok": true}, a.createBackup("manual"))
	case len(parts) > 0 && parts[0] == "logos":
		a.serveLogo(w, r, parts)
	case len(parts) > 0 && parts[0] == "customers":
		a.handleCustomers(w, r, parts)
	case len(parts) > 0 && parts[0] == "products":
		a.handleProducts(w, r, parts)
	case len(parts) > 0 && parts[0] == "tax-definitions":
		a.handleTaxDefinitions(w, r, parts)
	case len(parts) > 0 && parts[0] == "product-categories":
		a.handleProductOptions(w, r, parts, "categories")
	case len(parts) > 0 && parts[0] == "product-units":
		a.handleProductOptions(w, r, parts, "units")
	case len(parts) > 0 && parts[0] == "templates":
		a.handleTemplates(w, r, parts)
	case len(parts) > 0 && parts[0] == "invoices":
		a.handleInvoices(w, r, parts)
	default:
		writeError(w, errors.New("not found"), http.StatusNotFound)
	}
}

func (a *App) handleTaxDefinitions(w http.ResponseWriter, r *http.Request, parts []string) {
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			items, err := a.listTaxDefinitions()
			writeResult(w, items, err)
		case http.MethodPost:
			var input TaxDefinition
			if err := readJSON(r, &input); err != nil {
				writeError(w, err, http.StatusBadRequest)
				return
			}
			item, err := a.saveTaxDefinition(input)
			writeResult(w, item, err)
		default:
			writeError(w, errors.New("method not allowed"), http.StatusMethodNotAllowed)
		}
		return
	}

	id := parts[1]
	switch r.Method {
	case http.MethodGet:
		item, err := a.getTaxDefinition(id)
		writeResult(w, item, err)
	case http.MethodPut:
		var input TaxDefinition
		if err := readJSON(r, &input); err != nil {
			writeError(w, err, http.StatusBadRequest)
			return
		}
		input.ID = id
		item, err := a.saveTaxDefinition(input)
		writeResult(w, item, err)
	case http.MethodDelete:
		writeResult(w, map[string]bool{"ok": true}, a.deleteTaxDefinition(id))
	default:
		writeError(w, errors.New("method not allowed"), http.StatusMethodNotAllowed)
	}
}

func (a *App) handleProductOptions(w http.ResponseWriter, r *http.Request, parts []string, kind string) {
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			items, err := a.listProductOptions(kind)
			writeResult(w, items, err)
		case http.MethodPost:
			var input ProductOption
			if err := readJSON(r, &input); err != nil {
				writeError(w, err, http.StatusBadRequest)
				return
			}
			item, err := a.saveProductOption(kind, input)
			writeResult(w, item, err)
		default:
			writeError(w, errors.New("method not allowed"), http.StatusMethodNotAllowed)
		}
		return
	}

	id := parts[1]
	switch r.Method {
	case http.MethodPut:
		var input ProductOption
		if err := readJSON(r, &input); err != nil {
			writeError(w, err, http.StatusBadRequest)
			return
		}
		input.ID = id
		item, err := a.saveProductOption(kind, input)
		writeResult(w, item, err)
	case http.MethodDelete:
		writeResult(w, map[string]bool{"ok": true}, a.deleteProductOption(kind, id))
	default:
		writeError(w, errors.New("method not allowed"), http.StatusMethodNotAllowed)
	}
}

func (a *App) handleCustomers(w http.ResponseWriter, r *http.Request, parts []string) {
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			customers, err := a.listCustomers()
			writeResult(w, customers, err)
		case http.MethodPost:
			var input Customer
			if err := readJSON(r, &input); err != nil {
				writeError(w, err, http.StatusBadRequest)
				return
			}
			customer, err := a.saveCustomer(input)
			writeResult(w, customer, err)
		default:
			writeError(w, errors.New("method not allowed"), http.StatusMethodNotAllowed)
		}
		return
	}

	id := parts[1]
	switch r.Method {
	case http.MethodGet:
		customer, err := a.getCustomer(id)
		writeResult(w, customer, err)
	case http.MethodPut:
		var input Customer
		if err := readJSON(r, &input); err != nil {
			writeError(w, err, http.StatusBadRequest)
			return
		}
		input.ID = id
		customer, err := a.saveCustomer(input)
		writeResult(w, customer, err)
	case http.MethodDelete:
		writeResult(w, map[string]bool{"ok": true}, a.deleteCustomer(id))
	default:
		writeError(w, errors.New("method not allowed"), http.StatusMethodNotAllowed)
	}
}

func (a *App) handleProducts(w http.ResponseWriter, r *http.Request, parts []string) {
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			includeInactive := r.URL.Query().Get("includeInactive") == "1"
			products, err := a.listProducts(includeInactive)
			writeResult(w, products, err)
		case http.MethodPost:
			var input Product
			if err := readJSON(r, &input); err != nil {
				writeError(w, err, http.StatusBadRequest)
				return
			}
			product, err := a.saveProduct(input)
			writeResult(w, product, err)
		default:
			writeError(w, errors.New("method not allowed"), http.StatusMethodNotAllowed)
		}
		return
	}

	id := parts[1]
	if len(parts) == 3 && parts[2] == "activate" && r.Method == http.MethodPost {
		product, err := a.setProductActive(id, true)
		writeResult(w, product, err)
		return
	}

	switch r.Method {
	case http.MethodGet:
		product, err := a.getProduct(id)
		writeResult(w, product, err)
	case http.MethodPut:
		var input Product
		if err := readJSON(r, &input); err != nil {
			writeError(w, err, http.StatusBadRequest)
			return
		}
		input.ID = id
		product, err := a.saveProduct(input)
		writeResult(w, product, err)
	case http.MethodDelete:
		product, err := a.setProductActive(id, false)
		writeResult(w, product, err)
	default:
		writeError(w, errors.New("method not allowed"), http.StatusMethodNotAllowed)
	}
}

func (a *App) handleInvoices(w http.ResponseWriter, r *http.Request, parts []string) {
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			invoices, err := a.listInvoices()
			writeResult(w, invoices, err)
		case http.MethodPost:
			var input InvoiceInput
			if err := readJSON(r, &input); err != nil {
				writeError(w, err, http.StatusBadRequest)
				return
			}
			invoice, err := a.saveInvoice(input, "")
			writeResult(w, invoice, err)
		default:
			writeError(w, errors.New("method not allowed"), http.StatusMethodNotAllowed)
		}
		return
	}

	id := parts[1]
	if len(parts) >= 3 {
		switch parts[2] {
		case "send":
			if r.Method != http.MethodPost {
				writeError(w, errors.New("method not allowed"), http.StatusMethodNotAllowed)
				return
			}
			invoice, err := a.sendInvoice(id)
			writeResult(w, invoice, err)
			return
		case "paid":
			if r.Method != http.MethodPost {
				writeError(w, errors.New("method not allowed"), http.StatusMethodNotAllowed)
				return
			}
			var data struct {
				PaymentMethod string `json:"paymentMethod"`
			}
			_ = readJSON(r, &data)
			invoice, err := a.markInvoicePaid(id, data.PaymentMethod)
			writeResult(w, invoice, err)
			return
		case "void":
			if r.Method != http.MethodPost {
				writeError(w, errors.New("method not allowed"), http.StatusMethodNotAllowed)
				return
			}
			invoice, err := a.voidInvoice(id)
			writeResult(w, invoice, err)
			return
		case "duplicate":
			if r.Method != http.MethodPost {
				writeError(w, errors.New("method not allowed"), http.StatusMethodNotAllowed)
				return
			}
			invoice, err := a.duplicateInvoice(id)
			writeResult(w, invoice, err)
			return
		case "export":
			if r.Method != http.MethodGet {
				writeError(w, errors.New("method not allowed"), http.StatusMethodNotAllowed)
				return
			}
			format := r.URL.Query().Get("format")
			if format == "" {
				format = "html"
			}
			path, contentType, filename, err := a.exportInvoice(id, format)
			if err != nil {
				writeError(w, err, http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", contentType)
			if format != "html" {
				w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
			}
			http.ServeFile(w, r, path)
			return
		}
	}

	switch r.Method {
	case http.MethodGet:
		invoice, err := a.getInvoice(id)
		writeResult(w, invoice, err)
	case http.MethodPut:
		var input InvoiceInput
		if err := readJSON(r, &input); err != nil {
			writeError(w, err, http.StatusBadRequest)
			return
		}
		invoice, err := a.saveInvoice(input, id)
		writeResult(w, invoice, err)
	case http.MethodDelete:
		writeResult(w, map[string]bool{"ok": true}, a.deleteInvoice(id))
	default:
		writeError(w, errors.New("method not allowed"), http.StatusMethodNotAllowed)
	}
}

func readJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(dst)
}

func writeResult(w http.ResponseWriter, value any, err error) {
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, sql.ErrNoRows) {
			status = http.StatusNotFound
		}
		writeError(w, err, status)
		return
	}
	writeJSON(w, value, http.StatusOK)
}

func writeJSON(w http.ResponseWriter, value any, status int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, err error, status int) {
	writeJSON(w, map[string]string{"error": err.Error()}, status)
}

func splitPath(path string) []string {
	path = strings.Trim(path, "/")
	if path == "" {
		return nil
	}
	return strings.Split(path, "/")
}
