# Sunum Transkripti — Ring of the Middle Earth
## 15 dakika canlı demo + 5 dakika Soru-Cevap

---

## BAŞLAMADAN ÖNCE (Ön Hazırlık — ~5 dakika, 15 dakikaya dahil değil)

### Adım 0 — Tur süresini 30 saniyeye düşür (demo için hızlandır)

```bash
# Config dosyasını düzenle: turn-duration-seconds 60'tan 30'a
# Dosya: config/units.conf, son satır:
sed -i '' 's/turn-duration-seconds = 60/turn-duration-seconds = 30/' \
  /Users/emreyildiz/distrubuted-2/ring-of-the-middle-earth/config/units.conf
```

### Adım 1 — Temiz yeniden başlatma

```bash
cd /Users/emreyildiz/distrubuted-2/ring-of-the-middle-earth
docker compose down -v          # volume'ları sil (eski Kafka/ZK durumunu temizler)
make up                         # tam sistemi başlat
```

~30 saniye bekle, tüm servisler sağlıklı olana kadar:
```bash
docker compose ps               # tüm servisler "healthy" veya "Up" görünmeli
curl http://localhost/health    # {"status":"ok"} dönmeli
```

### Adım 2 — İki tarayıcı sekmesi aç

```
Sekme 1: http://localhost  → LIGHT SIDE'a tıkla
Sekme 2: http://localhost  → DARK SIDE'a tıkla
```

Her iki pencereyi yan yana ekranda hizala.

### Adım 3 — Senaryo 1 için birimleri konumlandır (15 dakika başlamadan önce gönder)

**Mevcut turu kontrol et (1 olmalı):**
```bash
curl -s "http://localhost/game/state?playerId=light" | python3 -c \
  "import sys,json; print('Tur:', json.load(sys.stdin)['turn'])"
```

**Light Side — Yüzük Taşıyıcısı'nı Weathertop'a yönlendir:**
```bash
curl -X POST http://localhost/order -H "Content-Type: application/json" -d '{
  "orderType":"ASSIGN_ROUTE",
  "playerId":"light",
  "unitId":"ring-bearer",
  "turn":1,
  "pathIds":["shire-to-bree","bree-to-weathertop"]
}'
# Beklenen: {"status":"accepted"}
```

**Dark Side — Cadı-Kral'ı kuzeye, Yüzük Taşıyıcısı'na doğru yönlendir:**
```bash
curl -X POST http://localhost/order -H "Content-Type: application/json" -d '{
  "orderType":"ASSIGN_ROUTE",
  "playerId":"dark",
  "unitId":"witch-king",
  "turn":1,
  "pathIds":[
    "osgiliath-to-minas-morgul",
    "minas-tirith-to-osgiliath",
    "rohan-plains-to-minas-tirith",
    "lothlorien-to-rohan-plains"
  ]
}'
# Beklenen: {"status":"accepted"}
```

**~2,5 dakika bekle (5 tur × 30 saniye) birimlerin konumlanması için:**
```bash
# 5. tura kadar sorgula:
until [ "$(curl -s 'http://localhost/game/state?playerId=light' | \
  python3 -c 'import sys,json; print(json.load(sys.stdin)["turn"])')" -ge 5 ]; do
  sleep 5
done && echo "HAZIR — şimdi 15 dakika saatini başlat"
```

4. tur işlendikten sonra:
- Yüzük Taşıyıcısı **Weathertop**'ta (3. turda geldi, orada bekliyor)
- Cadı-Kral **Lothlórien**'de (Weathertop'tan 2 adım uzakta — tespit menzili 3 içinde)

---

## ▶ 15 DAKİKA SAATİNİ BAŞLAT

---

## BÖLÜM A — Dağıtık Mimari (2 dakika)

**Ne söyleyip göstereceğin:**

Terminal aç. Şunu çalıştır:
```bash
docker compose ps
```

**Söyle:** "Sistem Docker üzerinde çalışıyor ve dağıtık bir kümeyi simüle ediyor. Şunlar var:
- Tek hata noktası olmayan 3 Kafka broker kümesi
- Kafka koordinasyonu için ZooKeeper
- 13 Avro şemasını doğrulayan Confluent Schema Registry
- nginx arkasında 3 stateless Go uygulama instance'ı (go-1, go-2, go-3)
- Yük dengeleyici ve SSE sticky router olarak görev yapan nginx"

