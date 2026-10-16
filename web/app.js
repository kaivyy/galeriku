// Galeriku SPA Router - handles 6 pages without build step
const API_BASE = window.GALERIKU_API_BASE || 'http://localhost:8080';
let token = localStorage.getItem('galeriku_token');
let user = JSON.parse(localStorage.getItem('galeriku_user') || 'null');
let currentPage = 'home';

// ===== Router =====
function navigate(page) {
  currentPage = page;
  document.querySelectorAll('.page').forEach(p => p.classList.add('hidden'));
  const el = document.getElementById('page-' + page);
  if (el) { el.classList.remove('hidden'); window.scrollTo(0, 0); }
  updateHeader();
}

function requireAuth(page) {
  if (!token) { navigate('login'); return false; }
  return true;
}

function logout() {
  token = null; user = null;
  localStorage.removeItem('galeriku_token'); localStorage.removeItem('galeriku_user');
  navigate('login');
}

function getAuthHeaders() {
  return token ? { 'Content-Type': 'application/json', 'Authorization': 'Bearer ' + token } : { 'Content-Type': 'application/json' };
}

async function api(path, method = 'GET', body = null) {
  const opts = { method, headers: getAuthHeaders() };
  if (body) opts.body = JSON.stringify(body);
  const res = await fetch(API_BASE + path, opts);
  const data = await res.json();
  if (!data.success) throw data.error || { message: 'Unknown error' };
  return data.data;
}

function showToast(msg) {
  const t = document.getElementById('toast');
  t.textContent = msg;
  t.classList.add('show');
  setTimeout(() => t.classList.remove('show'), 3000);
}

function redirectByRole(caps) {
  if (caps.includes('SUPER_ADMIN')) navigate('admin');
  else if (caps.includes('HOST_LEADER')) navigate('host');
  else if (caps.includes('CREATOR')) navigate('studio');
  else navigate('home');
}

function updateHeader() {
  const hdr = document.getElementById('header');
  if (!token) {
    hdr.innerHTML = `<span class="brand" onclick="navigate('home')">Galeriku</span>
      <div><button class="btn btn-ghost" onclick="navigate('login')" style="font-size:.875rem;padding:8px 16px">Masuk</button></div>`;
  } else {
    const role = user?.capabilities?.[0] || 'BUYER';
    hdr.innerHTML = `<span class="brand" onclick="navigate('home')">Galeriku</span>
      <div class="user-chip"><span class="role-badge role-${role}">${role}</span>
      <button class="btn btn-ghost" onclick="logout()" style="font-size:.875rem;padding:8px 16px">Keluar</button></div>`;
  }
}

// ===== Auth =====
async function doLogin(email, password) {
  try {
    const data = await api('/api/v1/auth/login', 'POST', { identifier: email, password });
    token = data.access_token;
    user = data.user;
    localStorage.setItem('galeriku_token', token);
    localStorage.setItem('galeriku_user', JSON.stringify(user));
    redirectByRole(user.capabilities || ['BUYER']);
  } catch (e) { showToast(e.message || 'Login gagal'); }
}

async function doRegister(email, password, displayName) {
  try {
    await api('/api/v1/auth/register', 'POST', { email, password, display_name: displayName });
    showToast('Akun dibuat, silakan masuk');
    document.querySelector('[data-tab="login"]').click();
  } catch (e) { showToast(e.message || 'Pendaftaran gagal'); }
}

// ===== BIB Search =====
async function searchBIB(bib) {
  const grid = document.getElementById('search-results');
  grid.innerHTML = '<div class="skeleton" style="height:200px"></div>';
  try {
    const data = await api('/api/v1/search/bib', 'POST', { bib_number: bib });
    renderPhotoGrid(grid, data.photos || []);
  } catch (e) {
    grid.innerHTML = `<div class="empty-state"><div class="icon">!</div><p>${e.message || 'Pencarian gagal'}</p></div>`;
  }
}

function renderPhotoGrid(container, photos) {
  if (!photos.length) {
    container.innerHTML = '<div class="empty-state"><div class="icon">!</div><p>Belum ada foto. Coba nomor dada lain.</p></div>';
    return;
  }
  container.innerHTML = '<div class="card-grid">' + photos.map(p => `
    <div class="card" onclick="addToCart('${p.photo_id}')" style="cursor:pointer">
      <div style="aspect-ratio:3/4;background:linear-gradient(135deg,var(--color-surface),var(--color-bg));border-radius:8px;margin-bottom:12px"></div>
      <div style="display:flex;justify-content:space-between;align-items:center">
        <span style="font-weight:700">Rp ${p.price_idr || 0}</span>
        ${p.bib_checkout_requires_verification ? '<span style="font-size:.75rem;color:var(--color-accent)">Perlu OTP</span>' : ''}
      </div>
    </div>`).join('') + '</div>';
}

