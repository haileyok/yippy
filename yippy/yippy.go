package yippy

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

type Yippy struct {
	httpd     *http.Server
	echo      *echo.Echo
	filesRoot string
	password  string
}

type Args struct {
	Addr      string
	FilesRoot string
	Password  string
}

func NewYippy(args *Args) *Yippy {
	e := echo.New()

	e.Use(middleware.Recover())
	e.Use(middleware.RemoveTrailingSlash())

	httpd := &http.Server{
		Addr:    args.Addr,
		Handler: e,
	}

	y := &Yippy{
		echo:      e,
		httpd:     httpd,
		filesRoot: args.FilesRoot,
		password:  args.Password,
	}

	return y
}

func (y *Yippy) Start() error {
	y.addRoutes()

	if err := y.httpd.ListenAndServe(); err != nil {
		return err
	}

	return nil
}

func (y *Yippy) addRoutes() {
	y.echo.GET("/**/*", y.handleIndex, y.authMiddleware)
}

func (y *Yippy) authMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(e echo.Context) error {
		pts := strings.Split(e.Request().Header.Get("authorization"), " ")

		if len(pts) != 2 || pts[0] != "Bearer" || pts[1] != y.password {
			return echo.NewHTTPError(http.StatusUnauthorized)
		}

		return next(e)
	}
}
