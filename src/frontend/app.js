const state = {
  settings: {},
  customers: [],
  products: [],
  customerList: [],
  productList: [],
  invoiceList: [],
  taxDefinitions: [],
  productCategories: [],
  productUnits: [],
  templates: [],
  listPages: {
    customers: { page: 0, query: '', total: 0, hasPrev: false, hasNext: false },
    products: { page: 0, query: '', total: 0, hasPrev: false, hasNext: false },
    invoices: { page: 0, query: '', total: 0, hasPrev: false, hasNext: false }
  }
};
let draggedItemRow = null;

$(init);

function init() {
  bindEvents();
  loadInstance();
  resetInvoiceForm();
  loadAll();
}

async function loadInstance() {
  try {
    const instance = await api('/instance');
    $('#app-version').text(instance.version ? `v${instance.version}` : '');
  } catch {
    $('#app-version').text('');
  }
}

function bindEvents() {
  $('.nav-button').on('click', function () {
    showView($(this).data('view'));
  });

  $('#backup-now').on('click', async function () {
    await runAction(() => api('/backup', 'POST', {}), 'Backup erstellt');
  });
  $('#shutdown-app').on('click', async function () {
    await runAction(() => api('/shutdown', 'POST', {}), 'App wird beendet');
    setTimeout(() => window.location.reload(), 500);
  });

  $('#new-customer').on('click', resetCustomerForm);
  $('#new-product').on('click', resetProductForm);
  $('#new-invoice').on('click', resetInvoiceForm);
  $('#add-item').on('click', () => addItemRow());

  $('#customer-form').on('submit', saveCustomer);
  $('#product-form').on('submit', saveProduct);
  $('#invoice-form').on('submit', saveInvoice);
  $('.settings-form').on('submit', saveSettings);
	  $('#tax-form').on('submit', saveTaxDefinition);
	  $('#category-form').on('submit', saveCategory);
	  $('#unit-form').on('submit', saveUnit);
	  $('#template-upload-form').on('submit', uploadTemplate);
	  $('#template-install-form').on('submit', installTemplate);
	  $('#logo-upload-button').on('click', uploadLogo);
	  $('#settings-branding-form [name=logo]').on('input change', renderLogoPreview);
  $('.settings-tab').on('click', function () {
    showSettingsSection($(this).data('settings-section'));
  });
  $('#settings-section-select').on('change', function () {
    showSettingsSection($(this).val());
  });

  $(document).on('input change', '#invoice-form input, #invoice-form select, #invoice-form textarea', updateInvoicePreview);
  $(document).on('click', '.remove-item', function () {
    $(this).closest('tr').remove();
    updateInvoicePreview();
  });
  $(document).on('click', '.move-item-up', function () {
    const row = $(this).closest('tr');
    row.prev().before(row);
    updateInvoicePreview();
  });
  $(document).on('click', '.move-item-down', function () {
    const row = $(this).closest('tr');
    row.next().after(row);
    updateInvoicePreview();
  });
  $(document).on('dragstart', '#invoice-items tr', function (event) {
    draggedItemRow = this;
    event.originalEvent.dataTransfer.effectAllowed = 'move';
  });
  $(document).on('dragover', '#invoice-items tr', function (event) {
    event.preventDefault();
    event.originalEvent.dataTransfer.dropEffect = 'move';
  });
  $(document).on('drop', '#invoice-items tr', function (event) {
    event.preventDefault();
    if (!draggedItemRow || draggedItemRow === this) return;
    const rows = Array.from($('#invoice-items tr'));
    const from = rows.indexOf(draggedItemRow);
    const to = rows.indexOf(this);
    if (from < to) $(this).after(draggedItemRow);
    else $(this).before(draggedItemRow);
    draggedItemRow = null;
    updateInvoicePreview();
  });
  $(document).on('change', '.item-product', function () {
    const product = state.products.find(p => p.id === $(this).val());
    if (!product) return;
    const row = $(this).closest('tr');
    row.find('.item-desc').val(product.description || product.name);
    row.find('.item-unit').val(product.unit || 'Stk');
    row.find('.item-price').val(numberInput(product.unitPrice));
    row.find('.item-tax-definition').val(product.taxDefinitionId || '');
    const taxDef = state.taxDefinitions.find(t => t.id === product.taxDefinitionId);
    row.find('.item-tax-rate').val(taxDef ? numberInput(taxDef.percent) : '0.00');
    if (product.category) $('#product-form [name=category]').val(product.category);
    updateInvoicePreview();
  });
  $(document).on('change', '#invoice-form [name=taxDefinitionId]', function () {
    const taxDef = state.taxDefinitions.find(t => t.id === $(this).val());
    if (taxDef) $('#invoice-form [name=taxRate]').val(numberInput(taxDef.percent));
    updateInvoicePreview();
  });
  $(document).on('change', '.item-tax-definition', function () {
    const taxDef = state.taxDefinitions.find(t => t.id === $(this).val());
    $(this).closest('tr').find('.item-tax-rate').val(taxDef ? numberInput(taxDef.percent) : '0.00');
    updateInvoicePreview();
  });
  $(document).on('keydown', '.list-search', async function (event) {
    if (event.key !== 'Enter') return;
    event.preventDefault();
    const list = $(this).data('list');
    state.listPages[list].query = $(this).val().trim();
    state.listPages[list].page = 0;
    await reloadList(list);
  });
  $(document).on('click', '.list-page', async function () {
    const list = $(this).data('list');
    const direction = $(this).data('page');
    const meta = state.listPages[list];
    if (direction === 'prev' && !meta.hasPrev) return;
    if (direction === 'next' && !meta.hasNext) return;
    meta.page += direction === 'next' ? 1 : -1;
    await reloadList(list);
  });

  $(document).on('click', '[data-action]', handleTableAction);
}

