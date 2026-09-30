# Yapay Zekâ Analizi ve Tüketim Tahmini nasıl çalışır?

Bu not, uygulamadaki iki ekranı basitçe anlatır. Ekranlar **Tüketim Tahmini** ve **Yapay Zekâ Analizi**'dir.

## Kısaca

- Sistem bir sayacın **geçmiş tüketimine bakar**, oradan **tekrar eden düzeni öğrenir** ve bu düzeni ileriye taşır.
- Hiçbir şey sihir değildir. "Pazartesi sabah 9'da genelde ne kadar harcıyoruz?" sorusunun cevabını, geçmiş verilere bakarak bulur.
- Tahmin tek bir sayı olarak gelmez. Bir **en olası değer** ve bir **aralık** gelir: "büyük ihtimalle 120 kWh, ama 100 ile 140 arasında olabilir."

## 1. Tüketim tahmini

### Neye bakıyor?

- **Saatlik tahminde** sayacın **son 8 haftalık** saatlik tüketimine bakar. Bu, "Tüketim Tahmini" ekranı ile günlük ve haftalık sekmeler için geçerlidir.
- **Aylık tahminde** sayacın **son 180 günlük** günlük tüketimine bakar.
- Bunlara ek olarak şirketin **takvimine** de bakar:
  - Hangi günler hafta sonu (şirkette tanımlıysa ona göre, değilse Cumartesi ve Pazar)?
  - Hangi günler tatil (şirket takviminde girilen tatiller)?
- Model ayrıca her saat için şunları da görür:
  - günün saati, haftanın günü ve ay,
  - bir gün önceki ve bir hafta önceki aynı saatin tüketimi,
  - son bir gün ve son bir haftanın ortalaması.

### Nasıl tahmin ediyor?

Varsayılan yöntem **"rastgele orman"** (random forest) adında bir makine öğrenmesi yöntemidir. En basit hâliyle şöyle çalışır:

1. Geçmişteki her saat için şu soruyu sorar: "Bu saatin özellikleri neydi, tüketim ne oldu?" Özellikler saat, gün, tatil mi, dün bu saatte ne vardı gibi bilgilerdir.
2. Bu örneklerden **60 küçük karar ağacı** öğrenir. Her ağaç biraz farklı bir kurala varır, örneğin "hafta içiyse ve saat 8–18 arasıysa ve dün bu saatte 50'nin üstündeyse → yaklaşık 55".
3. Gelecekteki her saat için 60 ağaca da sorar:
   - Cevapların **ortancası** tahmin olur (ekranda "Tahmin (medyan)").
   - Cevapların alttan %10'u ve üstten %10'u **güven bandını** oluşturur (ekranda "%10–%90 aralığı"). Bant ne kadar genişse model o kadar emin değildir.
4. İleriye saat saat gider. Bir saatin tahmini, bir sonraki saatin "önceki saat" bilgisi olarak kullanılır.

### Yedek yöntem

Asıl model çalışmazsa sistem otomatik olarak basit bir yönteme geçer. Bu yöntem **"mevsimsel naif"** diye geçer ve şöyle hesaplar: "Son 4 haftanın aynı gün ve aynı saatine bak, ortancasını al." Asıl model hata verdiğinde veya kapalı olduğunda bu devreye girer. Ekranda "… kullanılamadı; temel istatistiksel model kullanıldı." yazar.

> Sistem, harici bir yapay zekâ modelini (Chronos) ve LSTM denen bir sinir ağını da destekler. Ancak new.ekokod.com'da bu ikisi açık değildir. Şu an kullanılan model rastgele ormandır.

### Ne zaman tahmin vermez?

