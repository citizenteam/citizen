# Citizen RBAC Matrix

## 🎯 Rol Tanımları

| Rol | DB Değeri | `app_id` | Açıklama |
|-----|-----------|----------|----------|
| **Instance Admin** | `admin` | `__all__` | Tüm instance'a tam yetki (Org Owner otomatik alır) |
| **App Admin** | `admin` | `my-app` | Sadece belirli app'e admin yetkisi |
| **App Member** | `member` | `my-app` | Belirli app'e deploy + ayar yetkisi |
| **App Viewer** | `viewer` | `my-app` | Belirli app'i sadece izleyebilir (dashboard yok) |

## 📋 Permission Matrix

### 🔴 Instance-Level İşlemler (Sadece `__all__` + `admin`)

| İşlem | Admin | Member | Viewer | Not |
|-------|:-----:|:------:|:------:|-----|
| App oluşturma (`POST /apps`) | ✅ | ❌ | ❌ | |
| App silme (`DELETE /apps/:app_name`) | ✅ | ❌ | ❌ | |
| Docker connection yönetimi | ✅ | ❌ | ❌ | |
| GitHub config (manifest, connect-with-key) | ✅ | ❌ | ❌ | |
| GitHub instance disconnect | ✅ | ❌ | ❌ | |
| Failed login attempts görme | ✅ | ❌ | ❌ | |

### 🟡 App-Level Yazma İşlemleri (Member+)

| İşlem | Admin | Member | Viewer | Not |
|-------|:-----:|:------:|:------:|-----|
| Deploy (`POST /apps/:app_name/deploy`) | ✅ | ✅ | ❌ | |
| Restart (`POST /apps/:app_name/restart`) | ✅ | ✅ | ❌ | |
| Env değiştirme (`POST /apps/:app_name/env`) | ✅ | ✅ | ❌ | |
| Domain ekleme/silme | ✅ | ✅ | ❌ | |
| Build settings değiştirme | ✅ | ✅ | ❌ | |
| Buildpack yönetimi | ✅ | ✅ | ❌ | |
| Port değiştirme | ✅ | ✅ | ❌ | |
| Public app ayarı | ✅ | ✅ | ❌ | |
| API Access ayarı | ✅ | ✅ | ❌ | |
| GitHub repo bağlama | ✅ | ✅ | ❌ | |
| GitHub auto-deploy toggle | ✅ | ✅ | ❌ | |
| GitHub repo disconnect | ✅ | ✅ | ❌ | |
| Deployment update/status | ✅ | ✅ | ❌ | |

### 🟢 App-Level Okuma İşlemleri

| İşlem | Admin | Member | Viewer | Not |
|-------|:-----:|:------:|:------:|-----|
| App bilgisi (`GET /apps/:app_name`) | ✅ | ✅ | ✅ | RLS ile |
| Domain listesi | ✅ | ✅ | ✅ | RLS ile |
| Env görme (`GET /apps/:app_name/env`) | ✅ | ✅ | ❌ | Sensitive! |
| Loglar | ✅ | ✅ | ❌ | |
| Build settings görme | ✅ | ✅ | ❌ | |
| Deployment runs | ✅ | ✅ | ❌ | |
| Activities | ✅ | ✅ | ❌ | |

### ⚪ Genel İşlemler

| İşlem | Admin | Member | Viewer | Not |
|-------|:-----:|:------:|:------:|-----|
| Dashboard erişimi | ✅ | ✅ | ❌ | |
| App listesi (`GET /apps`) | ✅ | ✅ | ✅ | RLS ile (sadece atanan app'ler) |
| API Token oluşturma | ✅ | ✅ | ❌ | |
| Kendi audit logları | ✅ | ✅ | ❌ | |

## 🔒 RLS Akışı

```
CitizenAuth (Central)
       │
       │ Webhook: permission-update
       ▼
┌─────────────────────────────┐
│     app_permissions         │
│  ┌─────────┬────────┬────┐  │
│  │ user_id │ app_id │role│  │
│  ├─────────┼────────┼────┤  │
│  │ uuid-1  │ __all__│admin│  │ ← Instance Admin
│  │ uuid-2  │ my-app │member│ │ ← App Member
│  │ uuid-3  │ my-app │viewer│ │ ← App Viewer
│  └─────────┴────────┴────┘  │
└─────────────────────────────┘
       │
       │ RLS Policy
       ▼
   Filtered Results
```

## ✅ Final Onay

- [x] App silme → Sadece Admin (`__all__` + `admin`)
- [x] GitHub repo disconnect → Member da yapabilir
- [x] Deployment update/status → Member yapabilir
- [x] Env görme → Member+ (Viewer göremez - sensitive)
- [x] Loglar → Member+ (Viewer göremez)
- [x] App listesi → RLS ile herkes (atanan app'ler)
