package glance

import (
	"bytes"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestHeaderControlsURL(t *testing.T) {
	for _, raw := range []string{
		"https://example.invalid/controls", "//example.invalid/controls", "/a/../controls",
		"/a/%2e%2e/controls", "/a/%5ccontrols", "/a\\controls", "/a#fragment",
		"/a\ncontrols", "/a?bad=%zz", "/a//controls", "/a?bad=%0A",
	} {
		var widget bookmarksWidget
		if err := yaml.Unmarshal([]byte("type: bookmarks\nheader-controls-url: "+strconv.Quote(raw)+"\n"), &widget); err == nil {
			t.Errorf("accepted unsafe header-controls-url %q", raw)
		}
	}
	var widget bookmarksWidget
	if err := yaml.Unmarshal([]byte("type: bookmarks\nheader-controls-url: /_controls/pages/?page=projects&view=icon\n"), &widget); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := bookmarksWidgetTemplate.Execute(&output, &widget); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `class="widget-header-controls"`) || !strings.Contains(output.String(), `view=icon`) {
		t.Fatal("expected one header iframe", output.String())
	}
	widget.HeaderControlsURL = ""
	output.Reset()
	if err := bookmarksWidgetTemplate.Execute(&output, &widget); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), `widget-header-controls`) {
		t.Fatal("unconfigured widgets must not render a control")
	}
}