- Sayacın **hiç verisi yoksa**, "Bu analizör için geçmiş tüketim verisi yok." yazar.
- **Veri çok azsa** da tahmin vermez. Saatlik tahmin için en az **336 saatlik (2 hafta)** veri gerekir, aylık tahmin için en az **28 günlük**. Ekranda "Geçmiş veri yetersiz…" yazar.
- Geçmişte **boşluklar** varsa tahmin yine yapılır, ama ekranda "Verideki boşluklar" uyarısı çıkar. Boşluk çoksa tahmin daha az güvenilir olur.

### Ne zaman çalışıyor?

- **Her gece saat 04:00'te** sistem, her şirketin aktif sayaçları için **önümüzdeki 7 günün** saatlik tahminini kendiliğinden yapar ve kaydeder. "Tüketim Tahmini" ekranı bu kayıtlı tahmini gösterir.
- Ekrandaki **"Tahmin et"** düğmesi aynı hesabı o anda yeniden yapar.

## 2. Yapay Zekâ Analizi ekranı

Bu ekranda dört sekme var. İlk üçü yukarıdaki tahminin farklı görünümleridir, dördüncüsü anomali kontrolüdür.

| Sekme | Ne yapar? |
|---|---|
| **Günlük tahmin** | Seçilen günün 24 saatini tek tek tahmin eder ve günün toplamını verir. |
| **Haftalık tahmin** | Seçilen haftanın (Pazartesi–Pazar) her gününün toplamını verir. |
| **Aylık tahmin** | Seçilen ayın her gününü, son 180 günün günlük verisine bakarak tahmin eder. |
| **Anomali kontrolü** | Belirli bir saatteki tüketimin "normal mi, tuhaf mı" olduğunu söyler. |

Bu ekrandaki tahminler **kaydedilmez**. Sadece o an gösterilir.

### Anomali kontrolü nasıl karar veriyor?

Soru şudur: "Bu saatteki tüketim, geçmişteki benzer saatlere göre olağan dışı mı?"

1. Kontrol edilen saat örneğin **Salı 14:00** ise, sistem **son 12 haftanın bütün Salı 14:00'lerine** bakar.
2. Bu değerlerin **ortancasını** alır. Bu, "beklenen" değerdir.
3. Değerlerin normalde ne kadar oynadığına bakar. Buna **tipik sapma** denir.
4. Gerçek değer beklenen değerden **tipik sapmanın 3,5 katından fazla** uzaksa sonuç **"Anomali"** olur. Değilse **"Normal"** olur.
5. Ekranda şunlar da gösterilir:
   - **Sapma skoru**: gerçek değer, beklenenden kaç "tipik sapma" uzakta?
   - **Beklenen aralık**: bu aralığın içi normal sayılır.

Notlar:

- Karşılaştırma için en az **8 benzer saat** gerekir. Daha azı varsa "Karşılaştırma için yeterli geçmiş yok" yazar.
- "Gerçekleşen değer" boş bırakılırsa sistem o saatin kayıtlı tüketimini kullanır.
- Geçmişte o saatler hep aynıysa (hiç oynamıyorsa), çok küçük farklar anomali sayılmasın diye beklenen değerin %5'i tolerans olarak kullanılır.
- Ortanca kullanıldığı için geçmişteki birkaç tuhaf değer sonucu bozmaz. Yöntemin adı bu yüzden "sağlam z-skoru"dur.

## Özet tablo

| | Tüketim tahmini | Anomali kontrolü |
|---|---|---|
| Baktığı geçmiş | Son 8 hafta (saatlik) / son 180 gün (aylık) | Son 12 haftanın aynı gün ve saati |
| Yöntem | Rastgele orman (yedek: son 4 haftanın ortancası) | Ortanca + sapma (3,5 kat kuralı) |
| Takvim / tatil | Kullanılır | Kullanılmaz |
| Hava durumu | Şu an gönderilmiyor | Kullanılmaz |
| Çıktı | En olası değer + %10–%90 aralığı | Normal / Anomali + beklenen aralık |
| Otomatik mi? | Her gece 04:00'te 7 günlük tahmin | Hayır, düğmeyle |
