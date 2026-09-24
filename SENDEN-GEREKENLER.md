# Senden gerekenler

Kodlama işi bitti: F0–F15 fazlarının hepsi yazıldı ve test edildi. Aşağıdakiler bu makinede
yapılamıyor ya da senin kararını istiyor. Hiçbir şey push edilmedi, `main`'e dokunulmadı.

## 1. Karar vermen gerekenler
1. **F3 Q6 düzeltmesini onayla.** Saatlik tüketim grafiği saatlik sayaçlarda sıfır çıkıyordu;
   düzelttim (R470–R474). Artık grafik ile fatura, dönem sınırındaki okumalar eksiksizse aynı
   rakamı veriyor. Onaylıyor musun?
2. **Plan sorularına bak.** Hepsine önerilen seçenekle devam ettim. Önce şunlara bak:
   - Q-J7: tarife sınıflandırma cevapları
   - Q-J9: ISO işinin kullanıcıdan binaya taşınması
   - Q-J12: santral aylık toplamlarının bırakılması
   - Q-K5: mutabakata eklenen dört neden
3. **Yönetici kılavuzu Türkçe yazıldı** (`docs/admin-guide.md`). Uygun mu?
4. **Yedekler şifrelenmiyor.** Müşteri şifreli yedek isterse `backup.sh`'e `age`/`gpg` eklenir.

## 2. Vermen gereken şeyler
5. **Anonimleştirilmiş üretim veri dökümü** (MongoDB) ve bir deneme sunucusu. Bunlarla şunları
   yapacağım: gerçek envanter, üç prova göçü (süreleriyle) ve geri dönüş denemesi.
6. **Gerçek fatura örnekleri.** Motorun hesabını gerçek faturayla satır satır karşılaştırmak için
   gerekli (F4, hâlâ açık).
7. **Canlı sağlayıcı bilgileri** (OSOS/EDAŞ, GridBox, PM5340, iSolarCloud, EPİAŞ, SMTP). Taşınan
   kimlik bilgilerinin gerçekten bağlandığını görmek için.
8. **Gerçek üretim hacmi** (sayaç sayısı, okuma sıklığı). Yük testi varsayımla yapıldı: 10
   kullanıcı, p95 500 ms.

## 3. Senin yapman gerekenler
9. **Push / merge.** Dallar `phase/f1-data-model` … `phase/f15-hardening` sırayla birbirinin
   üstünde. Son dal: `phase/f15-hardening` (`/home/personal/ekokod-f15-phase`). Push ve `main`'e
   merge senin kararın.
10. **Ayrı bir sunucuda geri yükleme testi.** Yedeğin temiz bir konteynere geri yüklendiği
    kanıtlandı. Gerçek ikinci bir makinede `docs/runbook-operator.md` §4'ü bir kez uygula.
11. **İnternetsiz kurulum testi.** Paket, `--pull never` ile kurulup yükseltildi. Ağı gerçekten
    kapalı bir makinede `install.sh`'i bir kez çalıştır.
12. **24 saatlik dayanıklılık testi** (`make soak-test`), buna dayanabilecek bir makinede.
13. **Ekran okuyucu kontrolü** (NVDA ve VoiceOver). Giriş, yönetim arayüzü, tüketim, faturalar ve
    ayarlar ekranlarında bir kişinin denemesi gerekiyor.
14. **ML bağımlılık denetimi.** İnternete bağlı bir makinede `pip-audit` ile `ml/uv.lock` dosyasını
    tara.
15. **WSL ayarı (isteğe bağlı).** `C:\Users\meren\.wslconfig` bellek sınırı ancak `wsl --shutdown`
    sonrası devreye girer.