async function loadAll() {
  try {
    state.settings = await api('/settings');
    state.customers = await api('/customers');
    state.products = await api('/products?includeInactive=1');
    state.taxDefinitions = await api('/tax-definitions');
    state.productCategories = await api('/product-categories');
    state.productUnits = await api('/product-units');
    state.templates = await api('/templates');
    await loadListPages();
    renderAll();
  } catch (err) {
    showMessage(err.message, true);
  }
}

async function loadListPages() {
  await Promise.all([
    loadListPage('customers'),
    loadListPage('products'),
    loadListPage('invoices')
  ]);
}

async function loadListPage(list) {
  const meta = state.listPages[list];
  const q = encodeURIComponent(meta.query || '');
  const page = encodeURIComponent(meta.page || 0);
  const paths = {
    customers: `/customers?paged=1&page=${page}&q=${q}`,
    products: `/products?paged=1&includeInactive=1&page=${page}&q=${q}`,
    invoices: `/invoices?paged=1&page=${page}&q=${q}`
  };
  const result = await api(paths[list]);
  meta.page = result.page || 0;
  meta.total = result.total || 0;
  meta.hasPrev = !!result.hasPrev;
  meta.hasNext = !!result.hasNext;
  meta.query = result.query || meta.query || '';
  if (list === 'customers') state.customerList = result.items || [];
  if (list === 'products') state.productList = result.items || [];
  if (list === 'invoices') state.invoiceList = result.items || [];
}

async function reloadList(list) {
  try {
    await loadListPage(list);
    if (list === 'customers') renderCustomers();
    if (list === 'products') renderProducts();
    if (list === 'invoices') renderInvoices();
    renderListControls();
  } catch (err) {
    showMessage(err.message, true);
  }
}

function renderAll() {
  renderTemplates();
  renderSettings();
  renderCustomers();
  renderProducts();
  renderTaxDefinitions();
  renderProductOptions();
  renderInvoiceCustomerOptions();
  renderInvoiceProductOptions();
  renderInvoiceTaxOptions();
  renderInvoices();
  renderDashboard();
  renderListControls();
  updateInvoicePreview();
}

function renderListControls() {
  Object.entries(state.listPages).forEach(([list, meta]) => {
    $(`.list-search[data-list=${list}]`).val(meta.query || '');
    $(`.list-page[data-list=${list}][data-page=prev]`).prop('disabled', !meta.hasPrev);
    $(`.list-page[data-list=${list}][data-page=next]`).prop('disabled', !meta.hasNext);
  });
}

async function renderDashboard() {
  const data = await api('/dashboard');
  const currency = data.currency || 'EUR';
  $('#dashboard-cards').html([
    metric('Rechnungen', data.counts.invoices),
    metric('Kunden', data.counts.customers),
    metric('Offen', (data.statusCounts.sent || 0) + (data.statusCounts.overdue || 0)),
    metric('Bezahlt', money(data.money.paid, currency)),
    metric('Ausstehend', money(data.money.outstanding, currency))
  ].join(''));

  $('#recent-invoices').html((data.recent || []).map(inv => `
    <tr draggable="true">
      <td>${esc(inv.invoiceNumber)}</td>
      <td>${esc(inv.customerName)}</td>
      <td>${statusBadge(inv.status)}</td>
      <td>${esc(inv.issueDate)}</td>
      <td class="num">${money(inv.total, inv.currency)}</td>
    </tr>
  `).join('') || emptyRow(5));
}

function renderCustomers() {
  $('#customers-table').html(state.customerList.map(c => `
    <tr>
      <td>${esc(c.name)}</td>
      <td>${esc(c.email)}</td>
      <td>${esc([c.postalCode, c.city].filter(Boolean).join(' '))}</td>
      <td>${esc(c.countryCode)}</td>
      <td><div class="actions">
        <button class="mini secondary" data-action="edit-customer" data-id="${c.id}">Bearbeiten</button>
        <button class="mini danger" data-action="delete-customer" data-id="${c.id}">Löschen</button>
      </div></td>
    </tr>
  `).join('') || emptyRow(5));
}

