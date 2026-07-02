package sinvo

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

type htmlPDFRenderer struct {
	path string
	kind string
}

type invoiceXMLProfile struct {
	ID   string
	Name string
}

func normalizeXMLProfileID(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "xrechnung":
		return "xrechnung"
	case "facturx", "factur-x", "zugferd":
		return "facturx"
	case "fatturapa":
		return "fatturapa"
	default:
		return "ubl21"
	}
}

func normalizeLocale(value string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(value)), "en") {
		return "en"
	}
	return "de"
}

func renderInvoiceXML(inv InvoiceWithDetails) (string, invoiceXMLProfile) {
	profile := invoiceXMLProfile{ID: normalizeXMLProfileID(setting(inv.Settings, "xmlProfileId", "ubl21"))}
	names := map[string]string{
		"ubl21":     "UBL 2.1",
		"xrechnung": "XRechnung",
		"facturx":   "Factur-X/ZUGFeRD",
		"fatturapa": "FatturaPA",
	}
	profile.Name = names[profile.ID]
	switch profile.ID {
	case "facturx":
		return renderFacturXXML(inv, profile), profile
	case "fatturapa":
		return renderFatturaPAXML(inv, profile), profile
	default:
		return renderUBLXML(inv, profile), profile
	}
}

func renderUBLXML(inv InvoiceWithDetails, profile invoiceXMLProfile) string {
	settings := inv.Settings
	currency := invoiceCurrency(inv)
	customizationID := "urn:oasis:names:specification:ubl:profile:basic"
	profileID := "urn:oasis:names:specification:ubl:profile:basic"
	if profile.ID == "xrechnung" {
		customizationID = "urn:cen.eu:en16931:2017#compliant#urn:xeinkauf.de:kosit:xrechnung_3.0"
		profileID = "urn:fdc:peppol.eu:2017:poacc:billing:01:1.0"
	}

	var lines strings.Builder
	for i, item := range inv.Items {
		fmt.Fprintf(&lines, `
  <cac:InvoiceLine>
    <cbc:ID>%d</cbc:ID>
    <cbc:InvoicedQuantity unitCode="%s">%.2f</cbc:InvoicedQuantity>
    <cbc:LineExtensionAmount currencyID="%s">%.2f</cbc:LineExtensionAmount>
    <cac:Item>
      <cbc:Description>%s</cbc:Description>
      <cbc:Name>%s</cbc:Name>
      <cac:ClassifiedTaxCategory>
        <cbc:ID>S</cbc:ID>
        <cbc:Percent>%.2f</cbc:Percent>
        <cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme>
      </cac:ClassifiedTaxCategory>
    </cac:Item>
    <cac:Price><cbc:PriceAmount currencyID="%s">%.2f</cbc:PriceAmount></cac:Price>
  </cac:InvoiceLine>`, i+1, xmlEscape(unitCode(item.Unit)), item.Quantity, xmlEscape(currency), item.LineTotal, xmlEscape(item.Notes), xmlEscape(item.Description), item.TaxRate, xmlEscape(currency), item.UnitPrice)
	}

	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2" xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2" xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2">
  <cbc:CustomizationID>%s</cbc:CustomizationID>
  <cbc:ProfileID>%s</cbc:ProfileID>
  <cbc:ID>%s</cbc:ID>
  <cbc:IssueDate>%s</cbc:IssueDate>
  <cbc:DueDate>%s</cbc:DueDate>
  <cbc:InvoiceTypeCode>380</cbc:InvoiceTypeCode>
  <cbc:DocumentCurrencyCode>%s</cbc:DocumentCurrencyCode>
  <cac:AccountingSupplierParty>
    <cac:Party>
      <cac:PartyName><cbc:Name>%s</cbc:Name></cac:PartyName>
      <cac:PostalAddress>
        <cbc:StreetName>%s</cbc:StreetName>
        <cbc:CityName>%s</cbc:CityName>
        <cbc:PostalZone>%s</cbc:PostalZone>
        <cac:Country><cbc:IdentificationCode>%s</cbc:IdentificationCode></cac:Country>
      </cac:PostalAddress>
      <cac:PartyTaxScheme><cbc:CompanyID>%s</cbc:CompanyID><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:PartyTaxScheme>
    </cac:Party>
  </cac:AccountingSupplierParty>
  <cac:AccountingCustomerParty>
    <cac:Party>
      <cac:PartyName><cbc:Name>%s</cbc:Name></cac:PartyName>
      <cac:PostalAddress>
        <cbc:StreetName>%s</cbc:StreetName>
        <cbc:CityName>%s</cbc:CityName>
        <cbc:PostalZone>%s</cbc:PostalZone>
        <cac:Country><cbc:IdentificationCode>%s</cbc:IdentificationCode></cac:Country>
      </cac:PostalAddress>
      <cac:PartyTaxScheme><cbc:CompanyID>%s</cbc:CompanyID><cac:TaxScheme><cbc:ID>VAT</cbc:ID></cac:TaxScheme></cac:PartyTaxScheme>
    </cac:Party>
  </cac:AccountingCustomerParty>
  <cac:PaymentTerms><cbc:Note>%s</cbc:Note></cac:PaymentTerms>
  <cac:TaxTotal><cbc:TaxAmount currencyID="%s">%.2f</cbc:TaxAmount></cac:TaxTotal>
  <cac:LegalMonetaryTotal>
    <cbc:LineExtensionAmount currencyID="%s">%.2f</cbc:LineExtensionAmount>
    <cbc:TaxExclusiveAmount currencyID="%s">%.2f</cbc:TaxExclusiveAmount>
    <cbc:TaxInclusiveAmount currencyID="%s">%.2f</cbc:TaxInclusiveAmount>
    <cbc:PayableAmount currencyID="%s">%.2f</cbc:PayableAmount>
  </cac:LegalMonetaryTotal>%s
</Invoice>
`, xmlEscape(customizationID), xmlEscape(profileID), xmlEscape(inv.InvoiceNumber), xmlEscape(inv.IssueDate), xmlEscape(inv.DueDate), xmlEscape(currency), xmlEscape(setting(settings, "companyName", "")), xmlEscape(settings["companyAddress"]), xmlEscape(settings["companyCity"]), xmlEscape(settings["companyPostalCode"]), xmlEscape(setting(settings, "companyCountryCode", "DE")), xmlEscape(settings["companyTaxId"]), xmlEscape(inv.Customer.Name), xmlEscape(inv.Customer.Address), xmlEscape(inv.Customer.City), xmlEscape(inv.Customer.PostalCode), xmlEscape(setting(map[string]string{"code": inv.Customer.CountryCode}, "code", "DE")), xmlEscape(inv.Customer.TaxID), xmlEscape(invoicePaymentTerms(inv)), xmlEscape(currency), inv.TaxAmount, xmlEscape(currency), inv.Subtotal, xmlEscape(currency), inv.Subtotal-inv.DiscountAmount, xmlEscape(currency), inv.Total, xmlEscape(currency), inv.Total, lines.String())
}

func renderFacturXXML(inv InvoiceWithDetails, profile invoiceXMLProfile) string {
	settings := inv.Settings
	currency := invoiceCurrency(inv)
	var lines strings.Builder
	for i, item := range inv.Items {
		fmt.Fprintf(&lines, `
    <ram:IncludedSupplyChainTradeLineItem>
      <ram:AssociatedDocumentLineDocument><ram:LineID>%d</ram:LineID></ram:AssociatedDocumentLineDocument>
      <ram:SpecifiedTradeProduct><ram:Name>%s</ram:Name><ram:Description>%s</ram:Description></ram:SpecifiedTradeProduct>
      <ram:SpecifiedLineTradeAgreement><ram:NetPriceProductTradePrice><ram:ChargeAmount>%.2f</ram:ChargeAmount></ram:NetPriceProductTradePrice></ram:SpecifiedLineTradeAgreement>
      <ram:SpecifiedLineTradeDelivery><ram:BilledQuantity unitCode="%s">%.2f</ram:BilledQuantity></ram:SpecifiedLineTradeDelivery>
      <ram:SpecifiedLineTradeSettlement>
        <ram:ApplicableTradeTax><ram:TypeCode>VAT</ram:TypeCode><ram:CategoryCode>S</ram:CategoryCode><ram:RateApplicablePercent>%.2f</ram:RateApplicablePercent></ram:ApplicableTradeTax>
        <ram:SpecifiedTradeSettlementLineMonetarySummation><ram:LineTotalAmount>%.2f</ram:LineTotalAmount></ram:SpecifiedTradeSettlementLineMonetarySummation>
      </ram:SpecifiedLineTradeSettlement>
    </ram:IncludedSupplyChainTradeLineItem>`, i+1, xmlEscape(item.Description), xmlEscape(item.Notes), item.UnitPrice, xmlEscape(unitCode(item.Unit)), item.Quantity, item.TaxRate, item.LineTotal)
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<rsm:CrossIndustryInvoice xmlns:rsm="urn:un:unece:uncefact:data:standard:CrossIndustryInvoice:100" xmlns:ram="urn:un:unece:uncefact:data:standard:ReusableAggregateBusinessInformationEntity:100" xmlns:udt="urn:un:unece:uncefact:data:standard:UnqualifiedDataType:100">
  <rsm:ExchangedDocumentContext><ram:GuidelineSpecifiedDocumentContextParameter><ram:ID>urn:factur-x.eu:1p0:basic</ram:ID></ram:GuidelineSpecifiedDocumentContextParameter></rsm:ExchangedDocumentContext>
  <rsm:ExchangedDocument>
    <ram:ID>%s</ram:ID>
    <ram:TypeCode>380</ram:TypeCode>
    <ram:IssueDateTime><udt:DateTimeString format="102">%s</udt:DateTimeString></ram:IssueDateTime>
  </rsm:ExchangedDocument>
  <rsm:SupplyChainTradeTransaction>%s
    <ram:ApplicableHeaderTradeAgreement>
      <ram:SellerTradeParty><ram:Name>%s</ram:Name></ram:SellerTradeParty>
      <ram:BuyerTradeParty><ram:Name>%s</ram:Name></ram:BuyerTradeParty>
    </ram:ApplicableHeaderTradeAgreement>
    <ram:ApplicableHeaderTradeDelivery><ram:ActualDeliverySupplyChainEvent><ram:OccurrenceDateTime><udt:DateTimeString format="102">%s</udt:DateTimeString></ram:OccurrenceDateTime></ram:ActualDeliverySupplyChainEvent></ram:ApplicableHeaderTradeDelivery>
    <ram:ApplicableHeaderTradeSettlement>
      <ram:InvoiceCurrencyCode>%s</ram:InvoiceCurrencyCode>
      <ram:SpecifiedTradeSettlementHeaderMonetarySummation>
        <ram:LineTotalAmount>%.2f</ram:LineTotalAmount>
        <ram:TaxBasisTotalAmount>%.2f</ram:TaxBasisTotalAmount>
        <ram:TaxTotalAmount currencyID="%s">%.2f</ram:TaxTotalAmount>
        <ram:GrandTotalAmount>%.2f</ram:GrandTotalAmount>
        <ram:DuePayableAmount>%.2f</ram:DuePayableAmount>
      </ram:SpecifiedTradeSettlementHeaderMonetarySummation>
    </ram:ApplicableHeaderTradeSettlement>
  </rsm:SupplyChainTradeTransaction>
</rsm:CrossIndustryInvoice>
`, xmlEscape(inv.InvoiceNumber), compactDate(inv.IssueDate), lines.String(), xmlEscape(setting(settings, "companyName", "")), xmlEscape(inv.Customer.Name), compactDate(inv.IssueDate), xmlEscape(currency), inv.Subtotal, inv.Subtotal-inv.DiscountAmount, xmlEscape(currency), inv.TaxAmount, inv.Total, inv.Total)
}

