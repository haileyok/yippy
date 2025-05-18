package yippy

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/labstack/echo/v4"
)

var traversalPatterns = []string{
	"../",
	"..\\",
}

func isTraversalPath(path string) (bool, error) {
	path, err := url.QueryUnescape(path)
	if err != nil {
		return true, err
	}

	for _, p := range traversalPatterns {
		if strings.Contains(path, p) {
			return true, nil
		}
	}

	return false, nil
}

func (y *Yippy) handleIndex(e echo.Context) error {
	path := y.filesRoot + e.Path()

	itp, err := isTraversalPath(path)
	if err != nil || itp {
		return echo.NewHTTPError(400)
	}

	files, err := os.ReadDir(path)

	html := fmt.Sprintf("<h3>%s</h3>", path)

	for _, f := range files {
		html += fmt.Sprintf(`<p><a href="%s">%s</a></p>`, path+f.Name(), f.Name())
	}

	return e.HTML(200, html)
}