function renderProducts() {
  $('#products-table').html(state.productList.map(p => `
    <tr>
      <td>${esc(p.name)}</td>
      <td>${esc(p.sku)}</td>
      <td>${esc(p.unit)}</td>
      <td>${esc(p.category)}</td>
      <td class="num">${money(p.unitPrice, state.settings.currency || 'EUR')}</td>
      <td>${p.isActive ? '<span class="badge paid">Aktiv</span>' : '<span class="badge voided">Inaktiv</span>'}</td>
      <td><div class="actions">
        <button class="mini secondary" data-action="edit-product" data-id="${p.id}">Bearbeiten</button>
        ${p.isActive
          ? `<button class="mini danger" data-action="delete-product" data-id="${p.id}">Deaktivieren</button>`
          : `<button class="mini secondary" data-action="activate-product" data-id="${p.id}">Aktivieren</button>`}
      </div></td>
    </tr>
  `).join('') || emptyRow(7));
}

function renderTaxDefinitions() {
  $('#tax-table').html(state.taxDefinitions.map(t => `
    <tr>
      <td>${esc(t.code)}</td>
      <td>${esc(t.name)}</td>
      <td class="num">${numberInput(t.percent)}</td>
      <td>${t.isDefault ? '<span class="badge paid">Standard</span>' : ''}</td>
      <td><div class="actions">
        <button type="button" class="mini secondary" data-action="edit-tax" data-id="${t.id}">Bearbeiten</button>
        <button type="button" class="mini danger" data-action="delete-tax" data-id="${t.id}">Löschen</button>
      </div></td>
    </tr>
  `).join('') || emptyRow(5));
}

function renderProductOptions() {
  $('#categories-table').html(state.productCategories.map(o => optionRow(o, 'category')).join('') || emptyRow(4));
  $('#units-table').html(state.productUnits.map(o => optionRow(o, 'unit')).join('') || emptyRow(4));

  const categoryOptions = ['<option value="">Keine</option>'].concat(
    state.productCategories.map(o => `<option value="${escAttr(o.code)}">${esc(o.name)} (${esc(o.code)})</option>`)
  ).join('');
  const unitOptions = ['<option value="">Keine</option>'].concat(
    state.productUnits.map(o => `<option value="${escAttr(o.code)}">${esc(o.name)} (${esc(o.code)})</option>`)
  ).join('');
  const taxOptions = taxDefinitionOptions('Keine');

  const currentCategory = $('#product-form [name=category]').val();
  const currentUnit = $('#product-form [name=unit]').val();
  const currentTax = $('#product-form [name=taxDefinitionId]').val();
  $('#product-form [name=category]').html(categoryOptions).val(currentCategory || '');
  $('#product-form [name=unit]').html(unitOptions).val(currentUnit || 'Stk');
  $('#product-form [name=taxDefinitionId]').html(taxOptions).val(currentTax || '');
}

function optionRow(option, type) {
  return `
    <tr>
      <td>${esc(option.code)}</td>
      <td>${esc(option.name)}</td>
      <td class="num">${esc(option.sortOrder)}</td>
      <td><div class="actions">
        <button type="button" class="mini secondary" data-action="edit-${type}" data-id="${option.id}">Bearbeiten</button>
        ${option.isBuiltin ? '' : `<button type="button" class="mini danger" data-action="delete-${type}" data-id="${option.id}">Löschen</button>`}
      </div></td>
    </tr>
  `;
}

function renderInvoices() {
  $('#invoices-table').html(state.invoiceList.map(inv => `
    <tr>
      <td>${esc(inv.invoiceNumber)}</td>
      <td>${esc(inv.customerName)}</td>
      <td>${statusBadge(inv.status)}</td>
      <td>${esc(inv.issueDate)}</td>
      <td class="num">${money(inv.total, inv.currency)}</td>
      <td><div class="actions">${invoiceActions(inv)}</div></td>
    </tr>
  `).join('') || emptyRow(6));
}

function invoiceActions(inv) {
  const id = inv.id;
  const items = [
    `<button class="mini secondary" data-action="view-invoice" data-id="${id}">Ansehen</button>`,
    `<button class="mini secondary" data-action="export-html" data-id="${id}">HTML</button>`,
    `<button class="mini secondary" data-action="export-pdf" data-id="${id}">PDF</button>`,
    `<button class="mini secondary" data-action="export-xml" data-id="${id}">XML</button>`,
    `<button class="mini secondary" data-action="export-json" data-id="${id}">JSON</button>`,
    `<button class="mini secondary" data-action="export-csv" data-id="${id}">CSV</button>`,
    `<button class="mini secondary" data-action="duplicate-invoice" data-id="${id}">Duplizieren</button>`
  ];
  if (canChangeInvoice(inv)) {
    items.unshift(`<button class="mini secondary" data-action="edit-invoice" data-id="${id}">Bearbeiten</button>`);
  }
  if (inv.status === 'draft') {
    items.push(`<button class="mini" data-action="send-invoice" data-id="${id}">Senden</button>`);
  }
  if (inv.status === 'sent' || inv.status === 'overdue') {
    items.push(`<button class="mini" data-action="paid-invoice" data-id="${id}">Bezahlt</button>`);
    items.push(`<button class="mini danger" data-action="void-invoice" data-id="${id}">Storno</button>`);
  }
  if (canDeleteInvoice(inv)) {
    items.push(`<button class="mini danger" data-action="delete-invoice" data-id="${id}">Löschen</button>`);
  }
  return items.join('');
}