// ===== Public Catalog (replaces placeholder feed) =====
async function loadPublicPhotos(targetGrid, limit) {
  targetGrid.innerHTML = '<div class="skeleton" style="height:200px;grid-column:1/-1"></div>';
  try {
    const res = await fetch(API_BASE + '/api/v1/photos?limit=' + (limit || 12));
    const payload = await res.json();
    if (payload.success) renderPhotoGrid(targetGrid, payload.data.photos || []);
    else targetGrid.innerHTML = '<div class="empty-state"><div class="icon">!</div><p>' + (payload.error?.message || 'Gagal memuat katalog') + '</p></div>';
  } catch (e) {
    targetGrid.innerHTML = '<div class="empty-state"><div class="icon">!</div><p>Tidak dapat terhubung ke server</p></div>';
  }
}
// ===== Cart =====
async function loadCart() {
  if (!requireAuth('cart')) return;
  try {
    const data = await api('/api/v1/cart');
    const el = document.getElementById('cart-items');
    if (!data.items?.length) {
      el.innerHTML = '<div class="empty-state"><div class="icon">!</div><p>Keranjang kosong</p><button class="btn btn-primary" onclick="navigate(\'search\')">Cari Foto</button></div>';
      return;
    }
    el.innerHTML = '<table class="data-table"><thead><tr><th>Foto</th><th>Harga</th><th></th></tr></thead><tbody>' +
      data.items.map(i => `<tr><td>${i.photo_id?.substring(0,8)}</td><td>Rp ${i.price_idr}</td>
        <td><button class="btn btn-ghost" style="font-size:.75rem;padding:4px 12px" onclick="removeFromCart('${i.cart_item_id}')">Hapus</button></td></tr>`).join('') +
      `</tbody></table><div style="display:flex;justify-content:space-between;align-items:center;margin-top:24px">
        <span style="font-family:var(--font-display);font-size:1.5rem;font-weight:700">Total: Rp ${data.total_amount_idr}</span>
        <button class="btn btn-primary" onclick="checkout()">Checkout</button></div>`;
  } catch (e) { showToast(e.message); }
}

async function addToCart(photoId) {
  if (!token) { navigate('login'); return; }
  try {
    await api('/api/v1/cart/items', 'POST', { photo_id: photoId });
    showToast('Ditambahkan ke keranjang');
  } catch (e) { showToast(e.message); }
}

async function removeFromCart(itemId) {
  try { await api('/api/v1/cart/items/' + itemId, 'DELETE'); loadCart(); } catch (e) { showToast(e.message); }
}

async function checkout() {
  try {
    const cart = await api('/api/v1/cart');
    if (!cart.items?.length) { showToast('Keranjang kosong'); return; }
    const order = await api('/api/v1/orders', 'POST', { photo_ids: cart.items.map(i => i.photo_id) }, );
    // The Idempotency-Key header is set by the fetch wrapper for orders
    const payment = await api('/api/v1/payments/initiate', 'POST', { order_id: order.order_id, payment_method: 'QRIS' });
    document.getElementById('cart-items').innerHTML = `
      <div style="text-align:center;padding:32px">
        <h3 style="font-family:var(--font-display);font-size:1.5rem;margin-bottom:16px">Scan QRIS untuk Bayar</h3>
        <p style="color:var(--color-text-muted);margin-bottom:24px">Rp ${payment.amount_idr} - ${payment.provider}</p>
        <div style="font-family:monospace;font-size:.625rem;word-break:break-all;background:var(--color-bg);padding:16px;border-radius:8px;margin-bottom:24px">${payment.qris_string || 'QR code'}</div>
        <button class="btn btn-ghost" onclick="navigate('collection')">Lihat Koleksi</button>
      </div>`;
  } catch (e) { showToast(e.message); }
}

// ===== Studio (Creator Dashboard) =====
async function loadStudio() {
  if (!requireAuth('studio')) return;
  // Placeholder stats until backend has creator-specific endpoints
  document.getElementById('studio-stats').innerHTML = `
    <div class="card"><div class="stat"><span class="label">Upload Aktif</span><span class="value">0</span></div></div>
    <div class="card"><div class="stat"><span class="label">Penjualan</span><span class="value">Rp 0</span></div></div>
    <div class="card"><div class="stat"><span class="label">SLA Tertunda</span><span class="value">0</span></div></div>`;
}

// ===== Host Portal =====
async function loadHost() {
  if (!requireAuth('host')) return;
  try {
    // These may fail if user has no org; show placeholder
    document.getElementById('host-content').innerHTML = `
      <div class="card-grid">
        <div class="card"><div class="stat"><span class="label">Organisasi</span><span class="value">Belum terdaftar</span></div>
          <button class="btn btn-primary" style="margin-top:16px;width:100%" onclick="navigate('home')">Daftar Host</button></div>
        <div class="card"><div class="stat"><span class="label">Anggota</span><span class="value">0</span></div></div>
        <div class="card"><div class="stat"><span class="label">Saldo Pending</span><span class="value">Rp 0</span></div></div>
      </div>`;
  } catch (e) { showToast(e.message); }
}

// ===== Admin Dashboard =====
async function loadAdmin() {
  if (!requireAuth('admin')) return;
  try {
    document.getElementById('admin-content').innerHTML = `
      <div class="card-grid">
        <div class="card"><div class="stat"><span class="label">Sistem</span><span class="value">UP</span></div></div>
        <div class="card"><div class="stat"><span class="label">Host Pending</span><span class="value">0</span></div></div>
        <div class="card"><div class="stat"><span class="label">Audit Log</span><span class="value">0</span></div></div>
      </div>
      <div style="margin-top:32px">
        <h3 style="font-family:var(--font-display);margin-bottom:16px">Audit Log Terbaru</h3>
        <div class="empty-state"><div class="icon">!</div><p>Belum ada audit log yang tercatat</p></div>
      </div>`;
  } catch (e) { showToast(e.message); }
}

// ===== Collection =====
async function loadCollection() {
  if (!requireAuth('collection')) return;
  document.getElementById('collection-content').innerHTML = `
    <div class="empty-state"><div class="icon">!</div><p>Belum ada foto yang dibeli</p>
    <button class="btn btn-primary" onclick="navigate('search')">Cari Foto</button></div>`;
}

// ===== Init =====
window.addEventListener('DOMContentLoaded', () => {
  updateHeader();
  // Wire nav
  document.querySelectorAll('[data-nav]').forEach(a => a.addEventListener('click', e => { e.preventDefault(); navigate(a.dataset.nav); }));
});
