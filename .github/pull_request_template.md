## 📌 Summary / Overview
<!-- Berikan ringkasan singkat perubahan dan tujuan teknis dari PR ini -->

## 🎯 Milestone / Ticket
- Ref Docs: `docs/10-implementation-plan.md` - **Week X (Days Y-Z)**
- Related Issue/Discussion: #

## 🧱 Seams & Architectural Impact
- **Seam Under Test**: <!-- Tentukan boundary yang diuji (misal: HTTP Handler, Queue Consumer, DB Repository) -->
  - *Catches*: <!-- Apa yang berhasil dicegah/ditangkap oleh test di seam ini -->
  - *Misses*: <!-- Batasan luar yang tidak dicakup oleh seam ini -->
- **Database Migrations**: [ ] Yes / [ ] No (Version: `00000X_...`)
- **Breaking Changes / Contract Update**: [ ] Yes / [ ] No

## 🧪 Testing Matrix & Verification
Sesuai standar TDD (*Red-Green Loop*), verifikasi berikut telah dijalankan:

- [ ] **Unit Tests (`make test-unit`)**: Menguji isolated business logic & edge cases.
- [ ] **Component Tests (`make test-component`)**: Menguji HTTP handler, DB persistence, & Redis.
- [ ] **Coverage Gate (`make test-coverage`)**: Memenuhi ambang batas minimal coverage (>= 70%).
- [ ] **Linter & Formatting (`make lint`)**: Lolos `go vet`, `gofmt`, dan TypeScript typecheck.

### 📸 Test & Verification Evidence
```bash
# Tempelkan bukti output ringkas 'make test' atau screenshot jika ada
```

## 🔍 Observability & Traceability Checklist
- [ ] Log menggunakan structured logger (`log/slog`) dengan `trace_id` & `request_id`.
- [ ] **Security**: Tidak ada password, Better Auth token/cookie, atau raw API key (`sp_live_...`) yang tercatat di log.
- [ ] Error response mengikuti format standar: `{"success": false, "error": {...}}`.

## 🚀 Deployment / Migration Notes (Ops)
<!-- Catatan khusus jika ada env var baru di .env.example atau langkah migrasi -->

## 📌 Branch & PR Conventions (Wajib)
- Branch name: `feat/<issue-num>-<slug>` / `fix/<issue-num>-<slug>` / `docs/<issue-num>-<slug>` / `ci/<issue-num>-<slug>`.
- Satu PR = satu issue. Hindari PR yang menyentuh banyak issue sekaligus.
- PR title mengikuti semantic commit: `feat(scope): ...`, `fix(scope): ...`, `docs(scope): ...`, `ci(scope): ...`.
- PR body WAJIB mereferensikan issue via `Resolves #<issue_number>`.
- Lihat `docs/MULTI_AGENT_STANDARDS.md` (section Branch Naming & PR Conventions) untuk detail.