function canChangeInvoice(inv) {
  return inv.status === 'draft' || (state.settings.allowProtectedInvoiceChanges === 'true' && inv.status !== 'voided');
}

function canDeleteInvoice(inv) {
  return inv.status === 'draft' || inv.status === 'voided' || state.settings.allowProtectedInvoiceChanges === 'true';
}

function renderInvoiceCustomerOptions() {
  const options = ['<option value="">Kunde wählen</option>'].concat(state.customers.map(c => `<option value="${c.id}">${esc(c.name)}</option>`));
  $('#invoice-form [name=customerId]').html(options.join(''));
}

function renderInvoiceProductOptions() {
  const options = ['<option value="">Frei</option>'].concat(
    state.products.filter(p => p.isActive).map(p => `<option value="${p.id}">${esc(p.name)}</option>`)
  ).join('');
  $('#invoice-items .item-product').each(function () {
    const value = $(this).val();
    $(this).html(options).val(value || '');
  });
}

function renderInvoiceTaxOptions() {
  const current = $('#invoice-form [name=taxDefinitionId]').val();
  $('#invoice-form [name=taxDefinitionId]').html(taxDefinitionOptions('Keine')).val(current || '');
  if (!$('#invoice-form [name=id]').val() && !current) {
    applyDefaultInvoiceTax();
  }
  $('#invoice-items .item-tax-definition').each(function () {
    const value = $(this).val();
    $(this).html(taxDefinitionOptions('Keine')).val(value || '');
  });
}

function taxDefinitionOptions(emptyLabel) {
  return [`<option value="">${esc(emptyLabel)}</option>`].concat(
    state.taxDefinitions.map(t => `<option value="${t.id}">${esc(t.name || t.code || t.id)} (${numberInput(t.percent)}%)${t.isDefault ? ' Standard' : ''}</option>`)
  ).join('');
}

function renderSettings() {
  $('.settings-form').each(function () {
    const form = this;
    clearFormFields(form);
    Object.entries(state.settings).forEach(([key, value]) => {
      const field = form.elements[key];
      if (!field) return;
      if (field.type === 'checkbox') {
        field.checked = value === 'true';
      } else {
        field.value = value;
      }
    });
  });
  renderLogoPreview();
}

function renderLogoPreview() {
  const logo = $('#settings-branding-form [name=logo]').val() || state.settings.logo || '';
  $('#logo-preview-wrap').prop('hidden', !logo);
  if (logo) $('#logo-preview').attr('src', logo);
  else $('#logo-preview').removeAttr('src');
}

function renderTemplates() {
  const select = $('#settings-template-select');
  const selected = state.settings.templateId || '';
  const options = ['<option value="">Default</option>'].concat(
    state.templates.map(t => `<option value="${escAttr(t.id)}">${esc(t.name)}</option>`)
  ).join('');
  select.html(options).val(selected);

  $('#templates-list').html(state.templates.map(t => `
    <div class="template-card">
      <div>
        <h3>${esc(t.name)}</h3>
        <div class="template-meta">
          <span>${esc(t.id)}</span>
          <span>${esc(t.templateType || 'local')}</span>
          ${t.isDefault ? '<span class="badge paid">Default</span>' : ''}
        </div>
      </div>
      <div class="actions">
        <button type="button" class="mini secondary" data-action="preview-template" data-id="${escAttr(t.id)}">Preview</button>
        ${t.updatable ? `<button type="button" class="mini secondary" data-action="update-template" data-id="${escAttr(t.id)}">Update</button>` : ''}
        <button type="button" class="mini danger" data-action="delete-template" data-id="${escAttr(t.id)}" ${t.isDefault ? 'disabled' : ''}>Löschen</button>
      </div>
    </div>
  `).join('') || '<p class="muted">Keine Templates installiert.</p>');
}

async function saveCustomer(event) {
  event.preventDefault();
  const data = formObject('#customer-form');
  const id = data.id;
  delete data.id;
  await runAction(async () => {
    if (id) await api(`/customers/${id}`, 'PUT', data);
    else await api('/customers', 'POST', data);
    resetCustomerForm();
    await loadAll();
  }, 'Kunde gespeichert');
}

async function saveProduct(event) {
  event.preventDefault();
  const data = formObject('#product-form');
  data.unitPrice = toNumber(data.unitPrice);
  const id = data.id;
  delete data.id;
  await runAction(async () => {
    if (id) await api(`/products/${id}`, 'PUT', data);
    else await api('/products', 'POST', data);
    resetProductForm();
    await loadAll();
  }, 'Produkt gespeichert');
}

async function saveInvoice(event) {
  event.preventDefault();
  const payload = collectInvoicePayload();
  const id = $('#invoice-form [name=id]').val();
  await runAction(async () => {
    if (id) await api(`/invoices/${id}`, 'PUT', payload);
    else await api('/invoices', 'POST', payload);
    resetInvoiceForm();
    await loadAll();
  }, 'Rechnung gespeichert');
}

