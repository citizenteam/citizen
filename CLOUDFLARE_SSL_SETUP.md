# 🔐 Cloudflare DNS Challenge ile SSL Kurulumu

Port 80 kullanmadan Let's Encrypt SSL sertifikası almak için Cloudflare DNS Challenge kullanıyoruz.

## 📋 Adım 1: Cloudflare API Bilgilerini Alın

### Global API Key (Önerilen):

1. **Cloudflare Dashboard'a Giriş:**
   - https://dash.cloudflare.com/ adresine gidin
   - Giriş yapın

2. **API Token Sayfası:**
   - Sağ üst köşede profil ikonuna tıklayın
   - **"My Profile"** seçeneğine tıklayın
   - Sol menüden **"API Tokens"** seçeneğine tıklayın

3. **Global API Key:**
   - **"Global API Key"** başlığının yanındaki **"View"** butonuna tıklayın
   - Şifrenizi girin
   - API Key'i kopyalayın

4. **Email:**
   - Cloudflare hesabınızın email adresi

### Alternatif: API Token (Daha Güvenli):

Daha sınırlı yetkilerle bir token oluşturabilirsiniz:

1. **"Create Token"** butonuna tıklayın
2. **"Edit zone DNS"** şablonunu seçin
3. **Permissions:**
   - Zone - DNS - Edit
   - Zone - Zone - Read
4. **Zone Resources:**
   - Include - Specific zone - `selmangunes.com`
5. **Continue to summary** → **Create Token**
6. Token'ı kopyalayın

> ⚠️ **Not:** API Token kullanıyorsanız, `.env` dosyasında `CF_API_KEY` yerine `CF_DNS_API_TOKEN` kullanmanız gerekir.

## 📝 Adım 2: .env Dosyasını Güncelleyin

`.env` dosyasını düzenleyin:

```bash
nano /root/citizen/docker/.env
```

Son satırlardaki Cloudflare bilgilerini güncelleyin:

```bash
# ============================================
# CLOUDFLARE DNS CHALLENGE (for SSL without port 80)
# ============================================
CF_API_EMAIL=sizin@email.com
CF_API_KEY=sizin_global_api_key_buraya
```

veya API Token kullanıyorsanız:

```bash
CF_DNS_API_TOKEN=sizin_api_token_buraya
```

## 🚀 Adım 3: Servisleri Başlatın

```bash
cd /root/citizen/docker
docker-compose -f docker-compose.prod.yml down
docker-compose -f docker-compose.prod.yml up -d
```

## 🔍 Adım 4: Logları Kontrol Edin

SSL sertifikası oluşturulmasını izleyin:

```bash
docker logs -f citizen-traefik-prod
```

Başarılı olduğunda şu mesajları göreceksiniz:
```
time="..." level=info msg="Certificate obtained for domain citizen.selmangunes.com"
```

## 🌐 Adım 5: Cloudflare DNS Ayarları

1. **A Kaydı:**
   - Type: `A`
   - Name: `citizen` (veya `@` root domain için)
   - IPv4 address: `SUNUCU_IP_ADRESINIZ`
   - Proxy status: **🟠 Proxied (Turuncu Bulut)** - Aktif
   - TTL: Auto

2. **Cloudflare SSL/TLS Ayarları:**
   - Dashboard → SSL/TLS → Overview
   - Encryption mode: **Full (strict)** (önerilen)
   - veya **Flexible** (eğer backend'de SSL yoksa)

3. **Origin Rules (Port Yönlendirme):**
   - Dashboard → Rules → Origin Rules
   - **Create Rule**
   - Rule Name: `Citizen Port Override`
   - **When incoming requests match:**
     - Field: Hostname
     - Operator: equals  
     - Value: `citizen.selmangunes.com`
   - **Then:**
     - Destination Port: Override to `8080`
   - Save

## 🎯 Adım 6: Test Edin

1. **HTTP testi:**
   ```bash
   curl http://citizen.selmangunes.com:8080
   ```

2. **HTTPS testi:**
   ```bash
   curl https://citizen.selmangunes.com:8443
   ```

3. **Cloudflare üzerinden (Origin Rules ile):**
   ```bash
   curl https://citizen.selmangunes.com
   ```

## 🔧 Port Bilgileri

- **8080:** HTTP (internal: 80)
- **8443:** HTTPS (internal: 443)
- **8081:** Traefik Dashboard

## ⚠️ Sorun Giderme

### "Invalid API credentials" hatası:
- CF_API_EMAIL ve CF_API_KEY'i kontrol edin
- Global API Key'i doğru kopyaladığınızdan emin olun

### "DNS propagation failed" hatası:
- DNS kayıtlarının yayılması için 2-5 dakika bekleyin
- `delayBeforeCheck: 30` değerini artırın (traefik.yml)

### Sertifika almak çok uzun sürüyorsa:
```bash
# Logları kontrol edin
docker logs citizen-traefik-prod 2>&1 | grep -i "acme\|certificate\|error"

# acme.json'ı sıfırlayın
rm /root/citizen/docker/data/letsencrypt/acme.json
touch /root/citizen/docker/data/letsencrypt/acme.json
chmod 600 /root/citizen/docker/data/letsencrypt/acme.json
docker-compose -f docker-compose.prod.yml restart traefik
```

## 📚 Alternatif: API Token Kullanımı

API Token kullanmak istiyorsanız, `docker-compose.prod.yml` dosyasında değişiklik yapın:

```yaml
environment:
  - CF_DNS_API_TOKEN=${CF_DNS_API_TOKEN}
```

ve `.env` dosyasında:

```bash
CF_DNS_API_TOKEN=sizin_api_token_buraya
```

## ✅ Başarı Kriterleri

- ✅ `docker logs citizen-traefik-prod` sertifika mesajını gösteriyor
- ✅ `https://citizen.selmangunes.com` yeşil kilit ile açılıyor
- ✅ Browser'da SSL sertifika bilgileri "Let's Encrypt" gösteriyor
- ✅ Hiçbir port 80 hatası yok

---

**Not:** DNS Challenge sayesinde port 80'e hiç ihtiyaç duymadan SSL sertifikası alabilirsiniz!