func renderFatturaPAXML(inv InvoiceWithDetails, profile invoiceXMLProfile) string {
	settings := inv.Settings
	currency := invoiceCurrency(inv)
	var lines strings.Builder
	for i, item := range inv.Items {
		fmt.Fprintf(&lines, `
        <DettaglioLinee>
          <NumeroLinea>%d</NumeroLinea>
          <Descrizione>%s</Descrizione>
          <Quantita>%.2f</Quantita>
          <UnitaMisura>%s</UnitaMisura>
          <PrezzoUnitario>%.2f</PrezzoUnitario>
          <PrezzoTotale>%.2f</PrezzoTotale>
          <AliquotaIVA>%.2f</AliquotaIVA>
        </DettaglioLinee>`, i+1, xmlEscape(item.Description), item.Quantity, xmlEscape(item.Unit), item.UnitPrice, item.LineTotal, item.TaxRate)
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<p:FatturaElettronica versione="FPR12" xmlns:p="http://ivaservizi.agenziaentrate.gov.it/docs/xsd/fatture/v1.2">
  <FatturaElettronicaHeader>
    <CedentePrestatore><DatiAnagrafici><IdFiscaleIVA><IdPaese>%s</IdPaese><IdCodice>%s</IdCodice></IdFiscaleIVA><Anagrafica><Denominazione>%s</Denominazione></Anagrafica></DatiAnagrafici></CedentePrestatore>
    <CessionarioCommittente><DatiAnagrafici><IdFiscaleIVA><IdPaese>%s</IdPaese><IdCodice>%s</IdCodice></IdFiscaleIVA><Anagrafica><Denominazione>%s</Denominazione></Anagrafica></DatiAnagrafici></CessionarioCommittente>
  </FatturaElettronicaHeader>
  <FatturaElettronicaBody>
    <DatiGenerali><DatiGeneraliDocumento><TipoDocumento>TD01</TipoDocumento><Divisa>%s</Divisa><Data>%s</Data><Numero>%s</Numero><ImportoTotaleDocumento>%.2f</ImportoTotaleDocumento></DatiGeneraliDocumento></DatiGenerali>
    <DatiBeniServizi>%s
      <DatiRiepilogo><AliquotaIVA>%.2f</AliquotaIVA><ImponibileImporto>%.2f</ImponibileImporto><Imposta>%.2f</Imposta></DatiRiepilogo>
    </DatiBeniServizi>
  </FatturaElettronicaBody>
</p:FatturaElettronica>
`, xmlEscape(setting(settings, "companyCountryCode", "DE")), xmlEscape(settings["companyTaxId"]), xmlEscape(setting(settings, "companyName", "")), xmlEscape(setting(map[string]string{"code": inv.Customer.CountryCode}, "code", "DE")), xmlEscape(inv.Customer.TaxID), xmlEscape(inv.Customer.Name), xmlEscape(currency), xmlEscape(inv.IssueDate), xmlEscape(inv.InvoiceNumber), inv.Total, lines.String(), inv.TaxRate, inv.Subtotal-inv.DiscountAmount, inv.TaxAmount)
}

func (a *App) renderInvoicePDF(inv InvoiceWithDetails) ([]byte, error) {
	html, err := a.renderInvoiceHTML(inv)
	if err != nil {
		return nil, err
	}
	html = a.prepareInvoiceHTMLForPDF(html)
	pdf, err := a.renderHTMLToPDF(html)
	if err != nil {
		return nil, err
	}

	xml, profile := renderInvoiceXML(inv)
	attachmentName := ""
	if setting(inv.Settings, "embedXmlInPdf", "false") == "true" {
		base := safeFileName(inv.InvoiceNumber)
		if base == "" {
			base = inv.ID
		}
		attachmentName = base + "-" + profile.ID + ".xml"
	}
	if attachmentName != "" {
		return attachXMLToPDF(pdf, attachmentName, xml), nil
	}
	return pdf, nil
}

func embedInvoiceXMLInHTML(htmlDoc string, xml string, profile invoiceXMLProfile) string {
	block := fmt.Sprintf("\n<script type=\"application/xml\" id=\"invoice-xml\" data-profile=\"%s\">\n%s\n</script>\n", htmlEscape(profile.ID), htmlEscape(xml))
	index := strings.LastIndex(strings.ToLower(htmlDoc), "</body>")
	if index >= 0 {
		return htmlDoc[:index] + block + htmlDoc[index:]
	}
	return htmlDoc + block
}

func (a *App) prepareInvoiceHTMLForPDF(html string) string {
	logoBase := fileURL(filepath.Join(a.paths.DataDir, "logos")) + "/"
	html = strings.ReplaceAll(html, `src="/api/logos/`, `src="`+logoBase)
	html = strings.ReplaceAll(html, `src='/api/logos/`, `src='`+logoBase)
	return html
}

func (a *App) renderHTMLToPDF(html string) ([]byte, error) {
	renderers, err := a.findHTMLToPDFRenderers()
	if err != nil {
		return nil, err
	}
	htmlFile, err := os.CreateTemp(a.paths.ExportsDir, "invoice-*.html")
	if err != nil {
		return nil, err
	}
	htmlPath := htmlFile.Name()
	if _, err := htmlFile.WriteString(html); err != nil {
		_ = htmlFile.Close()
		_ = os.Remove(htmlPath)
		return nil, err
	}
	if err := htmlFile.Close(); err != nil {
		_ = os.Remove(htmlPath)
		return nil, err
	}
	pdfFile, err := os.CreateTemp(a.paths.ExportsDir, "invoice-*.pdf")
	if err != nil {
		_ = os.Remove(htmlPath)
		return nil, err
	}
	pdfPath := pdfFile.Name()
	_ = pdfFile.Close()
	_ = os.Remove(pdfPath)
	defer os.Remove(htmlPath)
	defer os.Remove(pdfPath)

	failures := []string{}
	for _, renderer := range renderers {
		_ = os.Remove(pdfPath)
		cmd := htmlToPDFCommand(renderer, htmlPath, pdfPath)
		out, err := cmd.CombinedOutput()
		if err != nil {
			failures = append(failures, rendererError(renderer, out, err))
			continue
		}
		body, err := os.ReadFile(pdfPath)
		if err != nil {
			failures = append(failures, renderer.path+": "+err.Error())
			continue
		}
		if len(body) == 0 || !bytes.HasPrefix(body, []byte("%PDF")) {
			failures = append(failures, renderer.path+": renderer did not create a valid PDF")
			continue
		}
		if len(failures) > 0 {
			log.Printf("PDF renderer fallback succeeded with %s after errors: %s", renderer.path, strings.Join(failures, " | "))
		}
		return body, nil
	}
	return nil, fmt.Errorf("pdf template rendering failed; tried %d renderer(s): %s", len(renderers), strings.Join(failures, " | "))
}

func htmlToPDFCommand(renderer htmlPDFRenderer, htmlPath, pdfPath string) *exec.Cmd {
	switch renderer.kind {
	case "wkhtmltopdf":
		return exec.Command(renderer.path, "--quiet", htmlPath, pdfPath)
	case "weasyprint":
		return exec.Command(renderer.path, htmlPath, pdfPath)
	default:
		return exec.Command(renderer.path,
			"--headless",
			"--disable-gpu",
			"--disable-dev-shm-usage",
			"--no-sandbox",
			"--print-to-pdf="+pdfPath,
			"--print-to-pdf-no-header",
			fileURL(htmlPath),
		)
	}
}

func rendererError(renderer htmlPDFRenderer, out []byte, err error) string {
	msg := strings.TrimSpace(string(out))
	if msg == "" {
		msg = err.Error()
	}
	return renderer.path + ": " + msg
}

func (a *App) findHTMLToPDFRenderers() ([]htmlPDFRenderer, error) {
	platform := runtime.GOARCH + "-" + runtime.GOOS
	candidates := []struct {
		path string
		kind string
	}{
		{os.Getenv("SINVO_GO_PDF_RENDERER"), ""},
		{filepath.Join(a.paths.BaseDir, "tools", platform, "chrome-headless-shell"), "chrome"},
		{filepath.Join(a.paths.BaseDir, "tools", platform, "chrome-headless-shell.exe"), "chrome"},
		{filepath.Join(a.paths.BaseDir, "tools", platform, "headless_shell"), "chrome"},
		{filepath.Join(a.paths.BaseDir, "tools", platform, "headless_shell.exe"), "chrome"},
		{filepath.Join(a.paths.BaseDir, "tools", platform, "chrome"), "chrome"},
		{filepath.Join(a.paths.BaseDir, "tools", platform, "chrome.exe"), "chrome"},
		{filepath.Join(a.paths.BaseDir, "tools", platform, "wkhtmltopdf"), "wkhtmltopdf"},
		{filepath.Join(a.paths.BaseDir, "tools", platform, "wkhtmltopdf.exe"), "wkhtmltopdf"},
		{filepath.Join(a.paths.BaseDir, "tools", "chrome", "chrome"), "chrome"},
		{filepath.Join(a.paths.BaseDir, "tools", "chrome", "chrome.exe"), "chrome"},
		{filepath.Join(a.paths.BaseDir, "tools", "chromium", "chrome"), "chrome"},
		{filepath.Join(a.paths.BaseDir, "tools", "chromium", "chrome.exe"), "chrome"},
		{filepath.Join(a.paths.BaseDir, "tools", "wkhtmltopdf", "wkhtmltopdf"), "wkhtmltopdf"},
		{filepath.Join(a.paths.BaseDir, "tools", "wkhtmltopdf", "wkhtmltopdf.exe"), "wkhtmltopdf"},
	}
	renderers := []htmlPDFRenderer{}
	seen := map[string]bool{}
	for _, item := range candidates {
		if item.path == "" {
			continue
		}
		if fileExists(item.path) && !seen[item.path] {
			seen[item.path] = true
			renderers = append(renderers, htmlPDFRenderer{path: item.path, kind: pdfRendererKind(item.path, item.kind)})
		}
	}
	for _, name := range []string{"google-chrome-stable", "google-chrome", "chromium", "chromium-browser", "msedge", "wkhtmltopdf", "weasyprint"} {
		path, err := exec.LookPath(name)
		if err == nil && !seen[path] {
			seen[path] = true
			renderers = append(renderers, htmlPDFRenderer{path: path, kind: pdfRendererKind(path, "")})
		}
	}
	if len(renderers) == 0 {
		return nil, errors.New("no HTML-to-PDF renderer found; put chrome-headless-shell under app/tools/" + platform + " or set SINVO_GO_PDF_RENDERER")
	}
	return renderers, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func pdfRendererKind(path, fallback string) string {
	if fallback != "" {
		return fallback
	}
	name := strings.ToLower(filepath.Base(path))
	switch {
	case strings.Contains(name, "wkhtmltopdf"):
		return "wkhtmltopdf"
	case strings.Contains(name, "weasyprint"):
		return "weasyprint"
	default:
		return "chrome"
	}
}

func fileURL(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	slashed := filepath.ToSlash(abs)
	if runtime.GOOS == "windows" && !strings.HasPrefix(slashed, "/") {
		slashed = "/" + slashed
	}
	return (&url.URL{Scheme: "file", Path: slashed}).String()
}

func attachXMLToPDF(pdf []byte, attachmentName, attachmentXML string) []byte {
	rootID, rootDict, ok := findPDFRoot(pdf)
	if !ok {
		return pdf
	}
	rootDict = strings.TrimSpace(rootDict)
	if strings.Contains(rootDict, "/Names") {
		return pdf
	}
	maxID := maxPDFObjectID(pdf)
	fileSpecID := maxID + 1
	embeddedID := maxID + 2
	updatedRoot := strings.TrimSuffix(rootDict, ">>") +
		fmt.Sprintf(" /Names << /EmbeddedFiles << /Names [(%s) %d 0 R] >> >> >>", pdfLiteralString(attachmentName), fileSpecID)
	prev := previousPDFStartXref(pdf)

	var out bytes.Buffer
	out.Write(pdf)
	if !bytes.HasSuffix(pdf, []byte("\n")) {
		out.WriteByte('\n')
	}
	rootOffset := out.Len()
	fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", rootID, updatedRoot)
	fileSpecOffset := out.Len()
	fmt.Fprintf(&out, "%d 0 obj\n<< /Type /Filespec /F (%s) /UF (%s) /Desc (Invoice XML) /EF << /F %d 0 R /UF %d 0 R >> >>\nendobj\n",
		fileSpecID, pdfLiteralString(attachmentName), pdfLiteralString(attachmentName), embeddedID, embeddedID)
	embeddedOffset := out.Len()
	fmt.Fprintf(&out, "%d 0 obj\n<< /Type /EmbeddedFile /Subtype /application#2Fxml /Length %d /Params << /Size %d >> >>\nstream\n%s\nendstream\nendobj\n",
		embeddedID, len(attachmentXML), len(attachmentXML), attachmentXML)
	xref := out.Len()
	fmt.Fprintf(&out, "xref\n%d 1\n%010d 00000 n \n%d 2\n%010d 00000 n \n%010d 00000 n \n",
		rootID, rootOffset, fileSpecID, fileSpecOffset, embeddedOffset)
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root %d 0 R", embeddedID+1, rootID)
	if prev > 0 {
		fmt.Fprintf(&out, " /Prev %d", prev)
	}
	fmt.Fprintf(&out, " >>\nstartxref\n%d\n%%%%EOF\n", xref)
	return out.Bytes()
}

func findPDFRoot(pdf []byte) (int, string, bool) {
	rootMatch := regexp.MustCompile(`/Root\s+(\d+)\s+0\s+R`).FindSubmatch(pdf)
	if len(rootMatch) != 2 {
		return 0, "", false
	}
	rootID, err := strconv.Atoi(string(rootMatch[1]))
	if err != nil {
		return 0, "", false
	}
	prefix := []byte(fmt.Sprintf("%d 0 obj", rootID))
	start := bytes.Index(pdf, prefix)
	if start < 0 {
		return 0, "", false
	}
	dictStart := bytes.Index(pdf[start:], []byte("<<"))
	if dictStart < 0 {
		return 0, "", false
	}
	dictStart += start
	depth := 0
	for i := dictStart; i < len(pdf)-1; i++ {
		if pdf[i] == '<' && pdf[i+1] == '<' {
			depth++
			i++
			continue
		}
		if pdf[i] == '>' && pdf[i+1] == '>' {
			depth--
			i++
			if depth == 0 {
				return rootID, string(pdf[dictStart : i+1]), true
			}
		}
	}
	return 0, "", false
}

func maxPDFObjectID(pdf []byte) int {
	maxID := 0
	matches := regexp.MustCompile(`(?m)^(\d+)\s+0\s+obj`).FindAllSubmatch(pdf, -1)
	for _, match := range matches {
		id, err := strconv.Atoi(string(match[1]))
		if err == nil && id > maxID {
			maxID = id
		}
	}
	return maxID
}

func previousPDFStartXref(pdf []byte) int {
	matches := regexp.MustCompile(`startxref\s+(\d+)`).FindAllSubmatch(pdf, -1)
	if len(matches) == 0 {
		return 0
	}
	prev, _ := strconv.Atoi(string(matches[len(matches)-1][1]))
	return prev
}

func pdfLiteralString(s string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `(`, `\(`, `)`, `\)`)
	return replacer.Replace(s)
}

func xmlEscape(s string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;")
	return replacer.Replace(s)
}

func invoiceCurrency(inv InvoiceWithDetails) string {
	if strings.TrimSpace(inv.Currency) != "" {
		return strings.TrimSpace(inv.Currency)
	}
	return setting(inv.Settings, "currency", "EUR")
}

func invoicePaymentTerms(inv InvoiceWithDetails) string {
	if strings.TrimSpace(inv.PaymentTerms) != "" {
		return inv.PaymentTerms
	}
	return inv.Settings["paymentTerms"]
}

func unitCode(unit string) string {
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "h", "std", "hour", "stunde":
		return "HUR"
	case "kg":
		return "KGM"
	case "m":
		return "MTR"
	default:
		return "C62"
	}
}

func compactDate(value string) string {
	return strings.ReplaceAll(value, "-", "")
}