async function saveSettings(event) {
  event.preventDefault();
  const form = event.currentTarget;
  const data = formObject(form);
  $(form).find('input[type=checkbox][name]').each(function () {
    data[this.name] = this.checked ? 'true' : 'false';
  });
  await runAction(async () => {
    state.settings = await api('/settings', 'PUT', data);
    await loadAll();
  }, 'Einstellungen gespeichert');
}

async function uploadLogo(event) {
  event.preventDefault();
  const file = $('#logo-upload-file')[0].files[0];
  if (!file) {
    showMessage('Bitte Logo-Datei wählen', true);
    return;
  }
  const formData = new FormData();
  formData.append('file', file);
  await runAction(async () => {
    const result = await $.ajax({ url: '/api/settings/logo-upload', method: 'POST', data: formData, processData: false, contentType: false });
    state.settings.logo = result.logo;
    $('#settings-branding-form [name=logo]').val(result.logo);
    $('#logo-upload-file').val('');
    renderLogoPreview();
  }, 'Logo hochgeladen');
}

async function saveTaxDefinition(event) {
  event.preventDefault();
  const data = formObject('#tax-form');
  data.percent = toNumber(data.percent);
  data.isDefault = $('#tax-form [name=isDefault]').prop('checked');
  const id = data.id;
  delete data.id;
  await runAction(async () => {
    if (id) await api(`/tax-definitions/${id}`, 'PUT', data);
    else await api('/tax-definitions', 'POST', data);
    resetTaxForm();
    await loadAll();
  }, 'Steuerdefinition gespeichert');
}

async function saveCategory(event) {
  event.preventDefault();
  await saveProductOption('#category-form', '/product-categories', 'Kategorie gespeichert');
}

async function saveUnit(event) {
  event.preventDefault();
  await saveProductOption('#unit-form', '/product-units', 'Einheit gespeichert');
}

async function saveProductOption(selector, path, message) {
  const data = formObject(selector);
  data.sortOrder = parseInt(data.sortOrder || '0', 10) || 0;
  const id = data.id;
  delete data.id;
  await runAction(async () => {
    if (id) await api(`${path}/${id}`, 'PUT', data);
    else await api(path, 'POST', data);
    $(selector)[0].reset();
    await loadAll();
  }, message);
}

async function uploadTemplate(event) {
  event.preventDefault();
  const file = $('#template-upload-form [name=file]')[0].files[0];
  if (!file) {
    showMessage('Bitte ZIP Archiv wählen', true);
    return;
  }
  if (!file.name.toLowerCase().endsWith('.zip')) {
    showMessage('Datei muss ein ZIP Archiv sein', true);
    return;
  }
  const formData = new FormData();
  formData.append('file', file);
  await runAction(async () => {
    await $.ajax({ url: '/api/templates/upload', method: 'POST', data: formData, processData: false, contentType: false });
    $('#template-upload-form')[0].reset();
    await loadAll();
  }, 'Template hochgeladen');
}

async function installTemplate(event) {
  event.preventDefault();
  const data = formObject('#template-install-form');
  if (!data.url) {
    showMessage('Manifest URL fehlt', true);
    return;
  }
  await runAction(async () => {
    await api('/templates/install-from-manifest', 'POST', { url: data.url });
    $('#template-install-form')[0].reset();
    await loadAll();
  }, 'Template installiert');
}

async function handleTableAction(event) {
  event.preventDefault();
  const action = $(this).data('action');
  const id = $(this).data('id');
  try {
    if (action === 'edit-customer') fillForm('#customer-form', state.customers.find(x => x.id === id));
    if (action === 'delete-customer' && confirm('Kunde löschen?')) await runReload(() => api(`/customers/${id}`, 'DELETE'));
    if (action === 'edit-product') fillForm('#product-form', state.products.find(x => x.id === id));
    if (action === 'delete-product') await runReload(() => api(`/products/${id}`, 'DELETE'));
    if (action === 'activate-product') await runReload(() => api(`/products/${id}/activate`, 'POST', {}));
    if (action === 'edit-tax') fillForm('#tax-form', state.taxDefinitions.find(x => x.id === id));
    if (action === 'delete-tax' && confirm('Steuerdefinition löschen?')) await runReload(() => api(`/tax-definitions/${id}`, 'DELETE'));
    if (action === 'edit-category') fillForm('#category-form', state.productCategories.find(x => x.id === id));
    if (action === 'delete-category' && confirm('Kategorie löschen?')) await runReload(() => api(`/product-categories/${id}`, 'DELETE'));
	    if (action === 'edit-unit') fillForm('#unit-form', state.productUnits.find(x => x.id === id));
	    if (action === 'delete-unit' && confirm('Einheit löschen?')) await runReload(() => api(`/product-units/${id}`, 'DELETE'));
	    if (action === 'delete-template' && confirm('Template löschen?')) await runReload(() => api(`/templates/${id}`, 'DELETE'));
	    if (action === 'update-template') await runReload(() => api(`/templates/${id}/update`, 'POST', {}));
	    if (action === 'preview-template') await previewTemplate(id);
	    if (action === 'edit-invoice') await editInvoice(id);
    if (action === 'view-invoice') await viewInvoice(id);
    if (action === 'send-invoice') await runReload(() => api(`/invoices/${id}/send`, 'POST', {}));
    if (action === 'paid-invoice') await runReload(() => api(`/invoices/${id}/paid`, 'POST', { paymentMethod: prompt('Zahlungsart') || '' }));
    if (action === 'void-invoice' && confirm('Rechnung stornieren?')) await runReload(() => api(`/invoices/${id}/void`, 'POST', {}));
    if (action === 'duplicate-invoice') await runReload(() => api(`/invoices/${id}/duplicate`, 'POST', {}));
    if (action === 'delete-invoice' && confirm('Rechnung löschen?')) await runReload(() => api(`/invoices/${id}`, 'DELETE'));
    if (action === 'export-html') window.open(`/api/invoices/${id}/export?format=html`, '_blank');
    if (action === 'export-pdf') window.location = `/api/invoices/${id}/export?format=pdf`;
    if (action === 'export-xml') window.location = `/api/invoices/${id}/export?format=xml`;
    if (action === 'export-json') window.location = `/api/invoices/${id}/export?format=json`;
    if (action === 'export-csv') window.location = `/api/invoices/${id}/export?format=csv`;
  } catch (err) {
    showMessage(err.message, true);
  }
}