nginx config'ini göster:
```bash
cat nginx.conf | grep -A5 "upstream\|sse_backend"
```

**Söyle:** "Normal API çağrıları üç Go instance'ına round-robin dağıtılıyor. SSE bağlantıları oyuncu ID'sine göre sabitlenmiş — Light Side her zaman go-1'e, Dark Side go-2'ye bağlanıyor. Bu sayede her oyuncu tutarlı bir event akışı alıyor."

Canlı göster:
```bash
curl http://localhost/health          # go-1/go-2/go-3'den birine gider
docker compose logs --tail=5 go-1    # başlangıç loglarını göster
```

**Söyle:** "Her Go instance'ı aynı anda 9 farklı goroutine tipini çalıştırıyor — Kafka consumer'lar, EventRouter, CacheManager, TurnProcessor, iki fan-out analiz pipeline'ı, SSE goroutine'leri ve bir HTTP sunucusu. Goroutine sınırlarını geçen hiçbir mutable paylaşımlı durum yok — her şey channel veya mutex üzerinden."

---

## BÖLÜM B — Kod Gezintisi (2 dakika)

Bu dosyaları kısaca göster (editörde açık olsun):

**1. `option-b/internal/router/router.go` — bilgi asimetrisi uygulama noktası:**
```bash
cat option-b/internal/router/router.go | head -50
```
**Söyle:** "EventRouter, kod tabanında topic yönlendirmesinin gerçekleştiği TEK yerdir. `game.ring.position` yalnızca Light Side'a gider. `game.ring.detection` yalnızca Dark Side'a gider. `game.broadcast`, Dark Side'a teslim edilmeden önce ring-bearer alanı sıyrılır. Tek bir fonksiyon, race detector ile test edilmiş."

**2. `option-b/internal/game/turn.go` satır 34 — saf fonksiyon:**
**Söyle:** "ProcessTurn saf bir fonksiyondur. Dünya durumu anlık görüntüsü ve doğrulanmış emirler listesi alır, 13 deterministik adım çalıştırır ve TurnResult döner. I/O yok, yan etki yok. Kafka replay'i bu sayede mümkün — emirleri tekrar oynatarak herhangi bir oyun durumunu yeniden inşa edebilirsin."

**3. `option-b/internal/kafka/validator.go` — Kafka Streams Topoloji 1:**
**Söyle:** "OrderValidator, KTable tabanlı bir emir doğrulama pipeline'ı uygular. 8 kural kontrol eder: doğru tur, birim sahipliği, yol uygunluğu, komşuluk, bekleme süreleri. Geçerli emirler `game.orders.validated`'a üretilir; geçersiz emirler `game.dlq`'ya gider. Her turdan sonra KTable güncellenen cache'den yeniden inşa edilir, böylece 2. tur ve sonrasındaki emirler doğru şekilde doğrulanır."

---

## SENARYO 1 — Bilgi Gizleme (5 dakika)

> **Önce durum kontrolü — konumları doğrula:**

```bash
# Light Side Yüzük Taşıyıcısı'nı Weathertop'ta görüyor
curl -s "http://localhost/game/state?playerId=light" | \
  python3 -c "import sys,json; d=json.load(sys.stdin); \
  print('Tur:', d['turn']); print('YT:', d['ringBearer'])"

# Dark Side boş konum görüyor
curl -s "http://localhost/game/state?playerId=dark" | \
  python3 -c "import sys,json; d=json.load(sys.stdin); \
  print('Tur:', d['turn']); print('YT:', d['ringBearer'])"
```

**Beklenen çıktı:**
```
Light Side → YT: {'currentRegion': 'weathertop'}
Dark Side  → YT: {'currentRegion': '', 'lastDetectedRegion': '', 'lastDetectedTurn': 0}
```

**Söyle:** "Light Side, Yüzük Taşıyıcısı'nı Weathertop'ta görebiliyor. Dark Side boş string görüyor — konum tamamen gizli. Bu EventRouter'da, WorldStateCache.GetPublicState'de ve Dark Side SSE kanalına hiçbir zaman game.ring.position yayınlanmamasıyla üç noktadan uygulanıyor."

> **Tespitin tetiklenmesini göster:**

