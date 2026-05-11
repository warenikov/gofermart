package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	authh "github.com/warenikov/gofermart/internal/handler/auth"
)

// Deps — внешние хендлеры/зависимости, которые подключаются в роутер.
type Deps struct {
	Auth *authh.Handler
}

// newRouter возвращает chi-роутер с базовыми middleware, health-эндпоинтом
// и подключёнными хендлерами из deps.
// Nil-хендлер из deps просто не регистрируется.
func newRouter(deps Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)

	r.Get("/health", health)

	if deps.Auth != nil {
		r.Route("/api/user", func(r chi.Router) {
			r.Post("/register", deps.Auth.Register)
			r.Post("/login", deps.Auth.Login)
		})
	}

	return r
}

func health(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}