async function editInvoice(id) {
  const invoice = await api(`/invoices/${id}`);
  resetInvoiceForm();
  fillForm('#invoice-form', invoice);
  $('#invoice-items').empty();
  invoice.items.forEach(addItemRow);
  showView('invoices');
  updateInvoicePreview();
}

async function viewInvoice(id) {
  const invoice = await api(`/invoices/${id}`);
  resetInvoiceForm();
  fillForm('#invoice-form', invoice);
  $('#invoice-items').empty();
  invoice.items.forEach(addItemRow);
  const editable = canChangeInvoice(invoice);
  $('#invoice-form input, #invoice-form select, #invoice-form textarea').prop('disabled', !editable);
  $('#invoice-form button[type=submit]').prop('disabled', !editable);
  showView('invoices');
  updateInvoicePreview();
}

async function previewTemplate(id) {
  const html = await $.ajax({
    url: `/api/templates/${id}/preview`,
    method: 'POST',
    data: '{}',
    contentType: 'application/json',
    dataType: 'html'
  }).catch(xhr => {
    const msg = xhr.responseJSON && xhr.responseJSON.error ? xhr.responseJSON.error : xhr.statusText;
    throw new Error(msg || 'Preview fehlgeschlagen');
  });
  const win = window.open('', '_blank');
  if (win) {
    win.document.open();
    win.document.write(html);
    win.document.close();
  }
}

function collectInvoicePayload() {
  const data = formObject('#invoice-form');
  return {
    customerId: data.customerId,
    invoiceNumber: data.invoiceNumber,
    issueDate: data.issueDate,
    dueDate: data.dueDate,
    currency: data.currency,
    taxMode: data.taxMode,
    taxDefinitionId: data.taxDefinitionId,
    taxRate: toNumber(data.taxRate),
    discountPercentage: toNumber(data.discountPercentage),
    discountAmount: toNumber(data.discountAmount),
    pricesIncludeTax: $('#invoice-form [name=pricesIncludeTax]').prop('checked'),
    roundingMode: data.roundingMode,
    paymentTerms: data.paymentTerms,
    notes: data.notes,
    items: collectItemRows()
  };
}

function collectItemRows() {
  const items = [];
  $('#invoice-items tr').each(function () {
    const row = $(this);
    const description = row.find('.item-desc').val().trim();
    const quantity = toNumber(row.find('.item-qty').val());
    const unitPrice = toNumber(row.find('.item-price').val());
    if (!description && !quantity && !unitPrice) return;
    items.push({
      productId: row.find('.item-product').val(),
      description,
      quantity,
      unit: row.find('.item-unit').val().trim(),
      unitPrice,
      taxRate: toNumber(row.find('.item-tax-rate').val()),
      taxDefinitionId: row.find('.item-tax-definition').val()
    });
  });
  return items;
}

function addItemRow(item = {}) {
  const productOptions = ['<option value="">Frei</option>'].concat(
    state.products.filter(p => p.isActive).map(p => `<option value="${p.id}">${esc(p.name)}</option>`)
  ).join('');
  const row = $(`
    <tr>
      <td class="reorder"><button type="button" class="mini secondary move-item-up" title="Nach oben">↑</button><button type="button" class="mini secondary move-item-down" title="Nach unten">↓</button></td>
      <td><select class="item-product">${productOptions}</select></td>
      <td><input class="item-desc" value="${escAttr(item.description || '')}"></td>
      <td><input class="item-qty num" type="number" step="0.01" min="0" value="${numberInput(item.quantity || 1)}"></td>
      <td><input class="item-unit" value="${escAttr(item.unit || 'Stk')}"></td>
      <td><input class="item-price num" type="number" step="0.01" min="0" value="${numberInput(item.unitPrice || 0)}"></td>
      <td class="line-tax-col"><select class="item-tax-definition">${taxDefinitionOptions('Keine')}</select><input class="item-tax-rate num" type="number" step="0.01" min="0" value="${numberInput(item.taxRate || 0)}"></td>
      <td class="num item-sum">0,00</td>
      <td><button type="button" class="mini danger remove-item">Entfernen</button></td>
    </tr>
  `);
  row.find('.item-product').val(item.productId || '');
  row.find('.item-tax-definition').val(item.taxDefinitionId || '');
  $('#invoice-items').append(row);
  updateInvoicePreview();
}

