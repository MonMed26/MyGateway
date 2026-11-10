# MyGateway

Semi payment gateway untuk merchant **Shopee** dan **GoPay/GoBiz**: QRIS dinamis,
onboarding lewat OTP, polling transaksi otomatis, webhook bertanda tangan, dan
dashboard admin modern (Next.js + shadcn/ui) yang di-embed ke dalam satu binary Go.

> **Unofficial.** Proyek ini memakai alur web merchant yang di-reverse-engineer,
> bukan API resmi Shopee/Gojek. Tidak berafiliasi dengan Shopee, GoPay, Gojek,
> atau afiliasinya. Gunakan dengan risiko sendiri dan patuhi ketentuan layanan
> masing-masing penyedia.

---

## Fitur

- **Dashboard admin** (login bcrypt, session cookie) — statistik, provider, transaksi, detail transaksi.
- **Onboarding provider via OTP** — Shopee (password opsional) dan GoPay, langsung dari dashboard.
- **QRIS dinamis** — static QRIS EMVCo diubah per transaksi dengan nominal unik.
- **Unique code global** — rentang kode unik diatur sekali untuk semua provider.
- **Rekonsiliasi otomatis** — polling feed provider, pencocokan nominal, expiry, anti double-settle.
- **Webhook global** — HMAC-SHA256, retry exponential, log pengiriman, dan kirim ulang manual.
- **API klien** — satu Bearer token untuk semua request, SQLite sebagai penyimpanan.
- **Satu binary** — UI statis di-embed; tidak perlu menjalankan Node saat produksi.

---

## Persyaratan

- Go **1.26+**
- Node.js **20+** dan npm (hanya jika ingin rebuild dashboard)
- SQLite tidak perlu diinstal (driver pure-Go, database dibuat otomatis)

---

## Struktur repositori

```
cmd/gateway/        Server HTTP (API + dashboard + worker webhook)
internal/db/        Buka SQLite, migrasi, dan kompatibilitas skema lama
internal/repo/      Repository: admin, provider, payment, webhook, settings
internal/gateway/   Manajer provider: build instance, OTP, polling, event
internal/webhook/   Dispatcher webhook (HMAC, retry, resend)
internal/httpapi/   REST API admin + API klien
internal/webui/     Embed dashboard hasil static export
web/                Source Next.js + shadcn/ui
core/               Tipe domain, error, port
shopee/ gopay/      Adapter provider
payment/            Allocator nominal unik, matcher, service polling
qris/ utils/        EMVCo/QRIS, CRC16, logger, kripto
scripts/            build-all.ps1, dev.ps1
```

---

## Quick start

```bash
git clone https://github.com/MonMed26/MyGateway.git
cd MyGateway
go build -o gateway.exe ./cmd/gateway
./gateway.exe
```

Buka **http://localhost:8080** dan login dengan akun default:

```
username: admin
password: admin123
```

> Ganti password default sebelum mengekspos dashboard ke jaringan publik.

### Build ulang dashboard + binary

```powershell
.\scripts\build-all.ps1
```

### Mode development

```powershell
.\scripts\dev.ps1
```

Menjalankan Go di `:8080` dan Next.js dev server di `:3000` (proxy `/api` ke `:8080`).

---

## Konfigurasi

| Env | Default | Keterangan |
|---|---|---|
| `GATEWAY_ADDR` | `:8080` | Alamat listen HTTP |
| `GATEWAY_DB` | `./data/gateway.db` | Lokasi SQLite |
| `GATEWAY_MASTER_KEY` | — | Kunci enkripsi session provider (disarankan) |
| `GATEWAY_MASTER_KEY_FILE` | `./data/master.key` | Dipakai bila env master key kosong |
| `GATEWAY_ADMIN_USER` | `admin` | Username admin awal |
| `GATEWAY_ADMIN_PASS` | `admin123` | Password admin awal (hanya saat belum ada admin) |

Session provider dienkripsi AES-256-GCM dengan master key sebelum disimpan di SQLite.

---

## Alur kerja

1. **Pengaturan global** — isi API key, webhook URL/secret, rentang unique code, dan provider default.
2. **Tambah provider** — pilih Shopee/GoPay, lalu selesaikan login OTP.
3. **Pilih store/outlet** — jika akun punya lebih dari satu.
4. **Tempel static QRIS** — payload EMVCo dari dashboard merchant.
5. **Klien membuat payment** — gateway mengalokasikan nominal unik dan membuat QRIS dinamis.
6. **Polling** — transaksi masuk dicocokkan; saat lunas, webhook dikirim dan transaksi tampil di dashboard.

---

## Pengaturan global

Berlaku untuk semua provider (menu **Pengaturan**):

| Setting | Fungsi |
|---|---|
| API Token | Bearer token tunggal untuk klien memanggil `/api/v1/*` |
| Webhook URL | Target notifikasi `payment.paid` |
| Webhook Secret | Kunci HMAC-SHA256 untuk header `X-Gateway-Signature` |
| Unique code min/max | Rentang kode unik yang ditambahkan ke nominal dasar |
| Provider default | Dipakai saat request tidak menyertakan `provider_id` |

---

## Onboarding provider

### Shopee

