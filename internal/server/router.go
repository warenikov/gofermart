package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	httpSwagger "github.com/swaggo/http-swagger/v2"
	"go.uber.org/zap"

	_ "github.com/warenikov/gofermart/docs" // регистрирует swagger spec
	authh "github.com/warenikov/gofermart/internal/handler/auth"
	balanceh "github.com/warenikov/gofermart/internal/handler/balance"
	orderh "github.com/warenikov/gofermart/internal/handler/order"
	mw "github.com/warenikov/gofermart/internal/middleware"
)

// Deps — внешние хендлеры/зависимости, которые подключаются в роутер.
type Deps struct {
	Auth        *authh.Handler
	Order       *orderh.Handler
	Balance     *balanceh.Handler
	TokenParser mw.TokenParser
}

// newRouter возвращает chi-роутер с базовыми middleware, health-эндпоинтом
// и подключёнными хендлерами из deps.
// Nil-хендлер из deps просто не регистрируется.
func newRouter(deps Deps, log *zap.Logger) http.Handler {
	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)
	r.Use(chimw.Compress(5))
	r.Use(mw.DecompressGzip)

	r.Get("/health", health)
	r.Get("/swagger/*", httpSwagger.Handler())

	r.Route("/api/user", func(r chi.Router) {
		if deps.Auth != nil {
			r.Post("/register", deps.Auth.Register)
			r.Post("/login", deps.Auth.Login)
		}

		if deps.TokenParser == nil {
			return
		}
		r.Group(func(r chi.Router) {
			r.Use(mw.Auth(deps.TokenParser, log))
			if deps.Order != nil {
				r.Post("/orders", deps.Order.Submit)
				r.Get("/orders", deps.Order.List)
			}
			if deps.Balance != nil {
				r.Get("/balance", deps.Balance.Get)
				r.Post("/balance/withdraw", deps.Balance.Withdraw)
				r.Get("/withdrawals", deps.Balance.ListWithdrawals)
			}
		})
	})

	return r
}

func health(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}