function updateInvoicePreview() {
  const items = collectItemRows();
  const taxRate = toNumber($('#invoice-form [name=taxRate]').val());
  const taxMode = $('#invoice-form [name=taxMode]').val() || 'invoice';
  const discountPercentage = toNumber($('#invoice-form [name=discountPercentage]').val());
  const discountAmountInput = toNumber($('#invoice-form [name=discountAmount]').val());
  const pricesIncludeTax = $('#invoice-form [name=pricesIncludeTax]').prop('checked');
  const roundingMode = $('#invoice-form [name=roundingMode]').val() || 'line';
  const totals = calculateTotals(items, discountPercentage, discountAmountInput, taxRate, taxMode, pricesIncludeTax, roundingMode);
  $('.line-tax-col').toggle(taxMode === 'line');

  $('#invoice-items tr').each(function (index) {
    const item = items[index];
    $(this).find('.item-sum').text(money(item ? item.quantity * item.unitPrice : 0, $('#invoice-form [name=currency]').val() || 'EUR'));
  });
  $('#calc-subtotal').text(money(totals.subtotal, $('#invoice-form [name=currency]').val() || 'EUR'));
  $('#calc-discount').text(money(totals.discountAmount, $('#invoice-form [name=currency]').val() || 'EUR'));
  $('#calc-tax').text(money(totals.taxAmount, $('#invoice-form [name=currency]').val() || 'EUR'));
  $('#calc-total').text(money(totals.total, $('#invoice-form [name=currency]').val() || 'EUR'));
}

function calculateTotals(items, discountPercentage, discountAmount, taxRate, taxMode, pricesIncludeTax, roundingMode) {
  const r2 = n => Math.round(n * 100) / 100;
  const rate = Math.max(0, Number(taxRate) || 0) / 100;
  const lineGrosses = items.map(it => (Number(it.quantity) || 0) * (Number(it.unitPrice) || 0));
  const subtotal = lineGrosses.reduce((a, b) => a + b, 0);
  let finalDiscount = Number(discountAmount) || 0;
  if (discountPercentage > 0) finalDiscount = subtotal * (discountPercentage / 100);
  finalDiscount = Math.min(Math.max(finalDiscount, 0), subtotal);
  let taxAmount = 0;
  let total = 0;
  if (roundingMode === 'line' && subtotal > 0) {
    let distributed = 0;
    const lineDiscounts = lineGrosses.map((gross, index) => {
      if (index === lineGrosses.length - 1) return r2(finalDiscount - distributed);
      const discount = r2(finalDiscount * (gross / subtotal));
      distributed += discount;
      return discount;
    });
    lineGrosses.forEach((gross, index) => {
      const afterDiscount = Math.max(0, gross - lineDiscounts[index]);
      const lineRate = taxMode === 'line' ? Math.max(0, Number(items[index].taxRate) || 0) / 100 : rate;
      if (pricesIncludeTax) {
        const net = lineRate > 0 ? afterDiscount / (1 + lineRate) : afterDiscount;
        taxAmount += r2(afterDiscount - net);
        total += r2(afterDiscount);
      } else {
        const tax = r2(afterDiscount * lineRate);
        taxAmount += tax;
        total += r2(afterDiscount + tax);
      }
    });
  } else {
    const afterDiscount = subtotal - finalDiscount;
    if (taxMode === 'line' && subtotal > 0) {
      lineGrosses.forEach((gross, index) => {
        const share = gross / subtotal;
        const lineAfterDiscount = Math.max(0, afterDiscount * share);
        const lineRate = Math.max(0, Number(items[index].taxRate) || 0) / 100;
        if (pricesIncludeTax) {
          const net = lineRate > 0 ? lineAfterDiscount / (1 + lineRate) : lineAfterDiscount;
          taxAmount += lineAfterDiscount - net;
          total += lineAfterDiscount;
        } else {
          taxAmount += lineAfterDiscount * lineRate;
          total += lineAfterDiscount + lineAfterDiscount * lineRate;
        }
      });
      taxAmount = r2(taxAmount);
      total = r2(total);
    } else if (pricesIncludeTax) {
      const net = rate > 0 ? afterDiscount / (1 + rate) : afterDiscount;
      taxAmount = r2(afterDiscount - net);
      total = r2(afterDiscount);
    } else {
      taxAmount = r2(afterDiscount * rate);
      total = r2(afterDiscount + taxAmount);
    }
  }
  return { subtotal: r2(subtotal), discountAmount: r2(finalDiscount), taxAmount: r2(taxAmount), total: r2(total) };
}

function resetCustomerForm() {
  $('#customer-form')[0].reset();
  $('#customer-form [name=id]').val('');
}