Mevcut turu al:
```bash
TURN=$(curl -s "http://localhost/game/state?playerId=light" | \
  python3 -c "import sys,json; print(json.load(sys.stdin)['turn'])")
echo "Mevcut tur: $TURN"
```

**Her iki tarayıcı sekmesine yan yana işaret et.**

Yüzük Taşıyıcısı'nın geçeceği yolda gözetleme artırmak için SEARCH_PATH emri gönder, sonra Event Log'u izle:
```bash
# Dark Side: Cadı-Kral, Yüzük Taşıyıcısı'nın sonraki turda geçeceği yolu arıyor
# (cadı-kral şu an lothlórien'de, moria-to-lothlorien'e komşu)
curl -X POST http://localhost/order -H "Content-Type: application/json" -d "{
  \"orderType\":\"SEARCH_PATH\",
  \"playerId\":\"dark\",
  \"unitId\":\"witch-king\",
  \"turn\":$TURN,
  \"pathId\":\"moria-to-lothlorien\"
}"
```

Turun tetiklenmesi için 30 saniye bekle. **Tarayıcı sekmelerine işaret et.**

**Tur sonrası:**
- Dark Side Event Log'u: `RING_BEARER_DETECTED` veya `RingBearerSpotted` eventi gösterir
- Light Side Event Log'u: bu eventi GÖSTERMEZ

```bash
# API üzerinden doğrula:
curl -s "http://localhost/game/state?playerId=dark" | \
  python3 -c "import sys,json; d=json.load(sys.stdin); print('DarkView:', d['ringBearer'])"
# Beklenen: lastDetectedRegion artık bir değer gösteriyor
```

**Söyle:** "Dark Side tespit eventini aldı. Light Side almadı. EventRouter bunu kanal seviyesinde uygular — Dark Side SSE kanalına yüzük taşıyıcısının konumunu yanlışlıkla gönderebilecek hiçbir kod yolu yoktur."

**Curl ile doğrula (eğitmen bunu canlı çalıştırsın):**
```bash
# Üç bağımsız uygulama noktası:
echo "=== 1. REST API (DarkView) ===" && \
curl -s "http://localhost/game/state?playerId=dark" | python3 -m json.tool | grep -A3 "ringBearer"

echo "=== 2. REST API (LightView) ===" && \
curl -s "http://localhost/game/state?playerId=light" | python3 -m json.tool | grep -A3 "ringBearer"
```

---

## SENARYO 2 — Maia Dispatch ve Yol Mekanikleri (5 dakika)

> **Mevcut turu al:**
```bash
TURN=$(curl -s "http://localhost/game/state?playerId=light" | \
  python3 -c "import sys,json; print(json.load(sys.stdin)['turn'])")
echo "Mevcut tur: $TURN"
```

