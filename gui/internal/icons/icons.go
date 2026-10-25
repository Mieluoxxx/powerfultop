// Package icons embeds the Lucide icons (ISC, see svg/LICENSE) that stand in for the SF Symbols
// of the Swift app, which MyGo's native UI can't draw.
package icons

import (
	"embed"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"
)

//go:embed svg/*.svg
var files embed.FS

var (
	mu     sync.Mutex
	parsed = map[string]*ui.SVG{}
)

// Get returns the icon named after its Lucide file, e.g. "cpu"; it panics on an unknown name,
// which only a typo in the app can cause.
func Get(name string) *ui.SVG {
	mu.Lock()
	defer mu.Unlock()
	if svg, ok := parsed[name]; ok {
		return svg
	}
	data, err := files.ReadFile("svg/" + name + ".svg")
	if err != nil {
		panic("icons: no icon " + name)
	}
	svg, err := ui.ParseSVG(data)
	if err != nil {
		panic("icons: " + name + ": " + err.Error())
	}
	parsed[name] = svg
	return svg
}

// Names lists every embedded icon.
func Names() []string {
	entries, _ := files.ReadDir("svg")
	var names []string
	for _, e := range entries {
		if name, ok := strings.CutSuffix(e.Name(), ".svg"); ok {
			names = append(names, name)
		}
	}
	return names
}
