# kasa

Günlük birikim takibi için terminal aracı: nakit (TL/USD/USDC) + gram altın + hisse (BIST/ABD) + vadeli mevduatı tek ekranda, canlı fiyatlarla ve günlük getiriyle gösterir. `kasa ai` ile portföyün üzerine Claude'la sohbet edebilirsin.

```
◆ kasa                                          16 Ağu 2026 · Paz  ↻ 30s

₺2.295.355   +₺8.287 · +%0,36 bugün   ≈ $47.941

▁▁▁▂▂▂▃▃▂▂▂▂▂▂▃▃▄▄▄▅▅▆▆▆▆▆▆▇▇█
son 30 gün +%6,00

VARLIK     MİKTAR      FİYAT      GÜN      GÜN ₺       DEĞER     PAY      K/Z ₺
────────────────────────────────────────────────────────────────────────────────
Altın      150 gr   ₺6.736,33   +%0,68   +₺6.825   ₺1.010.450  %44,0  +₺160.450
USD       $12.500      ₺47,88   +%0,11     +₺654     ₺598.489  %26,1  +₺188.489
...
```

## Kurulum

```sh
go install github.com/furkando/kasa/cmd/kasa@latest   # veya repo içinden:
go install ./cmd/kasa
```

`~/go/bin` PATH'te olmalı.

## Kullanım

- `kasa` — canlı dashboard (30 sn'de bir yenilenir; `r` elle yenile, `q` çık)
- **Detay:** `enter` seçili varlığın detayını açar — büyük değer grafiği (`1`/`3`/`y` ile 1 ay·3 ay·1 yıl), **7/30 günlük değişim hem ₺ hem $ cinsinden**, ilgili haber başlıkları (Yahoo); vadelide işleyen faiz, günlük tahakkuk, kalan gün ve vade sonu; faizli nakitte tahakkuk. `↑↓` ile detaydan çıkmadan varlıklar arasında gezinilir, `e` oradan düzenlemeye geçer.
- **Kur sayfası:** `u` USD/TRY detayını açar — anlık kur, önceki kapanış, 1a/3a/1y grafik, dönem min–max.
- **Sıralama:** `s` tabloyu döndürür: değer → gün % → gün ₺ → K/Z → ad; aktif kolon başlığında `↓` işareti.
- **Gizle/Göster:** `h` seçili satırı toplam dışına alır (satır sönük olarak "GİZLİ" bölümünde kalır, tekrar `h` geri getirir) — kilitli token, satılamaz varlık vb. için.
- **Kategoriler:** varlıklar otomatik kategorilenir (nakit, kripto, hisse, altın, vadeli, gayrimenkul, alacak/borç; mülk formunda serbest kategori — örn. araç). `g` gruplu ↔ düz görünüm; grup başlıklarında kategori toplamı, pay ve günlük değişim.
- **Mülk:** `n` → `m` ile ev/ofis/araba gibi sabit değerli varlık (TL/USD; USD olanlar kurla günlük değişir; alış fiyatı girersen K/Z).
- **Kripto:** `n` → `k` ile CoinGecko id'siyle canlı fiyatlı coin (örn. `lighter`, `bitcoin`); ort. maliyet $ ve not (kilit/vesting) alanları; detayda grafik + not.
- **Alacak/Borç:** `n` → `b` ile TL veya USD alacak (sana ödenecek, + değer) ya da borç (senin ödeyeceğin, − değer) eklenir; opsiyonel vade tarihi detayda kalan günle görünür. USD olanlar kurla birlikte günlük değişir, net toplam buna göre hesaplanır.
- **Yönetim tamamen TUI'den:** `e` varlığın **tüm alanlarını** düzenler (form: ad, tutar, faiz, tarihler…) · `n` yeni varlık (nakit/hisse/vadeli/altın) · `x` sil · `o` ayarlar (yenileme, AI sağlayıcı/model). Kaydedince `portfolio.yaml` yeniden yazılır — dosya artık sadece depo, elle düzenlemek gerekmiyor.
- **Vadeli faizi:** brüt oranı (%41 gibi) ve stopajı (%17,5) girersin; net oranı ve günlük tahakkuku kasa hesaplar. Faizli nakitte tutar/oran değişince `as_of` otomatik bugüne alınır.
- `kasa --once` (veya `-1`) — tek seferlik çıktı, script/quick-glance için
- `kasa demo` — rastgele üretilmiş örnek portföyle açılır (canlı fiyatlar gerçek): ekran görüntüsü paylaşmak için; gerçek config'e ve geçmişe dokunmaz, demo içindeki düzenlemeler geçici dosyaya gider. `kasa demo --once` da çalışır.
- **AI:** dashboard'da `a` sohbet panelini açar (`esc` geri); `kasa ai [soru]` alt komutu da var. Backend otomatik seçilir: makinede `claude` CLI kuruluysa **Claude Pro/Max aboneliğin** üzerinden çalışır (API kredisi gerekmez, ilk kullanımda `claude` içinde login olmuş olman yeterli); yoksa `ANTHROPIC_API_KEY` ile Anthropic API kullanılır. `ai.provider: claude|api` ile sabitlenebilir, `ai.model` ile model seçilir.

Nakit hesaplara `apy` + `as_of` verirsen (ör. Midas USD %3) günlük tahakkuk işlenir; USDC ≈ USD kabul edilir.

İlk çalıştırmada `~/.config/kasa/portfolio.yaml` şablonu oluşturulur; varlıklarını oraya yazarsın. Para ekleyince/çıkarınca sadece bu dosyayı güncelle. Çalışma dizinindeki `portfolio.yaml` veya `BIRIKIM_CONFIG` env değişkeni öncelik alır.

## Veri kaynakları

- **Yahoo Finance chart API** — USD/TRY (`TRY=X`), BIST (`SEMBOL.IS`), ABD hisseleri, ons altın (`GC=F`); canlı fiyat + önceki kapanış + 30 günlük tarihçe. query1 düşerse query2'ye düşer.
- **truncgil v4** — gram altın spot (`GRA`, Kapalıçarşı bazlı). Erişilemezse `GC=F × USDTRY` türetmesine düşer (~%1-2 baz farkı olabilir).
- Gram altın **tarihçesi** her zaman `GC=F × USDTRY` türetmesidir (truncgil geçmiş vermiyor); baz tutarlı olduğu için günlük getiriler doğrudur.
- Vadeli mevduat fiyat çekmez: `gross_rate × (1 − stopaj)` net oranla günlük basit-faiz tahakkuku (eski `annual_rate` alanı da hâlâ okunur).

## Notlar

- Gün değişimi ₺ kolonu kur etkisini içerir (ABD hissesinde hisse + USDTRY birlikte); `GÜN` yüzdesi varlığın kendi kurundaki hareketidir.
- Her çalıştırmada günün değeri `~/.local/share/kasa/snapshots.json`'a yazılır — holdings değişse bile gerçek tarihsel seri burada birikir.
- Dar terminalde kolonlar önem sırasına göre gizlenir (30G → K/Z% → MİKTAR → K/Z₺ → GÜN₺).
- Toplam K/Z yalnızca maliyeti (`cost`/`cost_try`) girilmiş varlıkları kapsar.