> **Adım 1: Bir yolu engelle (Dark Side — İsengard'daki Saruman komşu yolu bloke ediyor)**

```bash
curl -X POST http://localhost/order -H "Content-Type: application/json" -d "{
  \"orderType\":\"BLOCK_PATH\",
  \"playerId\":\"dark\",
  \"unitId\":\"saruman\",
  \"turn\":$TURN,
  \"pathId\":\"fangorn-to-isengard\"
}"
# Beklenen: {"status":"accepted"}
```

> **Adım 2: Aynı tur — Gandalf bloke edilen yolu açıyor (Maia Dispatch Demo)**

```bash
curl -X POST http://localhost/order -H "Content-Type: application/json" -d "{
  \"orderType\":\"MAIA_ABILITY\",
  \"playerId\":\"light\",
  \"unitId\":\"gandalf\",
  \"turn\":$TURN,
  \"targetPathId\":\"fangorn-to-isengard\"
}"
# Beklenen: {"status":"accepted"}
```

> **Adım 3: Aynı tur — Saruman farklı bir yolu bozuyor (PathCorrupted demo)**

```bash
curl -X POST http://localhost/order -H "Content-Type: application/json" -d "{
  \"orderType\":\"MAIA_ABILITY\",
  \"playerId\":\"dark\",
  \"unitId\":\"saruman\",
  \"turn\":$TURN,
  \"targetPathId\":\"fords-of-isen-to-edoras\"
}"
# Bekle — saruman bu turda zaten BLOCK_PATH kullandı. Duplicate emir reddedilecek.
# DÜZELTME: Saruman MAIA_ABILITY'yi bir sonraki turda gönder.
```

**SUNUCU NOTU:** Saruman tur başına yalnızca bir emir gönderebilir. PathCorrupt'ı sonraki turda gönder:

```bash
# Turun geçmesini bekle, sonra:
TURN=$((TURN + 1))
curl -X POST http://localhost/order -H "Content-Type: application/json" -d "{
  \"orderType\":\"MAIA_ABILITY\",
  \"playerId\":\"dark\",
  \"unitId\":\"saruman\",
  \"turn\":$TURN,
  \"targetPathId\":\"fords-of-isen-to-edoras\"
}"
# Beklenen: {"status":"accepted"}
```

**Turun tetiklenmesini bekle. Tarayıcı haritasını göster.**

> **Tur tetiklendikten sonra haritada gözlemlenecekler:**

- `fangorn-to-isengard` → her iki tarayıcıda da **mavi kesik çizgi** (TEMPORARILY_OPEN) olarak görünür
- `fords-of-isen-to-edoras` → PathCorrupt sonrası **turuncu** (yüksek gözetleme) görünür

```bash
# Yol durumlarını doğrula:
curl -s "http://localhost/game/state?playerId=light" | \
  python3 -c "
import sys,json; d=json.load(sys.stdin)
for p in d['paths']:
    if p['id'] in ['fangorn-to-isengard','fords-of-isen-to-edoras']:
        print(p['id'], '→', p['status'], 'gözetleme:', p['surveillanceLevel'])
"
```

**Söyle:** "Hem Gandalf hem Saruman tam olarak aynı emir tipini kullanıyor: `MAIA_ABILITY`. Oyun motoru, birim ID string sabitleri kullanmadan sadece birimin config'ini okuyarak farklı efektlere dispatch ediyor — Gandalf'ın `maiaAbilityPaths = []` (boş) olması OpenPath davranışı anlamına gelir. Saruman'ın boş olmayan listesi CorruptPath'i tetikler. Dispatch mantığında hiçbir yerde birim ID string sabiti geçmiyor."

> **Kodu kısaca göster:**
```bash
grep -A8 "MaiaAbilityPaths" option-b/internal/game/turn.go | head -15
```

**Söyle:** "`len(cfg.MaiaAbilityPaths) == 0` ise → Gandalf davranışı. Aksi halde → Saruman davranışı. Config her şeyi yönlendiriyor."

> **2 tur daha bekle — Gandalf yolunun geri döndüğünü göster:**

```bash
# Gandalf yolu açtıktan 2 tur sonra:
curl -s "http://localhost/game/state?playerId=light" | \
  python3 -c "
import sys,json; d=json.load(sys.stdin)
for p in d['paths']:
    if p['id'] == 'fangorn-to-isengard':
        print('fangorn-to-isengard durumu:', p['status'])
"
# Beklenen: durum tekrar BLOCKED (saruman hala yol ucundaki bölgede)
```

**Söyle:** "Gandalf'ın yolu tam olarak 2 tur açık kaldı, sonra bloke eden birim (Saruman) hala yol ucunda olduğu için geri döndü. Saruman'ın fords-of-isen-to-edoras üzerindeki bozulması ise kalıcı — SurveillanceLevel 3'e yükseltildi, hiç sıfırlanmaz."

> **Duplicate emir reddini kanıtlamak için DLQ'yu göster:**
```bash
# Gandalf'ı tekrar göndermeyi dene (cooldown = 3 tur)
TURN=$(curl -s "http://localhost/game/state?playerId=light" | \
  python3 -c "import sys,json; print(json.load(sys.stdin)['turn'])")
curl -X POST http://localhost/order -H "Content-Type: application/json" -d "{
  \"orderType\":\"MAIA_ABILITY\",
  \"playerId\":\"light\",
  \"unitId\":\"gandalf\",
  \"turn\":$TURN,
  \"targetPathId\":\"fangorn-to-isengard\"
}"
# Beklenen: emir kabul edilir ama validator tarafından REDDEDİLİR → DLQ'ya gider
# Sonra DLQ'yu göster:
docker exec ring-of-the-middle-earth-kafka-1-1 \
  kafka-console-consumer --bootstrap-server localhost:9092 \
  --topic game.dlq --from-beginning --timeout-ms 3000 2>/dev/null | \
  python3 -c "import sys; [print(l) for l in sys.stdin if 'COOLDOWN' in l or 'DLQ' in l]"
```

---

## SENARYO 3 — Hata Toleransı ve Exactly-Once (5 dakika)

### Bölüm A: Yüzük Taşıyıcısı'nı Cehennem Dağı'na Yönlendir

```bash
TURN=$(curl -s "http://localhost/game/state?playerId=light" | \
  python3 -c "import sys,json; print(json.load(sys.stdin)['turn'])")
RB=$(curl -s "http://localhost/game/state?playerId=light" | \
  python3 -c "import sys,json; print(json.load(sys.stdin)['ringBearer']['currentRegion'])")
echo "Tur: $TURN, Yüzük Taşıyıcısı: $RB"
```

Mevcut konumdan Cehennem Dağı'na tam rota gönder (YT neredeyse oradaki başlangıç yolunu ayarla):

```bash
# Yüzük Taşıyıcısı weathertop'ta olmalı. Rota:
curl -X POST http://localhost/order -H "Content-Type: application/json" -d "{
  \"orderType\":\"ASSIGN_ROUTE\",
  \"playerId\":\"light\",
  \"unitId\":\"ring-bearer\",
  \"turn\":$TURN,
  \"pathIds\":[
    \"weathertop-to-rivendell\",
    \"rivendell-to-moria\",
    \"moria-to-lothlorien\",
    \"lothlorien-to-emyn-muil\",
    \"emyn-muil-to-dead-marshes\",
    \"dead-marshes-to-mordor\",
    \"mordor-to-mount-doom\"
  ]
}"
# Beklenen: {"status":"accepted"}
```

**Söyle:** "Yüzük Taşıyıcısı'nın Cehennem Dağı'na 7 adımı var. Her turda otomatik olarak bir adım ilerler. 30 saniyelik turlarla bu 3,5 dakika sürer. Beklerken consumer group mimarisini açıklayacağım."

**Beklerken açıkla (docker loglarını göster):**
```bash
# 3 ayrı consumer group'u göster
docker exec ring-of-the-middle-earth-kafka-1-1 \
  kafka-consumer-groups --bootstrap-server localhost:9092 --list
```

**Söyle:** "Her Go instance'ı kendi consumer group ID'siyle abone oluyor: game-engine-1, game-engine-2, game-engine-3. Bu, ÜÇ instance'ın da her doğrulanmış emri alıp bağımsız olarak işlediği anlamına geliyor. Durum Kafka'da yaşıyor — Go instance'ları stateless. Bu yüzden bir instance'ı öldürmek hiçbir veri kaybetmiyor."

### Bölüm B: Tur İşleme Sırasında go-2'yi Öldür

**Yüzük Taşıyıcısı Cehennem Dağı'na 1-2 tur kala bekle:**
```bash
# YT mordor'a yaklaşana kadar sorgula
until curl -s "http://localhost/game/state?playerId=light" | \
  python3 -c "import sys,json; rb=json.load(sys.stdin)['ringBearer']['currentRegion']; \
  print(rb); exit(0 if rb in ['mordor','dead-marshes'] else 1)" 2>/dev/null; do
  sleep 5
done && echo "Yüzük Taşıyıcısı Cehennem Dağı yakınında — go-2'yi öldürmeye hazır"
```

**go-2'yi öldür:**
```bash
docker compose stop go-2
echo "go-2 durduruldu"
```

**Hemen Kafka rebalance loglarını göster:**
```bash
docker compose logs kafka-1 --follow 2>&1 | grep -i "rebalance\|join\|leader" &
# ~10 saniye çalışmasına izin ver sonra Ctrl+C
```

**Söyle:** "Kafka'nın consumer group protokolü game-engine-2'nin çevrimdışı olduğunu algıladı. Rebalance tetiklendi — go-2'nin partition'ları go-1 ve go-3'e yeniden dağıtıldı. Bu 10 saniyeden az sürer. Oyun kalan iki instance'da kesintisiz devam eder."

**Oyunun devam ettiğini doğrula:**
```bash
sleep 35  # sonraki tur tick'ini bekle
curl -s "http://localhost/game/state?playerId=light" | \
  python3 -c "import sys,json; d=json.load(sys.stdin); \
  print('Tur:', d['turn'], '| YT:', d['ringBearer']['currentRegion'])"
```

**Beklenen:** Tur ilerledi, Yüzük Taşıyıcısı hareket etti — go-1 ve go-3'ün go-2 olmadan işlemeye devam ettiğinin kanıtı.

### Bölüm C: Yüzük Taşıyıcısı Cehennem Dağı'na Ulaşıyor — Oyun Bitti

```bash
# mount-doom'a kadar sorgula
until curl -s "http://localhost/game/state?playerId=light" | \
  python3 -c "import sys,json; \
  rb=json.load(sys.stdin)['ringBearer']['currentRegion']; \
  exit(0 if rb == 'mount-doom' else 1)" 2>/dev/null; do sleep 5; done
echo "Yüzük Taşıyıcısı Cehennem Dağı'nda!"

# game.broadcast'te GameOver eventi kontrol et:
docker exec ring-of-the-middle-earth-kafka-1-1 \
  kafka-console-consumer \
  --bootstrap-server localhost:9092 \
  --topic game.broadcast \
  --from-beginning \
  --timeout-ms 3000 2>/dev/null | \
  python3 -c "
import sys, json
count = 0
for line in sys.stdin:
    try:
        d = json.loads(line)
        if 'winner' in d or 'game-over' in str(d):
            count += 1
            print(f'GameOver eventi #{count}:', d)
    except:
        pass
print(f'game.broadcast toplam GameOver eventi: {count}')
"
```

**Beklenen çıktı:**
```
GameOver eventi #1: {'winner': 'FREE_PEOPLES', 'cause': 'RING_DESTROYED', 'turn': N, ...}
game.broadcast toplam GameOver eventi: 1
```

**Söyle:** "game.broadcast'te tam olarak bir GameOver eventi var. Bunun nedeni runTurnProcessor'ın result.Winner boş olmadığında ProduceSync'i tam olarak bir kez çağırması ve ardından hemen return etmesidir — goroutine çıkar. Sonraki turlar işlenemez. Kafka topic'i kalıcı kayıttır."

### Bölüm D: go-2'yi Yeniden Başlat ve Kurtarımı Doğrula

```bash
docker compose start go-2
sleep 5
echo "go-2 yeniden başlatıldı"

# go-2'nin consumer group'a yeniden katıldığını doğrula:
docker exec ring-of-the-middle-earth-kafka-1-1 \
  kafka-consumer-groups --bootstrap-server localhost:9092 \
  --group game-engine-2 --describe 2>/dev/null | head -10
```

```bash
# go-2'nin doğru oyun durumuna sahip olduğunu doğrula:
curl -s "http://go-2:8080/game/state?playerId=light" 2>/dev/null | \
  python3 -c "import sys,json; d=json.load(sys.stdin); \
  print('go-2 turu:', d['turn'], '| YT:', d['ringBearer']['currentRegion'])"
```

*(go-2'ye host'tan doğrudan curl çalışmazsa Docker network üzerinden kontrol et)*

**Söyle:** "go-2 consumer group'a yeniden katıldı. Kafka, go-2'nin ayrıldığı noktadan itibaren partition offset'lerini tekrar oynadı. go-2, event akışından WorldStateCache'ini yeniden inşa etti — manuel müdahale yok, veri kaybı yok. Bu 'durum Kafka'da' tasarım ilkesidir."

---

## BÖLÜM C — Birim Testler (1 dakika)

```bash
cd /Users/emreyildiz/distrubuted-2/ring-of-the-middle-earth
make test
```

**Beklenen çıktı:**
```
=== RUN   TestCombat_TiePlains              PASS
=== RUN   TestCombat_FortressTerrain        PASS
=== RUN   TestCombat_UrukHaiIgnoresFortress PASS
=== RUN   TestCombat_UrukHaiVsFortified     PASS
=== RUN   TestCombat_LeadershipBonus        PASS
=== RUN   TestCombat_IndestructibleUnit     PASS
=== RUN   TestPipeline1_KnownThreat         PASS
=== RUN   TestPipeline1_NazgulProximity     PASS
=== RUN   TestPipeline2_PositiveIntercept   PASS
=== RUN   TestPipeline2_NegativeIntercept   PASS
=== RUN   TestRouter_DarkSideNeverSeesRB    PASS
=== RUN   TestRouter_RingBearerMovedEvent   PASS
=== RUN   TestRouter_DarkViewAlwaysEmpty    PASS
ok  github.com/rotr/ring-of-the-middle-earth/tests  (race: PASS)
```

**Söyle:** "13 birim test, hepsi geçiyor, race detector (`-race` flag) dahil. Router testleri doğrudan DarkView.RingBearerRegion'ın eşzamanlı okuma ve yazmalarda her zaman boş olduğunu iddia ediyor — az önce canlı olarak gösterdiğimiz aynı değişmez. Testler Docker olmadan çalışıyor — Kafka bağımlılığı yok."

---

## BÖLÜM D — Rapor (30 saniye)

```bash
# Rapor yapısını göster:
wc -l REPORT.md    # 358 satır
head -60 REPORT.md  # içindekiler tablosunu göster
```

**Söyle:** "Rapor 16 bölüm kapsıyor: mimari, 9-goroutine tasarımı, tüm 10 Kafka topic'i, üç demo senaryosu kod walkthrough olarak, 13 adımlı tur işleme pipeline'ı ve özellikle Bölüm 12 — entegrasyon testleri sırasında bulunan ve düzeltilen 7 hatanın tablosu, her biri kök neden ve çözümüyle. Uçtan uca oyun doğrulama bölümü Yüzük Taşıyıcısı'nın turdan tura rotasını belgeliyor."

---

## ▶ 15 DAKİKA SAATİNİ DURDUR

---

## SORU-CEVAP HAZIRLIĞI — Olası Sorular

**S: Neden Akka yerine Go seçtiniz?**
> "Goroutine'ler, spesifikasyonun goroutine diyagramıyla bire bir örtüşüyor. Kafka'nın state store olarak kullanıldığı stateless tasarım, actor persistence framework'lerine olan ihtiyacı ortadan kaldırıyor. `go test -race`, bilgi asimetrisi değişmezini doğrudan doğruluyor. Akka, PersistentActor ile daha iyi tur işleme atomikliği ve ClusterSingleton ile daha iyi bilgi gizleme sağlardı — her iki değiş tokuşu da raporun 14. bölümünde ele alıyorum."

**S: Exactly-once nasıl garanti ediliyor?**
> "runTurnProcessor, result.Winner boş olmadığında ProduceSync'i bir kez çağırır, ardından goroutine return eder. Sonraki tick'ler tetiklenemez. Kafka topic'i append-only'dir — `--from-beginning` ile kafka-console-consumer kullanarak game.broadcast'te tam olarak bir GameOver mesajı olduğunu doğrulayabilirsiniz."

**S: go-1 (birincil tur işlemcisi) çökerse ne olur?**
> "Her üç instance da bağımsız TurnProcessor goroutine'leri çalıştırıyor. Kafka'nın consumer group rebalance'ı saniyeler içinde partition'ları yeniden dağıtıyor. game.orders.validated partition'ında doğrulanmış bir emir alan bir sonraki instance sonraki turu işleyecek. İki instance'ın aynı turu işlemeye çalıştığı kısa bir örtüşme penceresi var, ancak ProcessTurn saf ve idempotent olduğundan (aynı giriş aynı çıktıyı üretir) sonuç tutarlıdır."

**S: Kafka DLQ nasıl çalışıyor?**
> "Doğrulamayı geçemeyen herhangi bir emir, yapılandırılmış bir hatayla game.dlq'ya yazılır: originalTopic, errorCode, errorMessage, rawPayload. Hata kodları game paketindeki tiplendirilmiş sabitlerdir: WRONG_TURN, NOT_YOUR_UNIT, INVALID_PATH, PATH_BLOCKED, UNIT_NOT_ADJACENT, DUPLICATE_UNIT_ORDER, ABILITY_ON_COOLDOWN."

**S: Yüzük taşıyıcısının konumu veritabanında/logda nasıl gizli kalıyor?**
> "Yüzük taşıyıcısının gerçek konumu, Dark Side'ın tükettiği hiçbir Kafka topic'ine yazılmıyor. game.ring.position, Dark Side consumer'ın abonelik listesinde değil. game.broadcast, Dark Side SSE kanalına teslim edilmeden önce stripRingBearer() tarafından sıyrılıyor. REST endpoint'i /game/state?playerId=dark her zaman gerçek konumdan bağımsız olarak currentRegion='' döndürüyor."

**S: Kötü niyetli bir istemci /game/state?playerId=light çağırarak hile yapabilir mi?**
> "Gerçek bir dağıtımda oyuncuları kimlik doğrulayıp playerId'yi bir oturum token'ına bağlarsınız. Mevcut uygulama playerId parametresine güveniyor — bu, değerlendirmenin auth değil dağıtık sistem mimarisine odaklandığı göz önüne alındığında kapsam içi bir basitleştirmedir."

---

## HIZLI REFERANS — Kritik Komutlar

```bash
# Oyun durumu
curl -s "http://localhost/game/state?playerId=light" | python3 -m json.tool
curl -s "http://localhost/game/state?playerId=dark" | python3 -m json.tool

# Emir gönder (değerleri gerektiği gibi değiştir)
curl -X POST http://localhost/order \
  -H "Content-Type: application/json" \
  -d '{"orderType":"ASSIGN_ROUTE","playerId":"light","unitId":"ring-bearer","turn":N,"pathIds":["yol-1","yol-2"]}'

# Kafka topic'leri kontrol et
docker exec ring-of-the-middle-earth-kafka-1-1 \
  kafka-console-consumer --bootstrap-server localhost:9092 \
  --topic game.broadcast --from-beginning --timeout-ms 3000 2>/dev/null

docker exec ring-of-the-middle-earth-kafka-1-1 \
  kafka-console-consumer --bootstrap-server localhost:9092 \
  --topic game.dlq --from-beginning --timeout-ms 3000 2>/dev/null

# Container kontrolü
docker compose stop go-2
docker compose start go-2
docker compose logs kafka-1 --follow

# Birim testler
make test

# Tam yeniden başlatma (oyun durumunu sıfırlar)
docker compose down -v && make up
```

---

## GEÇERLİ YOL ID'LERİ (emir göndermek için)

```
shire-to-bree               the-shire → bree
bree-to-weathertop          bree → weathertop
bree-to-rivendell           bree → rivendell
bree-to-tharbad             bree → tharbad
weathertop-to-rivendell     weathertop → rivendell
rivendell-to-moria          rivendell → moria
rivendell-to-lothlorien     rivendell → lothlorien
moria-to-lothlorien         moria → lothlorien
lothlorien-to-emyn-muil     lothlorien → emyn-muil
emyn-muil-to-dead-marshes   emyn-muil → dead-marshes
dead-marshes-to-mordor      dead-marshes → mordor
mordor-to-mount-doom        mordor → mount-doom (Cehennem Dağı)
fangorn-to-isengard         fangorn ↔ isengard
fords-of-isen-to-edoras     fords-of-isen ↔ edoras
osgiliath-to-minas-morgul   osgiliath ↔ minas-morgul
```

---

## BİRİM KONUMLARI (Yeni Oyun Başlangıcı)

| Birim | Taraf | Bölge | Sınıf | Önemli özellik |
|---|---|---|---|---|
| ring-bearer | Light | the-shire | RingBearer | — |
| aragorn | Light | bree | FellowshipGuard | güç 5, liderlik |
| legolas | Light | rivendell | FellowshipGuard | güç 3 |
| gimli | Light | rivendell | FellowshipGuard | güç 3 |
| gandalf | Light | rivendell | Maia | bloke yolları açar, bekleme 3 |
| rohan-cavalry | Light | edoras | FellowshipGuard | güç 4 |
| gondor-army | Light | minas-tirith | GondorArmy | güç 5, kale inşa edebilir |
| witch-king | Dark | minas-morgul | Nazgul | tespit menzili 2, yıkılamaz |
| nazgul-2 | Dark | minas-morgul | Nazgul | tespit menzili 1, yeniden doğar |
| nazgul-3 | Dark | minas-morgul | Nazgul | tespit menzili 1, yeniden doğar |
| saruman | Dark | isengard | Maia | yolları bozar, bekleme 2 |
| uruk-hai-legion | Dark | isengard | UrukHaiLegion | kaleyi yoksayar |
| sauron | Dark | mordor | Maia | yıkılamaz, tespit menzilini artırır |
