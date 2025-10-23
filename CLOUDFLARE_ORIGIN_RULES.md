# 🌐 Cloudflare Origin Rules Kurulumu - Port 8089/8443

Citizen uygulamanız şu portlarda çalışıyor:
- **HTTP:** Port 8089
- **HTTPS:** Port 8443

Cloudflare Origin Rules ile kullanıcılar normal URL'lerle erişebilir.

---

## 📋 Origin Rules Ayarları

### **Kural 1: HTTP Trafiği (Port 8089)**

1. Cloudflare Dashboard → **citizen.selmangunes.com** domain'ini seçin
2. Sol menüden **Rules** → **Origin Rules**
3. **Create Rule** butonuna tıklayın

**Ayarlar:**
```
Rule name: Citizen HTTP to 8089

Expression Preview veya Custom filter:
┌─────────────────────────────────────────────────┐
│ When incoming requests match:                   │
│                                                  │
│ Field:     Hostname                             │
│ Operator:  equals                               │
│ Value:     citizen.selmangunes.com              │
│                                                  │
│ AND                                             │
│                                                  │
│ Field:     SSL/HTTPS                            │
│ Operator:  is off                               │
└─────────────────────────────────────────────────┘

Then:
┌─────────────────────────────────────────────────┐
│ Destination Port: 8089                          │
└─────────────────────────────────────────────────┘
```

**Alternatif Expression (eğer dropdown'da yoksa):**
```
(http.host eq "citizen.selmangunes.com" and not ssl)
```

4. **Deploy** butonuna tıklayın

---

### **Kural 2: HTTPS Trafiği (Port 8443)**

**Create Rule** ile ikinci bir kural oluşturun:

**Ayarlar:**
```
Rule name: Citizen HTTPS to 8443

Expression:
┌─────────────────────────────────────────────────┐
│ When incoming requests match:                   │
│                                                  │
│ Field:     Hostname                             │
│ Operator:  equals                               │
│ Value:     citizen.selmangunes.com              │
│                                                  │
│ AND                                             │
│                                                  │
│ Field:     SSL/HTTPS                            │
│ Operator:  is on                                │
└─────────────────────────────────────────────────┘

Then:
┌─────────────────────────────────────────────────┐
│ Destination Port: 8443                          │
└─────────────────────────────────────────────────┘
```

**Alternatif Expression:**
```
(http.host eq "citizen.selmangunes.com" and ssl)
```

4. **Deploy** butonuna tıklayın

---

## ✅ DNS Ayarları

Cloudflare DNS'de A kaydınızı kontrol edin:

```
Type:       A
Name:       citizen
Content:    159.203.181.183 (sunucu IP'niz)
Proxy:      🟠 Proxied (Turuncu bulut - AKTİF)
TTL:        Auto
```

> ⚠️ **Önemli:** Proxy **mutlaka aktif** olmalı (turuncu bulut)

---

## 🔐 SSL/TLS Ayarları

1. Dashboard → **SSL/TLS** → **Overview**
2. **Encryption mode:** **Full (strict)** seçin
   - Eğer "Full (strict)" çalışmazsa → **Full** deneyin
3. **SSL/TLS** → **Edge Certificates**
   - ✅ **Always Use HTTPS:** ON
   - ✅ **Automatic HTTPS Rewrites:** ON

---

## 🧪 Test Adımları

### **1. Sunucu Tarafı Test:**

```bash
# Portların dinlendiğini kontrol edin
ss -tlnp | grep -E ':(8089|8443)'

# Container durumunu kontrol edin
docker ps | grep citizen

# Traefik loglarını izleyin
docker logs -f citizen-traefik-prod
```

### **2. HTTP Testi (Port 8089):**

```bash
# Direkt port ile test
curl -I http://citizen.selmangunes.com:8089

# Cloudflare üzerinden (Origin Rules ile)
curl -I http://citizen.selmangunes.com
```

### **3. HTTPS Testi (Port 8443):**

```bash
# Direkt port ile test
curl -I https://citizen.selmangunes.com:8443

# Cloudflare üzerinden (Origin Rules ile)
curl -I https://citizen.selmangunes.com
```

### **4. Browser Test:**

```
https://citizen.selmangunes.com
```

✅ Yeşil kilit simgesi görmelisiniz  
✅ Port numarası URL'de olmamalı  
✅ Sayfa yüklenmeli  

---

## 🔍 Sorun Giderme

### **521 Error (Web server is down):**
```bash
# Container'ların çalıştığını kontrol edin
docker ps

# Firewall kurallarını kontrol edin
sudo ufw status
sudo ufw allow 8089/tcp
sudo ufw allow 8443/tcp
```

### **525 Error (SSL Handshake Failed):**
- SSL/TLS ayarını **Full (strict)** yerine **Full** yapın
- API Token'ın doğru olduğundan emin olun
- SSL sertifikası oluşmasını bekleyin (2-5 dakika)

### **Origin Rules Çalışmıyor:**
```bash
# Cloudflare cache'i temizleyin
Dashboard → Caching → Configuration → Purge Everything

# DNS yayılımını kontrol edin
dig citizen.selmangunes.com
nslookup citizen.selmangunes.com
```

### **SSL Sertifikası Alamıyorum:**

API Token'ın doğru izinlere sahip olduğundan emin olun:

1. https://dash.cloudflare.com/profile/api-tokens
2. **Create Custom Token**
3. **Permissions:**
   - ✅ Zone | DNS | Edit
   - ✅ Zone | Zone | Read
4. **Zone Resources:**
   - Include | Specific zone | selmangunes.com

`.env` dosyasını güncelleyin:
```bash
nano /root/citizen/docker/.env
# Son satır:
CF_DNS_API_TOKEN=yeni_token_buraya
```

Traefik'i yeniden başlatın:
```bash
docker-compose -f docker-compose.prod.yml restart traefik
docker logs -f citizen-traefik-prod
```

---

## 📊 Origin Rules Özeti

| Trafik | Port | Origin Rule Expression |
|--------|------|------------------------|
| HTTP   | 8089 | `(http.host eq "citizen.selmangunes.com" and not ssl)` |
| HTTPS  | 8443 | `(http.host eq "citizen.selmangunes.com" and ssl)` |

---

## ✨ Sonuç

✅ Kullanıcılar: `https://citizen.selmangunes.com` (normal URL)  
✅ Cloudflare: Origin Rules ile port yönlendirme  
✅ Sunucu: Port 8089 (HTTP) ve 8443 (HTTPS) dinliyor  
✅ SSL: Let's Encrypt (DNS Challenge)  
✅ Güvenlik: Cloudflare DDoS, WAF, Cache  

---

**Not:** Origin Rules kurulduktan sonra, URL'de port numarası belirtmenize gerek kalmaz!

