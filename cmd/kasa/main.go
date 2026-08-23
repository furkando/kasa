package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"

	"github.com/furkando/kasa/internal/ai"
	"github.com/furkando/kasa/internal/config"
	"github.com/furkando/kasa/internal/store"
	"github.com/furkando/kasa/internal/ui"
)

const sampleConfig = `# kasa — portföy tanımı
# Her şey TUI'den yönetilir: ↑↓ seç · enter düzenle · n yeni · x sil · o ayarlar

refresh_seconds: 30

# ai:
#   provider: claude       # claude = claude CLI (Pro/Max aboneliği) | api = ANTHROPIC_API_KEY; boş = otomatik
#   model: claude-opus-5   # boş bırakılırsa varsayılan

assets:
  cash:
    - name: Banka TL
      currency: TRY
      amount: 50000
    - name: USD Nakit
      currency: USD
      amount: 1000
      apy: 0.03           # (ops.) yıllık getiri; as_of'tan itibaren işler
      as_of: 2026-08-16
      cost_try: 40000     # (ops.) toplam maliyet ₺ — toplam K/Z için

  gold:
    grams: 50           # gram altın
    cost_try: 250000    # (ops.)

  stocks:
    - symbol: THYAO.IS  # BIST: SEMBOL.IS — ABD: AAPL, VOO, ...
      shares: 100
      cost: 285.50      # (ops.) hisse başı ort. maliyet (hissenin kendi kurunda)
    - symbol: AAPL
      shares: 10
      cost: 180

  loans:
    - name: Alacak
      currency: USD
      amount: 20000
      kind: alacak        # alacak = sana ödenecek · borc = senin borcun
      due: 2026-12-01     # ops.

  properties:
    - name: Ev
      currency: TRY
      value: 3000000      # güncel tahmini değer
      category: gayrimenkul   # serbest: araç, arsa…

  cryptos:
    - name: BTC
      symbol: BTC
      coingecko_id: bitcoin   # coingecko.com adresindeki id
      amount: 0.5
      cost: 60000             # ops. ort. maliyet $

  deposits:
    - name: Vadeli
      principal: 500000   # ₺ anapara
      gross_rate: 0.41    # brüt yıllık faiz, %41
      stopaj: 0.175       # %17,5 — net oranı kasa hesaplar
      start: 2026-08-01
      maturity: 2026-09-01
`

func main() {
	args := os.Args[1:]
	demo := false
	if len(args) > 0 && (args[0] == "demo" || args[0] == "fake") {
		demo = true
		args = args[1:]
	}

	var cfg *config.Config
	var st *store.Store

	if demo {
		cfg = config.Demo()
		if path, err := config.DemoPath(cfg); err == nil {
			os.Setenv("KASA_CONFIG", path) // demo edits land here, real config untouched
		}
		st = &store.Store{} // no-op: demo runs leave no snapshots
	} else {
		var err error
		cfg, st = loadReal()
		_ = err
	}

	if len(args) > 0 {
		switch args[0] {
		case "-1", "--once":
			width := 110
			if tw, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && tw > 40 {
				width = tw
			}
			out, err := ui.RenderOnce(cfg, st, width, demo)
			if err != nil {
				fmt.Fprintln(os.Stderr, "kasa:", err)
				os.Exit(1)
			}
			fmt.Println(out)
			return
		case "ai":
			runAI(cfg, st, strings.Join(args[1:], " "))
			return
		}
	}

	m := ui.New(cfg, st)
	m.Demo = demo
	var p *tea.Program
	m.Sender = func(msg tea.Msg) { p.Send(msg) } // p is set before Run; used only after
	p = tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "kasa:", err)
		os.Exit(1)
	}
}

func loadReal() (*config.Config, *store.Store) {
	path := config.DefaultPath()
	cfg, err := config.Load(path)
	if err != nil {
		if !os.IsNotExist(err) {
			fmt.Fprintln(os.Stderr, "kasa:", err)
			os.Exit(1)
		}
		if mkErr := os.MkdirAll(filepath.Dir(path), 0o755); mkErr != nil {
			fmt.Fprintln(os.Stderr, "kasa:", mkErr)
			os.Exit(1)
		}
		if wrErr := os.WriteFile(path, []byte(sampleConfig), 0o644); wrErr != nil {
			fmt.Fprintln(os.Stderr, "kasa:", wrErr)
			os.Exit(1)
		}
		fmt.Printf("Örnek portföy dosyası oluşturuldu:\n\n  %s\n\nKendi varlıklarınla doldurup tekrar `kasa` çalıştır.\n", path)
		os.Exit(0)
	}

	st, err := store.Open()
	if err != nil {
		fmt.Fprintln(os.Stderr, "kasa:", err)
		os.Exit(1)
	}
	return cfg, st
}

func runAI(cfg *config.Config, st *store.Store, question string) {
	fmt.Println("◆ kasa ai — güncel fiyatlar çekiliyor…")
	bctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	system, err := ai.BuildContext(bctx, cfg, st)
	cancel()
	if err != nil && system == "" {
		fmt.Fprintln(os.Stderr, "kasa:", err)
		os.Exit(1)
	}
	chat := ai.NewChat(cfg, system)

	ask := func(q string) bool {
		fmt.Println()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		if err := chat.Ask(ctx, q, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "\nkasa: AI hatası:", err)
			fmt.Fprintln(os.Stderr, "İpucu: `claude` CLI kuruluysa Max aboneliğinle çalışır (claude içinde /login); API için ANTHROPIC_API_KEY + kredi gerekir.")
			return false
		}
		fmt.Println()
		return true
	}

	if question != "" {
		if !ask(question) {
			os.Exit(1)
		}
		return
	}

	fmt.Println("Portföyün hakkında soru sor (çıkış: q veya ctrl+d)")
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for {
		fmt.Print("\nsen › ")
		if !scanner.Scan() {
			fmt.Println()
			return
		}
		q := strings.TrimSpace(scanner.Text())
		if q == "" {
			continue
		}
		if q == "q" || q == "quit" || q == "exit" || q == "çık" {
			return
		}
		ask(q)
	}
}