function resetProductForm() {
  $('#product-form')[0].reset();
  $('#product-form [name=id]').val('');
  renderProductOptions();
  $('#product-form [name=unit]').val('Stk');
  $('#product-form [name=unitPrice]').val('0');
}

function resetInvoiceForm() {
  $('#invoice-form')[0].reset();
  $('#invoice-form input, #invoice-form select, #invoice-form textarea, #invoice-form button').prop('disabled', false);
  $('#invoice-form [name=id]').val('');
  $('#invoice-form [name=issueDate]').val(new Date().toISOString().slice(0, 10));
  $('#invoice-form [name=currency]').val(state.settings.currency || 'EUR');
  $('#invoice-form [name=taxRate]').val('0');
  $('#invoice-form [name=taxMode]').val('invoice');
  $('#invoice-form [name=taxDefinitionId]').val('');
  $('#invoice-form [name=paymentTerms]').val(state.settings.paymentTerms || '');
  $('#invoice-form [name=notes]').val(state.settings.defaultNotes || '');
  $('#invoice-form [name=roundingMode]').val('line');
  $('#invoice-form [name=pricesIncludeTax]').prop('checked', false);
  applyDefaultInvoiceTax();
  $('#invoice-items').empty();
  addItemRow();
}

function applyDefaultInvoiceTax() {
  const taxDef = state.taxDefinitions.find(t => t.isDefault);
  if (!taxDef) return;
  $('#invoice-form [name=taxDefinitionId]').val(taxDef.id);
  $('#invoice-form [name=taxRate]').val(numberInput(taxDef.percent));
}

function resetTaxForm() {
  $('#tax-form')[0].reset();
  $('#tax-form [name=id]').val('');
  $('#tax-form [name=percent]').val('0');
}

function fillForm(selector, data) {
  if (!data) return;
  const form = $(selector)[0];
  clearFormFields(form);
  Object.entries(data).forEach(([key, value]) => {
    const field = form.elements[key];
    if (!field) return;
    if (field.type === 'checkbox') field.checked = value === true || value === 'true' || value === 1 || value === '1';
    else field.value = value == null ? '' : value;
  });
}

function clearFormFields(form) {
  $(form).find('[name]').each(function () {
    if (this.type === 'checkbox' || this.type === 'radio') this.checked = false;
    else this.value = '';
  });
}

function formObject(selector) {
  const out = {};
  $(selector).serializeArray().forEach(x => out[x.name] = x.value);
  return out;
}

async function runReload(fn) {
  await runAction(async () => {
    await fn();
    await loadAll();
  }, 'Gespeichert');
}

async function runAction(fn, okText) {
  try {
    await fn();
    showMessage(okText, false);
  } catch (err) {
    showMessage(err.message, true);
  }
}

function showView(name) {
  $('.nav-button').removeClass('active');
  $(`.nav-button[data-view=${name}]`).addClass('active');
  $('.view').removeClass('active');
  $(`#view-${name}`).addClass('active');
}

function showSettingsSection(name) {
  $('.settings-tab').removeClass('active');
  $(`.settings-tab[data-settings-section=${name}]`).addClass('active');
  $('#settings-section-select').val(name);
  $('.settings-panel').removeClass('active');
  $(`.settings-panel[data-settings-panel=${name}]`).addClass('active');
}

function api(path, method = 'GET', data) {
  return $.ajax({
    url: `/api${path}`,
    method,
    data: data == null ? undefined : JSON.stringify(data),
    contentType: 'application/json',
    dataType: 'json'
  }).catch(xhr => {
    const msg = xhr.responseJSON && xhr.responseJSON.error ? xhr.responseJSON.error : xhr.statusText;
    throw new Error(msg || 'Fehler');
  });
}

function showMessage(text, error) {
  $('#message').text(text).toggleClass('error', !!error).prop('hidden', false);
  clearTimeout(showMessage.timer);
  showMessage.timer = setTimeout(() => $('#message').prop('hidden', true), 3500);
}

function metric(label, value) {
  return `<div class="metric-card"><span>${esc(label)}</span><strong>${esc(String(value))}</strong></div>`;
}

function statusBadge(status) {
  const labels = { draft: 'Entwurf', sent: 'Gesendet', overdue: 'Überfällig', paid: 'Bezahlt', voided: 'Storniert' };
  return `<span class="badge ${escAttr(status)}">${labels[status] || esc(status)}</span>`;
}

function money(value, currency) {
  try {
    return new Intl.NumberFormat('de-DE', { style: 'currency', currency: currency || 'EUR' }).format(Number(value) || 0);
  } catch {
    return `${Number(value || 0).toFixed(2)} ${currency || 'EUR'}`;
  }
}

function numberInput(value) {
  return String(Number(value || 0).toFixed(2));
}

function toNumber(value) {
  if (typeof value === 'number') return value;
  return Number(String(value || '').replace(',', '.')) || 0;
}

function emptyRow(cols) {
  return `<tr><td colspan="${cols}">Keine Daten</td></tr>`;
}

function esc(value) {
  return String(value == null ? '' : value)
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#39;');
}

function escAttr(value) {
  return esc(value);
}