1. **Providers → Tambah Provider** → pilih `shopee`.
2. Buka detail provider → **Koneksi Merchant**.
3. Masukkan nomor HP akun Shopee (`0812…` / `+62…`) dan password bila ada.
4. Klik **Kirim OTP**, lalu masukkan kode yang diterima.
5. Jika ada beberapa merchant, pilih salah satu → **Selesaikan Login**.
6. Pilih store bila perlu, lalu tempel **static QRIS** dan simpan.

### GoPay / GoBiz

1. Buat provider `gopay`.
2. Masukkan nomor HP, kirim OTP, lalu verifikasi.
3. Gateway otomatis mengambil merchant/outlet dan QRIS bila tersedia; jika tidak, pilih merchant/outlet secara manual.

Setelah terhubung, status akan menunjukkan **Session tersimpan · Authenticated · Polling aktif**.
Gunakan **Refresh Session** untuk memperpanjang tanpa OTP baru.

---

## API klien

Semua endpoint klien memakai satu Bearer token global:

```
Authorization: Bearer <API_TOKEN>
```

| Method | Path | Keterangan |
|---|---|---|
| `GET` | `/api/v1/ping` | Cek koneksi & status webhook |
| `POST` | `/api/v1/payments` | Buat payment |
| `GET` | `/api/v1/payments/{id}` | Status payment |
| `POST` | `/api/v1/payments/{id}/cancel` | Batalkan payment |

### Membuat payment

```bash
curl -X POST http://localhost:8080/api/v1/payments \
  -H "Authorization: Bearer <API_TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{
    "amount": 15000,
    "reference": "ORDER-1",
    "expires_in_seconds": 300
  }'
```

`provider_id` opsional. Bila kosong, gateway memakai **provider default**; jika tidak
diset dan hanya ada satu provider terhubung, provider itu yang dipakai.

```json
{
  "id": "pay_xxxxxxxxxxxx",
  "reference": "ORDER-1",
  "status": "pending",
  "base_amount": 15000,
  "unique_offset": 42,
  "unique_amount": 15042,
  "qr_string": "00020101021226610016ID.CO.SHOPEE...6304ABCD",
  "expires_at": "2026-11-10T13:16:11Z"
}
```

`unique_amount` adalah nominal yang harus dibayar pembeli (nominal dasar + kode unik).
Kode unik dialokasikan acak dalam rentang global dan tidak dipakai ulang selama
masih ada payment aktif atau masih dalam masa karantina.

---

## Webhook

Saat payment berstatus `paid`, gateway mengirim `POST` JSON ke **Webhook URL** global.

Header:

```
Content-Type: application/json
X-Gateway-Event: payment.paid
X-Gateway-Delivery: <delivery-id>
X-Gateway-Signature: sha256=<hmac-sha256-hex>
```

Body:

```json
{
  "event": "payment.paid",
  "created_at": "2026-11-10T13:10:00Z",
  "payment": {
    "id": "pay_xxxxxxxxxxxx",
    "config_id": "0b5c495b6181cf66",
    "reference": "ORDER-1",
    "base_amount": 15000,
    "unique_offset": 42,
    "unique_amount": 15042,
    "status": "paid",
    "qr_string": "0002010102122661...",
    "created_at": "2026-11-10T13:05:00Z",
    "expires_at": "2026-11-10T13:10:00Z",
    "paid_at": "2026-11-10T13:07:12Z",
    "transaction": { "...": "payload feed provider" },
    "metadata": { "order_id": "A-1" }
  }
}
```

Verifikasi (Node.js):

```js
const crypto = require("crypto");
const expected = "sha256=" + crypto
  .createHmac("sha256", WEBHOOK_SECRET)
  .update(rawBody)
  .digest("hex");
const valid = crypto.timingSafeEqual(Buffer.from(expected), Buffer.from(signature));
```

Perilaku:

- Retry otomatis hingga 6 kali dengan backoff exponential (15s → 30s → … cap 1 jam).
- Setiap percobaan tercatat di dashboard (HTTP status + respons).
- Bisa dikirim ulang manual dari halaman detail transaksi.
- Konsumen sebaiknya idempoten berdasarkan `payment.id` / `X-Gateway-Delivery`.

---

## Database

SQLite dengan WAL. Tabel utama:

| Tabel | Isi |
|---|---|
| `admin_users`, `admin_sessions` | Akun admin + session (token di-hash) |
| `provider_configs` | Provider, merchant/store/outlet, QRIS, session terenkripsi |
| `settings` | API token, webhook, unique code, provider default |
| `payments` | Payment intent, status, nominal, QRIS, transaksi |
| `webhook_deliveries` | Riwayat & jadwal pengiriman webhook |

Database lama dengan skema per-provider dimigrasikan otomatis saat server mulai.

---

## Keamanan

- Ganti kredensial admin default (`admin123`) sebelum produksi.
- Set `GATEWAY_MASTER_KEY` di produksi; jangan commit `data/`.
- API token & webhook secret saat ini tersimpan di SQLite (akses admin). Untuk
  produksi, pertimbangkan enkripsi at-rest tambahan.
- Session provider dienkripsi AES-256-GCM; jangan bagikan file database.
- Belum termasuk rate limiting, 2FA, dan audit log.
- Deploy satu instance (SQLite). Untuk multi-instance, gunakan database/queue bersama.
- Transaksi keuangan: uji menyeluruh sebelum dipakai menerima uang sungguhan.

---

## Lisensi

MIT. Lihat [LICENSE](LICENSE).
