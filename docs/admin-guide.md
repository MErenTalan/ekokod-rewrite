# Yönetici kılavuzu — EKORM

Bu kılavuz şirket ve bina yöneticileri içindir: kullanıcılar, binalar, entegrasyonlar, tarifeler,
alarmlar ve raporlar. Kurulum, yedekleme ve sunucu işleri için [`runbook-operator.md`](runbook-operator.md)
belgesine bakın.

## 1. Roller

| rol | ne görür | ne yapabilir |
|---|---|---|
| **Yönetici** (platform) | tüm şirketler | şirket ekler/siler; "Bu şirket adına çalış" ile bir müşterinin ekranlarını onun gibi kullanır |
| **Şirket Yöneticisi** | şirketin tüm binaları | kullanıcı, bina, entegrasyon, tarife, alarm, rapor ve SMTP ayarlarını yönetir |
| **Şirket Salt Okunur Yöneticisi** | şirketin tüm binaları | yalnızca görüntüler ve dışa aktarır |
| **Bina Yöneticisi** | kendisine atanmış binalar | o binaların verisini, alarmlarını ve raporlarını yönetir |
| **Bina Salt Okunur Yöneticisi** | kendisine atanmış binalar | yalnızca görüntüler |
| **Demo Kullanıcı** | örnek (demo) şirket | salt okunur; profil ve şifre değiştirilemez |

Bir kullanıcı yetkisi olmayan bir sayfaya ya da kayda ulaşamaz. Menüde de yalnızca yetkili olduğu
bölümleri görür.

## 2. İlk kurulum sırası

1. **Ayarlar → Şirket:** şirket bilgilerini (sektör, alan, personel) doldurun. Sektör, Yönetim
   Arayüzü'ndeki sektörel karşılaştırmada kullanılır.
2. **Ayarlar → SMTP Ayarları:** şifre sıfırlama, alarm ve rapor e-postaları bu sunucudan gider.
   "Test e-postası gönder" ile deneyin.
3. **Ayarlar → Binalar:** her tesis için bir bina açın. **Fatura kesim günü** fatura dönemlerini
   belirler. Konum bilgisi haritada ve hava durumunda kullanılır.
4. **Ayarlar → Entegrasyonlar:** sağlayıcıyı (OSOS/EDAŞ, GridBox, PM5340, iSolarCloud) ve kullanıcı
   bilgilerini girin. Sonra **Doğrula**, ardından **Analizörleri keşfet**. Geçmiş için **Geçmiş
   veriyi çek** ile bir tarih aralığı seçin. Şifreler şifreli saklanır ve bir daha gösterilmez;
   boş bırakırsanız kayıtlı şifre korunur.
5. **Ayarlar → Analizörler:** keşfedilen analizörleri **Binaya ata**. Çarpan (akım trafosu oranı)
   doğru olmalıdır: tüm tüketim ve fatura hesapları bu çarpanla yapılır.
6. **Faturalar ve Tarifeler → Tarifeler:** binalara tarife atayın (tek tek ya da toplu). Tarife
   geçmişi binada görünür; tarihli bir değişiklik o tarihten sonraki faturaları etkiler.
7. **Ayarlar → Kullanıcılar:** kullanıcıları ekleyin ve rollerini seçin. Bina rolleri için binaları
   atayın.
8. **Güneş santrali** varsa: **Ayarlar → Güneş Santralleri**. Yıllık ya da 12 aylık hedef üretimi
   girin. İsterseniz mahsuplaşma analizörünü seçin ve iSolarCloud'a bağlayın.

## 3. Veri nasıl gelir

- Sayaç verisi her gece (varsayılan 03:00) sağlayıcılardan çekilir. Bir sağlayıcı ulaşılamazsa
  bir sonraki çalışmada eksik aralık tamamlanır.
- **Mesajlar** sekmesi arka plan işlerini (veri çekme, fatura, rapor) ve hatalarını listeler. Bir
  entegrasyon hata veriyorsa önce buraya bakın.
- **Tüketim** ekranındaki saatlik, günlük ve aylık rakamlar ardışık dönem sınırlarındaki endeks
  farkıyla hesaplanır. Saatlik okuyan sayaçlarda saatlik seri de doğrudur. **Faturalar** ise her
  zaman dönem sınırındaki gerçek okumalarla hesaplanır. İki rakam arasındaki fark ancak eksik
  okuma olduğunda görülür.

## 4. Faturalar

- Faturalar her sabah (varsayılan 05:00) kapanan dönemler için hesaplanır. Bir fatura satırında
  tüketim, dağıtım, vergiler ve reaktif ceza ayrı ayrı görünür.
- Eksik veri olan dönemler "eksik saat" bilgisiyle işaretlenir. Veri sonradan tamamlanırsa sistem
  yöneticiniz faturayı yeniden hesaplatabilir (`ekokod recompute bills`).

## 5. Alarmlar

**Alarmlar** sayfasında kural tipi seçilir: reaktif sınır, veri iletişimi, akım/gerilim/güç ya da
fatura artışı. Ardından analizörler ve alıcılar seçilir. Kurallar saatlik değerlendirilir. Tetiklenen
alarmlar e-posta ile gider ve alarm geçmişinde görünür. SMS kanalı görünür ama şu an etkin değildir.

## 6. Raporlar

**Raporlar** sayfasında aylık ve yıllık raporlar bina bazında hazırlanır. Otomatik gönderim için
alıcı ekleyin. Aylık raporlar ayın 2'sinde, yıllık raporlar 3 Ocak'ta üretilip gönderilir. Bir
raporu istediğiniz zaman elle de üretebilirsiniz; iş bittiğinde indirme bağlantısı çıkar.

## 7. Diğer modüller

- **Yük Profili:** hafta içi/hafta sonu ve tatil günlerine göre saatlik profil. Tatiller
  **Takvim**'den yönetilir.
- **Karbon Ayak İzi** ve **ISO 50001 Modülü:** emisyon kayıtları ve enerji yönetim sistemi
  dokümanları.
- **Finansal Analiz**, **Yenilenebilir Enerji**, **GES Santralleri:** üretim, mahsuplaşma ve
  yatırım göstergeleri.
- **Tüketim Tahmini** ve **Yapay Zekâ Analizi:** saatlik tahmin ve anomali kontrolü. Salt okunur
  roller sonuçları görür ama yeni tahmin çalıştıramaz.

## 8. Güvenlik

- Şifreler en az 10 karakter olmalı; büyük/küçük harf, rakam ve özel karakter içermelidir. Son kullanılan 5 şifre yeniden kullanılamaz.
- **Ayarlar → Hesap → Aktif oturumlar:** açık oturumlarınızı görün. Kaybolan bir cihazın oturumunu
  kapatın ya da **Tüm oturumları kapat** deyin.
- Üst üste hatalı girişlerde aynı adresten giriş bir süre engellenir (varsayılan 15 dakikada 5
  deneme).
- Silinen kayıtlar veritabanından hemen kaldırılmaz, silindi olarak işaretlenir. Her yönetim işlemi
  denetim kaydına yazılır.
