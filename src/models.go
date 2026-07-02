package sinvo

import "database/sql"

type Paths struct {
	BaseDir    string
	DataDir    string
	DBPath     string
	BackupsDir string
	ExportsDir string
}

type App struct {
	db       *sql.DB
	paths    Paths
	shutdown func()
}

type Customer struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	ContactName string `json:"contactName,omitempty"`
	Email       string `json:"email,omitempty"`
	Phone       string `json:"phone,omitempty"`
	Address     string `json:"address,omitempty"`
	City        string `json:"city,omitempty"`
	PostalCode  string `json:"postalCode,omitempty"`
	CountryCode string `json:"countryCode,omitempty"`
	TaxID       string `json:"taxId,omitempty"`
	CreatedAt   string `json:"createdAt"`
}

type Product struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Description     string  `json:"description,omitempty"`
	UnitPrice       float64 `json:"unitPrice"`
	SKU             string  `json:"sku,omitempty"`
	Unit            string  `json:"unit,omitempty"`
	Category        string  `json:"category,omitempty"`
	TaxDefinitionID string  `json:"taxDefinitionId,omitempty"`
	IsActive        bool    `json:"isActive"`
	CreatedAt       string  `json:"createdAt"`
	UpdatedAt       string  `json:"updatedAt"`
}

type Invoice struct {
	ID                 string  `json:"id"`
	InvoiceNumber      string  `json:"invoiceNumber"`
	CustomerID         string  `json:"customerId"`
	CustomerName       string  `json:"customerName,omitempty"`
	IssueDate          string  `json:"issueDate"`
	DueDate            string  `json:"dueDate,omitempty"`
	Currency           string  `json:"currency"`
	Status             string  `json:"status"`
	Subtotal           float64 `json:"subtotal"`
	DiscountAmount     float64 `json:"discountAmount"`
	DiscountPercentage float64 `json:"discountPercentage"`
	TaxMode            string  `json:"taxMode,omitempty"`
	TaxRate            float64 `json:"taxRate"`
	TaxDefinitionID    string  `json:"taxDefinitionId,omitempty"`
	TaxAmount          float64 `json:"taxAmount"`
	Total              float64 `json:"total"`
	PricesIncludeTax   bool    `json:"pricesIncludeTax"`
	RoundingMode       string  `json:"roundingMode"`
	PaymentTerms       string  `json:"paymentTerms,omitempty"`
	Notes              string  `json:"notes,omitempty"`
	CreatedAt          string  `json:"createdAt"`
	UpdatedAt          string  `json:"updatedAt"`
}

type InvoiceItem struct {
	ID              string  `json:"id,omitempty"`
	InvoiceID       string  `json:"invoiceId,omitempty"`
	ProductID       string  `json:"productId,omitempty"`
	Description     string  `json:"description"`
	Quantity        float64 `json:"quantity"`
	Unit            string  `json:"unit,omitempty"`
	UnitPrice       float64 `json:"unitPrice"`
	TaxRate         float64 `json:"taxRate"`
	TaxDefinitionID string  `json:"taxDefinitionId,omitempty"`
	LineTotal       float64 `json:"lineTotal"`
	Notes           string  `json:"notes,omitempty"`
	SortOrder       int     `json:"sortOrder"`
}

type StatusHistoryEntry struct {
	ID            string `json:"id"`
	InvoiceID     string `json:"invoiceId"`
	Status        string `json:"status"`
	ChangedAt     string `json:"changedAt"`
	PaymentMethod string `json:"paymentMethod,omitempty"`
	Note          string `json:"note,omitempty"`
}

type InvoiceWithDetails struct {
	Invoice
	Customer      Customer             `json:"customer"`
	Items         []InvoiceItem        `json:"items"`
	StatusHistory []StatusHistoryEntry `json:"statusHistory"`
	Settings      map[string]string    `json:"settings,omitempty"`
}

type InvoiceInput struct {
	CustomerID         string        `json:"customerId"`
	InvoiceNumber      string        `json:"invoiceNumber"`
	IssueDate          string        `json:"issueDate"`
	DueDate            string        `json:"dueDate"`
	Currency           string        `json:"currency"`
	TaxMode            string        `json:"taxMode"`
	DiscountAmount     float64       `json:"discountAmount"`
	DiscountPercentage float64       `json:"discountPercentage"`
	TaxRate            float64       `json:"taxRate"`
	TaxDefinitionID    string        `json:"taxDefinitionId"`
	PricesIncludeTax   bool          `json:"pricesIncludeTax"`
	RoundingMode       string        `json:"roundingMode"`
	PaymentTerms       string        `json:"paymentTerms"`
	Notes              string        `json:"notes"`
	Items              []InvoiceItem `json:"items"`
}

type Dashboard struct {
	Counts       map[string]int     `json:"counts"`
	StatusCounts map[string]int     `json:"statusCounts"`
	Money        map[string]float64 `json:"money"`
	Recent       []Invoice          `json:"recent"`
	Currency     string             `json:"currency"`
}

type calculatedTotals struct {
	Subtotal       float64
	DiscountAmount float64
	TaxAmount      float64
	Total          float64
}

type TaxDefinition struct {
	ID          string  `json:"id"`
	Code        string  `json:"code,omitempty"`
	Name        string  `json:"name,omitempty"`
	Percent     float64 `json:"percent"`
	CountryCode string  `json:"countryCode,omitempty"`
	IsDefault   bool    `json:"isDefault"`
	CreatedAt   string  `json:"createdAt,omitempty"`
	UpdatedAt   string  `json:"updatedAt,omitempty"`
}

type ProductOption struct {
	ID        string `json:"id"`
	Code      string `json:"code"`
	Name      string `json:"name"`
	SortOrder int    `json:"sortOrder"`
	IsBuiltin bool   `json:"isBuiltin"`
	CreatedAt string `json:"createdAt"`
}

type Template struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	HTML         string `json:"html,omitempty"`
	IsDefault    bool   `json:"isDefault"`
	TemplateType string `json:"templateType"`
	CreatedAt    string `json:"createdAt"`
	Updatable    bool   `json:"updatable,omitempty"`
}
