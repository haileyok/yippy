package yippy

import (
	"net/http"

	"github.com/gorilla/sessions"
	"github.com/labstack/echo-contrib/session"
	"github.com/labstack/echo/v4"
)

func (y *Yippy) handleLoginGet(e echo.Context) error {
	html := `<form method="post" action="/login"><input type="password" name="password"></form>`
	return e.HTML(200, html)
}

func (y *Yippy) handleLoginPost(e echo.Context) error {
	pw := e.FormValue("password")

	if pw != y.password {
		return echo.NewHTTPError(http.StatusUnauthorized)
	}

	sess, err := session.Get("session", e)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError)
	}

	sess.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   604800,
		HttpOnly: true,
	}

	sess.Values = map[any]any{}
	sess.Values["logged_in"] = true

	if err := sess.Save(e.Request(), e.Response()); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError)
	}

	return e.Redirect(302, "/")
}
