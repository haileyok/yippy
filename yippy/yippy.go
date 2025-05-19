package yippy

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/google/uuid"
	"github.com/gorilla/sessions"
	"github.com/labstack/echo-contrib/session"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	slogecho "github.com/samber/slog-echo"
)

type Yippy struct {
	httpd          *http.Server
	echo           *echo.Echo
	logger         *slog.Logger
	sessionManager *SessionManager
	filesRoot      string
	password       string
}

type Args struct {
	Addr          string
	FilesRoot     string
	SessionSecret string
	Password      string
	Logger        *slog.Logger
}

func NewYippy(args *Args) *Yippy {
	if args.Password == "" {
		panic("no password set in environment file")
	}

	if args.Logger == nil {
		l := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{}))
		args.Logger = l
	}

	e := echo.New()
	e.DisableHTTP2 = true

	e.Use(slogecho.New(slog.Default()))

	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
	}))

	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			reqID := uuid.NewString()
			c.Set("request_id", reqID)
			c.Response().Header().Set("X-Request-ID", reqID)
			return next(c)
		}
	})

	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogMethod: true,
		LogURI:    true,
		LogStatus: true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			args.Logger.Info("handled request",
				"method", v.Method,
				"uri", v.URI,
				"status", v.Status,
				"error", v.Error,
				"request_id", c.Get("request_id"),
			)
			return nil
		},
	}))

	// e.Use(middleware.Recover())
	e.Use(middleware.RemoveTrailingSlash())
	e.Use(session.Middleware(sessions.NewCookieStore([]byte(args.SessionSecret))))

	httpd := &http.Server{
		Addr:              args.Addr,
		Handler:           e,
		ReadTimeout:       0,
		ReadHeaderTimeout: 0,
		WriteTimeout:      0,
		IdleTimeout:       0,
	}

	y := &Yippy{
		echo:           e,
		httpd:          httpd,
		logger:         args.Logger,
		sessionManager: NewSessionManager(args.Logger),
		filesRoot:      args.FilesRoot,
		password:       args.Password,
	}

	return y
}

func (y *Yippy) Start() error {
	y.logger.Info("starting yippy", "addr", y.httpd.Addr)
	y.addRoutes()

	if err := y.httpd.ListenAndServe(); err != nil {
		return err
	}

	return nil
}

func (y *Yippy) addRoutes() {
	y.echo.GET("/login", y.handleLoginGet)
	y.echo.POST("/login", y.handleLoginPost)
	y.echo.GET("/transcode", y.handleTranscode)
	y.echo.GET("/subtitles", y.handleSubs)
	y.echo.GET("/**/*", y.handleIndex, y.authMiddleware)
}

func (y *Yippy) authMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(e echo.Context) error {
		sess, err := session.Get("session", e)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError)
		}

		loggedIn, ok := sess.Values["logged_in"].(bool)
		if !ok || !loggedIn {
			return e.Redirect(302, "/login")
		}

		return next(e)
	}
}
